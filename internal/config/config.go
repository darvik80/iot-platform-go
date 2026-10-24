package config

import (
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cogitor-iot-platform/internal/models"
)

// ── Sub-structs ──────────────────────────────────────────────────────

type DatabaseConfig struct {
	Driver       string // sqlite | postgres
	DSN          string
	MaxOpenConns int
}

type HTTPConfig struct {
	Addr string
}

type MQTTConfig struct {
	Addr    string
	WSAddr  string
	TLSAddr string
	TLSCert string
	TLSKey  string
	TLSCA   string

	// Ports announced to clients when they differ from the listen ports
	// (e.g. Docker -p 11883:1883). 0 = use the listen port.
	PublicTCPPort int
	PublicWSPort  int
	PublicTLSPort int

	// Full WebSocket URL announced to clients when MQTT over WS is published
	// behind a reverse proxy / CDN (e.g. wss://iot.example.com/mqtt) instead of
	// ws://{broker host}:{WS port}/. Empty = derive from the broker host.
	PublicWSURL string
}

type AuthConfig struct {
	JWTSecret string
}

type EmailConfig struct {
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string
	Mode     string // smtp | log
}

type RegisterConfig struct {
	Enabled       bool
	DefaultRole   models.Role
	RequireVerify bool
}

type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type RulesBusConfig struct {
	Enabled bool
	Prefix  string
}

type OTAConfig struct {
	Dir string
}

type PlatformConfig struct {
	Name                   string
	PublicBaseURL          string
	BrokerHostPublic       string
	TelemetryRetentionDays int
	RPCDefaultTimeout      time.Duration
	LabAllowUnknownDevices bool
}

// ── Top-level Config ─────────────────────────────────────────────────

type Config struct {
	Database  DatabaseConfig
	HTTP      HTTPConfig
	MQTT      MQTTConfig
	Auth      AuthConfig
	Email     EmailConfig
	Register  RegisterConfig
	Google    GoogleConfig
	Rules     RulesBusConfig
	OTA       OTAConfig
	Platform  PlatformConfig
	Timezone  string
	LogLevel  string
	DataDir   string
	Admin     AdminConfig
	Bootstrap BootstrapConfig
}

type AdminConfig struct {
	Email    string
	Password string
	Login    string
}

type BootstrapConfig struct {
	AccountSlug string
	AccountName string
}

// ── Load ─────────────────────────────────────────────────────────────

func Load() Config {
	role := models.Role(getenv("REGISTER_DEFAULT_ROLE", "admin", "AUTH_REGISTER_DEFAULT_ROLE"))
	switch role {
	case models.RoleAdmin, models.RoleOperator, models.RoleViewer:
	default:
		role = models.RoleViewer
	}

	// Database driver auto-detection with backward compatibility
	driver := getenv("DATABASE_DRIVER", "", "DATABASE_DRIVER")
	dsn := getenv("DATABASE_DSN", "", "DATABASE_DSN")
	sqlitePath := getenv("SQLITE_PATH", "/opt/iot-platform/lib/platform.db", "DATABASE_SQLITE_PATH")

	// If no explicit driver, check for legacy SQLITE_PATH or default to sqlite
	if driver == "" {
		if os.Getenv("SQLITE_PATH") != "" || os.Getenv("DATABASE_SQLITE_PATH") != "" {
			driver = "sqlite"
			dsn = sqlitePath
		} else {
			driver = "sqlite"
			dsn = sqlitePath
		}
	} else if dsn == "" {
		// If driver is set but DSN is empty, use default for that driver
		if driver == "postgres" {
			dsn = "postgres://localhost:5432/iot_platform?sslmode=disable"
		} else {
			dsn = sqlitePath
		}
	}

	dataDir := getenv("DATA_DIR", "", "DATA_DIR")
	if dataDir == "" {
		// Default to directory containing the database file
		if driver == "sqlite" {
			dataDir = filepath.Dir(dsn)
		} else {
			dataDir = "./data"
		}
	}

	return Config{
		Database: DatabaseConfig{
			Driver:       driver,
			DSN:          dsn,
			MaxOpenConns: getenvInt("DB_MAX_OPEN_CONNS", 8, "DATABASE_MAX_OPEN_CONNS"),
		},
		HTTP: HTTPConfig{
			Addr: getenv("HTTP_ADDR", ":8080", "SERVER_HTTP_ADDR"),
		},
		MQTT: MQTTConfig{
			Addr:    getenv("MQTT_ADDR", ":1883"),
			WSAddr:  getenv("MQTT_WS_ADDR", ":8083"),
			TLSAddr: getenv("MQTT_TLS_ADDR", ":8883"),
			TLSCert: getenv("MQTT_TLS_CERT", ""),
			TLSKey:  getenv("MQTT_TLS_KEY", ""),
			TLSCA:   getenv("MQTT_TLS_CA", ""),

			PublicTCPPort: getenvInt("MQTT_PUBLIC_PORT", 0),
			PublicWSPort:  getenvInt("MQTT_WS_PUBLIC_PORT", 0),
			PublicTLSPort: getenvInt("MQTT_TLS_PUBLIC_PORT", 0),
			PublicWSURL:   strings.TrimSpace(getenv("MQTT_WS_PUBLIC_URL", "", "PLATFORM_MQTT_WS_PUBLIC_URL")),
		},
		Auth: AuthConfig{
			JWTSecret: getenv("JWT_SECRET", DevJWTSecret, "AUTH_JWT_SECRET"),
		},
		Email: EmailConfig{
			SMTPHost: getenv("SMTP_HOST", "", "EMAIL_SMTP_HOST"),
			SMTPPort: getenvInt("SMTP_PORT", 587, "EMAIL_SMTP_PORT"),
			SMTPUser: getenv("SMTP_USER", "", "EMAIL_SMTP_USER"),
			SMTPPass: getenv("SMTP_PASS", "", "EMAIL_SMTP_PASS"),
			SMTPFrom: getenv("SMTP_FROM", "noreply@cogitor.local", "EMAIL_SMTP_FROM"),
			Mode:     strings.ToLower(getenv("EMAIL_MODE", "")),
		},
		Register: RegisterConfig{
			Enabled:       getenvBool("REGISTER_ENABLED", true, "AUTH_REGISTER_ENABLED"),
			DefaultRole:   role,
			RequireVerify: getenvBool("REQUIRE_EMAIL_VERIFY", false, "AUTH_REQUIRE_EMAIL_VERIFY"),
		},
		Google: GoogleConfig{
			ClientID:     getenv("GOOGLE_CLIENT_ID", ""),
			ClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
			RedirectURL:  getenv("GOOGLE_REDIRECT_URL", ""),
		},
		Rules: RulesBusConfig{
			Enabled: getenvBool("RULES_BUS_ENABLED", true),
			Prefix:  strings.TrimRight(getenv("RULES_BUS_PREFIX", "platform/v1"), "/"),
		},
		OTA: OTAConfig{
			Dir: getenv("OTA_DIR", ""),
		},
		Platform: PlatformConfig{
			Name:                   getenv("PLATFORM_NAME", "Cogitor IoT", "SERVER_PLATFORM_NAME"),
			PublicBaseURL:          strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:8080", "SERVER_PUBLIC_BASE_URL", "PLATFORM_PUBLIC_BASE_URL"), "/"),
			BrokerHostPublic:       NormalizeHost(firstNonEmpty(os.Getenv("PLATFORM_BROKER_HOST_PUBLIC"), os.Getenv("BROKER_HOST_PUBLIC"), os.Getenv("MQTT_BROKER_HOST_PUBLIC"), os.Getenv("MQTT_HOST_PUBLIC"), "localhost")),
			TelemetryRetentionDays: getenvInt("TELEMETRY_RETENTION_DAYS", 30, "PLATFORM_TELEMETRY_RETENTION_DAYS"),
			RPCDefaultTimeout:      time.Duration(getenvInt("RPC_DEFAULT_TIMEOUT_MS", 30000, "PLATFORM_RPC_DEFAULT_TIMEOUT_MS")) * time.Millisecond,
			LabAllowUnknownDevices: getenvBool("LAB_ALLOW_UNKNOWN_DEVICES", false, "PLATFORM_LAB_ALLOW_UNKNOWN_DEVICES"),
		},
		Timezone: strings.TrimSpace(getenv("TIMEZONE", "UTC")),
		LogLevel: strings.ToLower(strings.TrimSpace(getenv("LOG_LEVEL", "info"))),
		DataDir:  dataDir,
		Admin: AdminConfig{
			Email:    getenv("ADMIN_EMAIL", "admin@cogitor.local"),
			Password: getenv("ADMIN_PASSWORD", "admin"),
			Login:    getenv("ADMIN_LOGIN", "admin"),
		},
		Bootstrap: BootstrapConfig{
			AccountSlug: getenv("BOOTSTRAP_ACCOUNT_SLUG", "default"),
			AccountName: getenv("BOOTSTRAP_ACCOUNT_NAME", "Default"),
		},
	}
}

// ── Methods ──────────────────────────────────────────────────────────

// EffectiveMode is smtp when a host is configured, unless EMAIL_MODE=log.
// An unset mode with no host stays log. The setup wizard used to omit
// email.mode, so a host in config.yml must be enough to send mail.
func (e EmailConfig) EffectiveMode() string {
	mode := strings.ToLower(strings.TrimSpace(e.Mode))
	if mode == "log" {
		return "log"
	}
	if strings.TrimSpace(e.SMTPHost) == "" {
		return "log"
	}
	return "smtp"
}

func (c Config) SlogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(c.LogLevel)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// TimezoneOrUTC returns a valid IANA zone (UTC if empty or unknown).
func (c Config) TimezoneOrUTC() string {
	tz := strings.TrimSpace(c.Timezone)
	if tz == "" {
		return time.UTC.String()
	}
	if _, err := time.LoadLocation(tz); err == nil {
		return tz
	}
	return time.UTC.String()
}

func (c Config) GoogleEnabled() bool {
	return c.Google.ClientID != "" && c.Google.ClientSecret != ""
}

func (c Config) GoogleCallbackURL() string {
	if c.Google.RedirectURL != "" {
		return c.Google.RedirectURL
	}
	return c.Platform.PublicBaseURL + "/api/v1/auth/google/callback"
}

// ── MQTT helpers ─────────────────────────────────────────────────────

func (m MQTTConfig) PublicPort() int {
	if m.PublicTCPPort > 0 {
		return m.PublicTCPPort
	}
	return parseListenPort(m.Addr, 1883)
}

// PublicURL is mqtt://{BROKER_HOST_PUBLIC}:{MQTT_PORT}.
func (m MQTTConfig) PublicURL(host string) string {
	port := m.PublicPort()
	if port <= 0 {
		return ""
	}
	if host == "" {
		host = "localhost"
	}
	return "mqtt://" + host + ":" + strconv.Itoa(port)
}

func (m MQTTConfig) TLSEnabled() bool {
	return m.TLSCert != "" && m.TLSKey != ""
}

func (m MQTTConfig) TLSPublicPort() int {
	if !m.TLSEnabled() {
		return 0
	}
	if m.PublicTLSPort > 0 {
		return m.PublicTLSPort
	}
	return parseListenPort(m.TLSAddr, 8883)
}

func (m MQTTConfig) TLSPublicURL(host string) string {
	port := m.TLSPublicPort()
	if port <= 0 {
		return ""
	}
	if host == "" {
		host = "localhost"
	}
	return "mqtts://" + host + ":" + strconv.Itoa(port)
}

func (m MQTTConfig) WSEnabled() bool {
	ws := strings.TrimSpace(m.WSAddr)
	return ws != "" && !strings.EqualFold(ws, "off") && ws != "-"
}

func (m MQTTConfig) WSPublicPort() int {
	if !m.WSEnabled() {
		return 0
	}
	if m.PublicWSURL != "" {
		if u, err := url.Parse(m.PublicWSURL); err == nil {
			if p, err := strconv.Atoi(u.Port()); err == nil && p > 0 {
				return p
			}
			if strings.EqualFold(u.Scheme, "wss") {
				return 443
			}
			return 80
		}
	}
	if m.PublicWSPort > 0 {
		return m.PublicWSPort
	}
	return parseListenPort(m.WSAddr, 8083)
}

func (m MQTTConfig) WSPublicURL(host string) string {
	port := m.WSPublicPort()
	if port <= 0 {
		return ""
	}
	if m.PublicWSURL != "" {
		return m.PublicWSURL
	}
	if host == "" {
		host = "localhost"
	}
	return "ws://" + host + ":" + strconv.Itoa(port) + "/"
}

// NormalizeHost reduces a broker host entered as a URL or host:port
// ("mqtt://mqtt.example.com:1883/") to the bare host name or IP.
func NormalizeHost(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return s
}

// ── OTA / Firmware helpers ───────────────────────────────────────────

func (c Config) FirmwareDir() string {
	if strings.TrimSpace(c.OTA.Dir) != "" {
		return c.OTA.Dir
	}
	return filepath.Join(c.DataDir, "firmware")
}

func (c Config) FirmwarePublicURL(token string) string {
	return strings.TrimRight(c.Platform.PublicBaseURL, "/") + "/ota/fw/" + token + ".bin"
}

// ── Shared helpers ───────────────────────────────────────────────────

func parseListenPort(addr string, def int) int {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimPrefix(addr, "tcp://")
	addr = strings.TrimPrefix(addr, "mqtt://")
	addr = strings.TrimPrefix(addr, "ws://")
	addr = strings.TrimPrefix(addr, "wss://")
	if i := strings.IndexByte(addr, '/'); i >= 0 {
		addr = addr[:i]
	}
	if addr == "" {
		return def
	}
	if !strings.Contains(addr, ":") {
		n, err := strconv.Atoi(addr)
		if err == nil && n > 0 {
			return n
		}
		return def
	}
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return def
	}
	n, err := strconv.Atoi(p)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func getenv(k, def string, alts ...string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	for _, a := range alts {
		if v := os.Getenv(a); v != "" {
			return v
		}
	}
	return def
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func getenvInt(k string, def int, alts ...string) int {
	v := firstEnv(append([]string{k}, alts...)...)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvBool(k string, def bool, alts ...string) bool {
	v := strings.ToLower(firstEnv(append([]string{k}, alts...)...))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}
