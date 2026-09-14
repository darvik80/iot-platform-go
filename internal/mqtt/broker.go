package mqtt

import (
	"cogitor-iot-platform/internal/protocol"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/services"
)

// sourceRoot is the project root detected at init time via runtime.Caller.
// Empty in production binaries (source files not available).
var sourceRoot string

func init() {
	if _, f, _, ok := runtime.Caller(0); ok {
		// f = .../internal/mqtt/broker.go → walk up to find go.mod
		dir := filepath.Dir(f)
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				sourceRoot = dir
				return
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return
			}
			dir = parent
		}
	}
}

// Broker is an in-process MQTT server (mochi). Devices and Node-RED connect here;
// the platform publishes downlinks / rules-bus events via the inline client.
type Broker struct {
	cfg           config.Config
	plat          *services.Platform
	log           *slog.Logger
	server        *mochi.Server
	listenAddr    string
	tlsListenAddr string
	ok            atomic.Bool

	sessMu   sync.Mutex
	sessions map[string]*sessMeta
	evMu     sync.Mutex
	events   []models.MQTTEvent
}

func New(cfg config.Config, plat *services.Platform, log *slog.Logger) *Broker {
	if log == nil {
		log = slog.Default()
	}
	return &Broker{cfg: cfg, plat: plat, log: log, sessions: map[string]*sessMeta{}}
}

func (b *Broker) Connected() bool { return b.ok.Load() }

func (b *Broker) Start() error {
	addr := b.cfg.MQTTAddr
	if addr == "" {
		addr = ":1883"
	}
	ln, err := listenTCP(addr)
	if err != nil {
		return fmt.Errorf("mqtt listen: %w", err)
	}
	b.listenAddr = ln.Addr().String()

	s := mochi.New(&mochi.Options{
		InlineClient: true,
		Logger:       b.log.With("component", "mqtt"),
	})
	if err := s.AddHook(&platHook{b: b}, nil); err != nil {
		_ = ln.Close()
		return fmt.Errorf("mqtt hook: %w", err)
	}
	if err := s.AddListener(listeners.NewNet("tcp", ln)); err != nil {
		_ = ln.Close()
		return fmt.Errorf("mqtt listener: %w", err)
	}
	if b.cfg.MQTTWSEnabled() {
		ws := normalizeMQTTAddr(b.cfg.MQTTWSAddr)
		if err := s.AddListener(listeners.NewWebsocket(listeners.Config{ID: "ws", Address: ws})); err != nil {
			_ = ln.Close()
			return fmt.Errorf("mqtt ws listen %s: %w", ws, err)
		}
		b.log.Info("mqtt websocket listening", "addr", ws, "url", b.cfg.MQTTWSPublicURL())
	}
	if b.cfg.MQTTTLSEnabled() {
		tlsCfg, err := b.loadTLSConfig()
		if err != nil {
			_ = ln.Close()
			return fmt.Errorf("mqtt tls config: %w", err)
		}
		tlsAddr := normalizeMQTTAddr(b.cfg.MQTTTLSAddr)
		tcpL := listeners.NewTCP(listeners.Config{
			ID:        "tls",
			Address:   tlsAddr,
			TLSConfig: tlsCfg,
		})
		if err := s.AddListener(tcpL); err != nil {
			_ = ln.Close()
			return fmt.Errorf("mqtt tls listen %s: %w", tlsAddr, err)
		}
		b.tlsListenAddr = tlsAddr
		b.log.Info("mqtt tls listening", "addr", tlsAddr, "url", b.cfg.MQTTTLSPublicURL(),
			"mutual_tls", b.cfg.MQTTTLSCA != "")
	}
	b.server = s
	errCh := make(chan error, 1)
	go func() {
		if err := s.Serve(); err != nil {
			b.ok.Store(false)
			errCh <- err
			b.log.Error("mqtt broker stopped", "err", err)
		}
	}()
	select {
	case err := <-errCh:
		return fmt.Errorf("mqtt serve: %w", err)
	case <-time.After(150 * time.Millisecond):
	}
	b.ok.Store(true)
	b.log.Info("mqtt broker listening", "addr", b.listenAddr, "ws", b.cfg.MQTTWSAddr, "tls", b.tlsListenAddr)
	return nil
}

// loadTLSConfig builds a tls.Config from the configured cert/key files.
// If MQTT_TLS_CA is set, mutual TLS (client certificate verification) is enabled.
// Paths may be absolute or relative (resolved against the executable directory).
func (b *Broker) loadTLSConfig() (*tls.Config, error) {
	certPath := resolveTLSPath(b.cfg.MQTTTLSCert)
	keyPath := resolveTLSPath(b.cfg.MQTTTLSKey)

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load cert/key (%s, %s): %w", certPath, keyPath, err)
	}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if caFile := b.cfg.MQTTTLSCA; caFile != "" {
		caPath := resolveTLSPath(caFile)
		caCert, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read CA cert (%s): %w", caPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("invalid CA certificate(s)")
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	return cfg, nil
}

// resolveTLSPath returns p as-is if absolute, otherwise tries to find it:
//  1. Relative to CWD
//  2. Relative to the source root (detected via runtime.Caller at init)
//  3. Relative to the executable directory
//  4. Falls back to p unchanged
func resolveTLSPath(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	// 1. Try CWD
	if _, err := os.Stat(p); err == nil {
		return p
	}
	// 2. Try source root (works in go run / IDE — source files available)
	if sourceRoot != "" {
		candidate := filepath.Join(sourceRoot, p)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 3. Try exe directory (works in Docker/systemd with certs next to binary)
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), p)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// 4. Return as-is
	return p
}

func (b *Broker) Close() {
	b.ok.Store(false)
	if b.server != nil {
		_ = b.server.Close()
	}
}

func (b *Broker) Publish(topic string, qos byte, retained bool, payload []byte) error {
	if b.server == nil {
		return fmt.Errorf("mqtt broker not started")
	}
	err := b.server.Publish(topic, payload, retained, qos)
	if err != nil {
		b.note("error", "publish", "inline", "", "", topic, err.Error(), len(payload))
		return err
	}
	level := "info"
	if !strings.Contains(topic, "/"+protocol.SegEvents+"/") {
		level = "info"
	}
	b.note(level, "publish", "inline", "", "", topic, "", len(payload))
	return nil
}

// DropUsername disconnects every session authenticated as username
// (token rotate / disable / delete).
func (b *Broker) DropUsername(username string) {
	if b.server == nil || username == "" {
		return
	}
	n := 0
	for _, cl := range b.server.Clients.GetAll() {
		if cl.Net.Inline {
			continue
		}
		if string(cl.Properties.Username) == username {
			n++
			_ = b.server.DisconnectClient(cl, packets.ErrNotAuthorized)
		}
	}
	if n > 0 {
		b.note("info", "kick", "", username, "", "", fmt.Sprintf("dropped %d session(s)", n), 0)
	}
}
