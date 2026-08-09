package mqtt

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"

	"crearts-iot-platform/internal/config"
	"crearts-iot-platform/internal/models"
	"crearts-iot-platform/internal/services"
)

// Broker is an in-process MQTT server (mochi). Devices and Node-RED connect here;
// the platform publishes downlinks / rules-bus events via the inline client.
type Broker struct {
	cfg        config.Config
	plat       *services.Platform
	log        *slog.Logger
	server     *mochi.Server
	listenAddr string
	ok         atomic.Bool

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
	if ws := b.cfg.MQTTWSAddr; ws != "" {
		if err := s.AddListener(listeners.NewWebsocket(listeners.Config{ID: "ws", Address: ws})); err != nil {
			_ = ln.Close()
			return fmt.Errorf("mqtt ws listen %s: %w", ws, err)
		}
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
	b.log.Info("mqtt broker listening", "addr", b.listenAddr, "ws", b.cfg.MQTTWSAddr)
	return nil
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
	level := "debug"
	if !strings.Contains(topic, "/events/") {
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
