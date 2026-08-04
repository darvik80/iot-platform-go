package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"crearts-iot-platform/internal/models"
)

type Config struct {
	HTTPAddr               string
	SQLitePath             string
	MQTTBroker             string
	MQTTUsername           string
	MQTTPassword           string
	MQTTClientID           string
	JWTSecret              string
	TelemetryRetentionDays int
	RPCDefaultTimeout      time.Duration
	LabAllowUnknown        bool
	RulesBusEnabled        bool
	RulesBusPrefix         string
	NodeRedURL             string
	RabbitMQMgmtURL        string
	BrokerHostPublic       string
	AdminEmail             string
	AdminPassword          string
	AdminLogin             string
	PlatformName           string
	PublicBaseURL          string
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
		MQTTBroker:             getenv("MQTT_BROKER", "tcp://192.168.1.100:1883"),
		MQTTUsername:           getenv("MQTT_USERNAME", "crearts"),
		MQTTPassword:           getenv("MQTT_PASSWORD", "crearts"),
		MQTTClientID:           getenv("MQTT_CLIENT_ID", "crearts-platform"),
		JWTSecret:              getenv("JWT_SECRET", "dev-jwt-secret-change-me"),
		TelemetryRetentionDays: getenvInt("TELEMETRY_RETENTION_DAYS", 30),
		RPCDefaultTimeout:      time.Duration(getenvInt("RPC_DEFAULT_TIMEOUT_MS", 30000)) * time.Millisecond,
		LabAllowUnknown:        getenvBool("LAB_ALLOW_UNKNOWN_DEVICES", false),
		RulesBusEnabled:        getenvBool("RULES_BUS_ENABLED", true),
		RulesBusPrefix:         strings.TrimRight(getenv("RULES_BUS_PREFIX", "platform/v1"), "/"),
		NodeRedURL:             getenv("NODERED_URL", "http://localhost:1880"),
		RabbitMQMgmtURL:        getenv("RABBITMQ_MGMT_URL", "http://192.168.1.100:15672"),
		BrokerHostPublic:       getenv("BROKER_HOST_PUBLIC", "192.168.1.100"),
		AdminEmail:             getenv("ADMIN_EMAIL", "admin@crearts.local"),
		AdminPassword:          getenv("ADMIN_PASSWORD", "admin"),
		AdminLogin:             getenv("ADMIN_LOGIN", "admin"),
		PlatformName:           getenv("PLATFORM_NAME", "Crearts IoT"),
		PublicBaseURL:          strings.TrimRight(getenv("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
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

func (c Config) GoogleCallbackURL() string {
	if c.GoogleRedirectURL != "" {
		return c.GoogleRedirectURL
	}
	return c.PublicBaseURL + "/api/v1/auth/google/callback"
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
