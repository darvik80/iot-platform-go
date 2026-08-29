package config

import (
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cogitor-iot-platform/internal/models"
)

type Config struct {
	HTTPAddr               string
	SQLitePath             string
	MQTTAddr               string
	MQTTWSAddr             string
	MQTTTLSAddr            string
	MQTTTLSCert            string
	MQTTTLSKey             string
	MQTTTLSCA              string
	JWTSecret              string
	TelemetryRetentionDays int
	RPCDefaultTimeout      time.Duration
	LabAllowUnknown        bool
	RulesBusEnabled        bool
	RulesBusPrefix         string
	NodeRedURL             string
	BrokerHostPublic       string
	AdminEmail             string
	AdminPassword          string
	AdminLogin             string
	PlatformName           string
	PublicBaseURL          string
	Timezone               string
	LogLevel               string
	OTADir                 string
	RegisterEnabled        bool
	RegisterDefaultRole    models.Role
	RequireEmailVerify     bool
	SMTPHost               string
	SMTPPort               int
	SMTPUser               string
	SMTPPass               string
	SMTPFrom               string
	EmailMode              string // smtp | log
	GoogleClientID         string
	GoogleClientSecret     string
	GoogleRedirectURL      string
}

func Load() Config {
	role := models.Role(getenv("REGISTER_DEFAULT_ROLE", "viewer", "AUTH_REGISTER_DEFAULT_ROLE"))
	switch role {
	case models.RoleAdmin, models.RoleOperator, models.RoleViewer:
	default:
		role = models.RoleViewer
	}
	return Config{
		HTTPAddr:               getenv("HTTP_ADDR", ":8080", "SERVER_HTTP_ADDR"),
		SQLitePath:             getenv("SQLITE_PATH", "./data/platform.db", "DATABASE_SQLITE_PATH"),
		MQTTAddr:               getenv("MQTT_ADDR", ":1883"),
		MQTTWSAddr:             getenv("MQTT_WS_ADDR", ":8083"),
		MQTTTLSAddr:            getenv("MQTT_TLS_ADDR", ":8883"),
		MQTTTLSCert:            getenv("MQTT_TLS_CERT", ""),
		MQTTTLSKey:             getenv("MQTT_TLS_KEY", ""),
		MQTTTLSCA:              getenv("MQTT_TLS_CA", ""),
		JWTSecret:              getenv("JWT_SECRET", "dev-jwt-secret-change-me", "AUTH_JWT_SECRET"),
		TelemetryRetentionDays: getenvInt("TELEMETRY_RETENTION_DAYS", 30),
		RPCDefaultTimeout:      time.Duration(getenvInt("RPC_DEFAULT_TIMEOUT_MS", 30000)) * time.Millisecond,
		LabAllowUnknown:        getenvBool("LAB_ALLOW_UNKNOWN_DEVICES", false),
		RulesBusEnabled:        getenvBool("RULES_BUS_ENABLED", true),
		RulesBusPrefix:         strings.TrimRight(getenv("RULES_BUS_PREFIX", "platform/v1"), "/"),
		NodeRedURL:             getenv("NODERED_URL", "http://localhost:1880"),
		BrokerHostPublic:       firstNonEmpty(os.Getenv("BROKER_HOST_PUBLIC"), os.Getenv("MQTT_BROKER_HOST_PUBLIC"), os.Getenv("MQTT_HOST_PUBLIC"), "localhost"),
		AdminEmail:             getenv("ADMIN_EMAIL", "admin@cogitor.local"),
		AdminPassword:          getenv("ADMIN_PASSWORD", "admin"),
		AdminLogin:             getenv("ADMIN_LOGIN", "admin"),
		PlatformName:           getenv("PLATFORM_NAME", "Cogitor IoT", "SERVER_PLATFORM_NAME"),
		PublicBaseURL:          strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:8080", "SERVER_PUBLIC_BASE_URL"), "/"),
		Timezone:               strings.TrimSpace(getenv("TIMEZONE", "UTC")),
		LogLevel:               strings.ToLower(strings.TrimSpace(getenv("LOG_LEVEL", "info"))),
		OTADir:                 getenv("OTA_DIR", ""),
		RegisterEnabled:        getenvBool("REGISTER_ENABLED", true, "AUTH_REGISTER_ENABLED"),
		RegisterDefaultRole:    role,
		RequireEmailVerify:     getenvBool("REQUIRE_EMAIL_VERIFY", true, "AUTH_REQUIRE_EMAIL_VERIFY"),
		SMTPHost:               getenv("SMTP_HOST", "", "EMAIL_SMTP_HOST"),
		SMTPPort:               getenvInt("SMTP_PORT", 587, "EMAIL_SMTP_PORT"),
		SMTPUser:               getenv("SMTP_USER", "", "EMAIL_SMTP_USER"),
		SMTPPass:               getenv("SMTP_PASS", "", "EMAIL_SMTP_PASS"),
		SMTPFrom:               getenv("SMTP_FROM", "noreply@cogitor.local", "EMAIL_SMTP_FROM"),
		EmailMode:              strings.ToLower(getenv("EMAIL_MODE", "log")),
		GoogleClientID:         getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:     getenv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:      getenv("GOOGLE_REDIRECT_URL", ""),
	}
}

func (c Config) GoogleEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
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
		return "UTC"
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return "UTC"
	}
	return tz
}

func (c Config) MQTTPublicPort() int {
	return parseListenPort(c.MQTTAddr, 1883)
}

// MQTTPublicURL is mqtt://{BROKER_HOST_PUBLIC}:{MQTT_PORT}.
func (c Config) MQTTPublicURL() string {
	port := c.MQTTPublicPort()
	if port <= 0 {
		return ""
	}
	host := strings.TrimSpace(c.BrokerHostPublic)
	if host == "" {
		host = "localhost"
	}
	return "mqtt://" + host + ":" + strconv.Itoa(port)
}

// MQTTTLSEnabled reports whether MQTT-over-TLS is configured (cert + key present).
func (c Config) MQTTTLSEnabled() bool {
	return c.MQTTTLSCert != "" && c.MQTTTLSKey != ""
}

// MQTTTLSPublicPort is the advertised MQTT-TLS port (0 if disabled).
func (c Config) MQTTTLSPublicPort() int {
	if !c.MQTTTLSEnabled() {
		return 0
	}
	return parseListenPort(c.MQTTTLSAddr, 8883)
}

// MQTTTLSPublicURL is mqtts://{BROKER_HOST_PUBLIC}:{MQTT_TLS_PORT}/ (empty if TLS off).
func (c Config) MQTTTLSPublicURL() string {
	port := c.MQTTTLSPublicPort()
	if port <= 0 {
		return ""
	}
	host := strings.TrimSpace(c.BrokerHostPublic)
	if host == "" {
		host = "localhost"
	}
	return "mqtts://" + host + ":" + strconv.Itoa(port)
}

// MQTTWSEnabled reports whether MQTT-over-WebSocket listener is configured.
func (c Config) MQTTWSEnabled() bool {
	ws := strings.TrimSpace(c.MQTTWSAddr)
	return ws != "" && !strings.EqualFold(ws, "off") && ws != "-"
}

// MQTTWSPublicPort is the advertised MQTT WebSocket port (0 if disabled).
func (c Config) MQTTWSPublicPort() int {
	if !c.MQTTWSEnabled() {
		return 0
	}
	return parseListenPort(c.MQTTWSAddr, 8083)
}

// MQTTWSPublicURL is ws://{BROKER_HOST_PUBLIC}:{MQTT_WS_PORT}/ (empty if WS off).
func (c Config) MQTTWSPublicURL() string {
	port := c.MQTTWSPublicPort()
	if port <= 0 {
		return ""
	}
	host := strings.TrimSpace(c.BrokerHostPublic)
	if host == "" {
		host = "localhost"
	}
	return "ws://" + host + ":" + strconv.Itoa(port) + "/"
}

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

func (c Config) FirmwareDir() string {
	if strings.TrimSpace(c.OTADir) != "" {
		return c.OTADir
	}
	return filepath.Join(filepath.Dir(c.SQLitePath), "firmware")
}

func (c Config) FirmwarePublicURL(token string) string {
	return strings.TrimRight(c.PublicBaseURL, "/") + "/ota/fw/" + token + ".bin"
}

func (c Config) GoogleCallbackURL() string {
	if c.GoogleRedirectURL != "" {
		return c.GoogleRedirectURL
	}
	return c.PublicBaseURL + "/api/v1/auth/google/callback"
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
