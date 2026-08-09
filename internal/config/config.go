package config

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"crearts-iot-platform/internal/models"
)

type Config struct {
	HTTPAddr               string
	SQLitePath             string
	MQTTAddr               string
	MQTTWSAddr             string
	JWTSecret              string
	TelemetryRetentionDays int
	RPCDefaultTimeout      time.Duration
	LabAllowUnknown        bool
	RulesBusEnabled        bool
	RulesBusPrefix         string
	NodeRedURL             string
	NodeRedMQTTUser        string
	NodeRedMQTTPassword    string
	BrokerHostPublic       string
	AdminEmail             string
	AdminPassword          string
	AdminLogin             string
	PlatformName           string
	PublicBaseURL          string
	Timezone               string
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
	role := models.Role(getenv("REGISTER_DEFAULT_ROLE", "viewer"))
	switch role {
	case models.RoleAdmin, models.RoleOperator, models.RoleViewer:
	default:
		role = models.RoleViewer
	}
	return Config{
		HTTPAddr:               getenv("HTTP_ADDR", ":8080"),
		SQLitePath:             getenv("SQLITE_PATH", "./data/platform.db"),
		MQTTAddr:               getenv("MQTT_ADDR", ":1883"),
		MQTTWSAddr:             getenv("MQTT_WS_ADDR", ""),
		JWTSecret:              getenv("JWT_SECRET", "dev-jwt-secret-change-me"),
		TelemetryRetentionDays: getenvInt("TELEMETRY_RETENTION_DAYS", 30),
		RPCDefaultTimeout:      time.Duration(getenvInt("RPC_DEFAULT_TIMEOUT_MS", 30000)) * time.Millisecond,
		LabAllowUnknown:        getenvBool("LAB_ALLOW_UNKNOWN_DEVICES", false),
		RulesBusEnabled:        getenvBool("RULES_BUS_ENABLED", true),
		RulesBusPrefix:         strings.TrimRight(getenv("RULES_BUS_PREFIX", "platform/v1"), "/"),
		NodeRedURL:             getenv("NODERED_URL", "http://localhost:1880"),
		NodeRedMQTTUser:        getenv("NODERED_MQTT_USER", "nodered"),
		NodeRedMQTTPassword:    getenv("NODERED_MQTT_PASSWORD", "nodered"),
		BrokerHostPublic:       firstNonEmpty(os.Getenv("BROKER_HOST_PUBLIC"), os.Getenv("MQTT_BROKER_HOST_PUBLIC"), os.Getenv("MQTT_HOST_PUBLIC"), "localhost"),
		AdminEmail:             getenv("ADMIN_EMAIL", "admin@crearts.local"),
		AdminPassword:          getenv("ADMIN_PASSWORD", "admin"),
		AdminLogin:             getenv("ADMIN_LOGIN", "admin"),
		PlatformName:           getenv("PLATFORM_NAME", "Crearts IoT"),
		PublicBaseURL:          strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		Timezone:               strings.TrimSpace(getenv("TIMEZONE", "UTC")),
		OTADir:                 getenv("OTA_DIR", ""),
		RegisterEnabled:        getenvBool("REGISTER_ENABLED", true),
		RegisterDefaultRole:    role,
		RequireEmailVerify:     getenvBool("REQUIRE_EMAIL_VERIFY", true),
		SMTPHost:               getenv("SMTP_HOST", ""),
		SMTPPort:               getenvInt("SMTP_PORT", 587),
		SMTPUser:               getenv("SMTP_USER", ""),
		SMTPPass:               getenv("SMTP_PASS", ""),
		SMTPFrom:               getenv("SMTP_FROM", "noreply@crearts.local"),
		EmailMode:              strings.ToLower(getenv("EMAIL_MODE", "log")),
		GoogleClientID:         getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:     getenv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:      getenv("GOOGLE_REDIRECT_URL", ""),
	}
}

func (c Config) GoogleEnabled() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
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
	addr := strings.TrimSpace(c.MQTTAddr)
	addr = strings.TrimPrefix(addr, "tcp://")
	addr = strings.TrimPrefix(addr, "mqtt://")
	if addr == "" {
		return 1883
	}
	if !strings.Contains(addr, ":") {
		n, err := strconv.Atoi(addr)
		if err == nil && n > 0 {
			return n
		}
		return 1883
	}
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 1883
	}
	n, err := strconv.Atoi(p)
	if err != nil || n <= 0 {
		return 1883
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

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvBool(k string, def bool) bool {
	v := strings.ToLower(os.Getenv(k))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}
