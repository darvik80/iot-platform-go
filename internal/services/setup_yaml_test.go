package services

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cogitor-iot-platform/internal/config"
)

func TestMarshalConfigYAMLRoundTrip(t *testing.T) {
	cfg := config.Config{
		Database: config.DatabaseConfig{Driver: "sqlite", DSN: "/opt/iot-platform/lib/platform.db"},
		Auth:     config.AuthConfig{JWTSecret: "secret-value"},
		Platform: config.PlatformConfig{PublicBaseURL: "https://iot.example", BrokerHostPublic: "mqtt.example"},
		Email: config.EmailConfig{
			SMTPHost: "smtp.example",
			SMTPPort: 465,
			SMTPUser: "bot",
			SMTPPass: "pw",
			SMTPFrom: "noreply@example",
			Mode:     "smtp",
		},
		MQTT: config.MQTTConfig{Addr: ":1883", WSAddr: ":8083"},
	}
	raw, err := marshalConfigYAML(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte{'\t'}) {
		t.Fatalf("generated yaml contains tabs:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte("\n  driver:")) {
		t.Fatalf("expected 2-space indent:\n%s", raw)
	}
	if !bytes.Contains(raw, []byte("mode: smtp")) {
		t.Fatalf("missing email.mode:\n%s", raw)
	}

	keys := []string{
		"EMAIL_MODE", "EMAIL_SMTP_HOST", "SMTP_HOST", "EMAIL_SMTP_PORT", "SMTP_PORT",
		"DATABASE_DRIVER", "DATABASE_DSN", "AUTH_JWT_SECRET", "JWT_SECRET",
	}
	for _, k := range keys {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := config.LoadYAMLFile(path, true); !ok || err != nil {
		t.Fatalf("load generated yaml: ok=%v err=%v", ok, err)
	}
	got := config.Load()
	if got.Email.EffectiveMode() != "smtp" {
		t.Fatalf("effective mode=%q (mode=%q host=%q)", got.Email.EffectiveMode(), got.Email.Mode, got.Email.SMTPHost)
	}
	if got.Email.SMTPHost != "smtp.example" || got.Database.Driver != "sqlite" {
		t.Fatalf("host=%q driver=%q", got.Email.SMTPHost, got.Database.Driver)
	}
}

func TestMarshalConfigYAMLInfersSMTPMode(t *testing.T) {
	raw, err := marshalConfigYAML(config.Config{
		Email: config.EmailConfig{SMTPHost: "smtp.example", SMTPPort: 587},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "mode: smtp") {
		t.Fatalf("host without mode must write mode: smtp:\n%s", raw)
	}
}
