package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadYAMLFileErrors(t *testing.T) {
	dir := t.TempDir()

	if ok, err := LoadYAMLFile(filepath.Join(dir, "missing.yml"), false); ok || err != nil {
		t.Fatalf("missing file: ok=%v err=%v, want false/nil", ok, err)
	}

	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("platform:\n  broker_host_public: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := LoadYAMLFile(bad, false); ok || err == nil {
		t.Fatalf("invalid yaml: ok=%v err=%v, want false/error", ok, err)
	}

	good := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(good, []byte("platform:\n  broker_host_public: mqtt.example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLATFORM_BROKER_HOST_PUBLIC", "")
	os.Unsetenv("PLATFORM_BROKER_HOST_PUBLIC")
	if ok, err := LoadYAMLFile(good, false); !ok || err != nil {
		t.Fatalf("valid .yaml: ok=%v err=%v", ok, err)
	}
	if got := os.Getenv("PLATFORM_BROKER_HOST_PUBLIC"); got != "mqtt.example.com" {
		t.Fatalf("env = %q", got)
	}
}

func TestFlattenServerSection(t *testing.T) {
	flat := map[string]string{}
	flatten("", map[string]any{
		"server": map[string]any{
			"http_addr":       ":9090",
			"public_base_url": "http://localhost:8081",
			"platform_name":   "YamlIoT",
		},
		"database": map[string]any{"sqlite_path": "./tmp.db"},
		"auth":     map[string]any{"jwt_secret": "s"},
		"email":    map[string]any{"mode": "smtp", "smtp_host": "smtp.example"},
		"mqtt":     map[string]any{"addr": ":1884", "broker_host_public": "192.0.2.1"},
	}, flat)
	want := map[string]string{
		"SERVER_HTTP_ADDR":        ":9090",
		"SERVER_PUBLIC_BASE_URL":  "http://localhost:8081",
		"SERVER_PLATFORM_NAME":    "YamlIoT",
		"DATABASE_SQLITE_PATH":    "./tmp.db",
		"AUTH_JWT_SECRET":         "s",
		"EMAIL_MODE":              "smtp",
		"EMAIL_SMTP_HOST":         "smtp.example",
		"MQTT_ADDR":               ":1884",
		"MQTT_BROKER_HOST_PUBLIC": "192.0.2.1",
	}
	for k, v := range want {
		if flat[k] != v {
			t.Errorf("%s=%q want %q", k, flat[k], v)
		}
	}
}

func TestLoadYAMLServerPublicBaseURL(t *testing.T) {
	keys := []string{
		"PUBLIC_BASE_URL", "SERVER_PUBLIC_BASE_URL", "PLATFORM_PUBLIC_BASE_URL",
		"HTTP_ADDR", "SERVER_HTTP_ADDR",
		"BROKER_HOST_PUBLIC", "PLATFORM_BROKER_HOST_PUBLIC", "MQTT_BROKER_HOST_PUBLIC", "MQTT_HOST_PUBLIC",
	}
	saved := map[string]*string{}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			vv := v
			saved[k] = &vv
		}
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			_ = os.Unsetenv(k)
		}
		for k, v := range saved {
			_ = os.Setenv(k, *v)
		}
	})

	path := filepath.Join(t.TempDir(), "config.yml")
	src := `
server:
  http_addr: ":9090"
  public_base_url: "http://localhost:8081"
mqtt:
  broker_host_public: "192.0.2.1"
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	LoadYAML(path)
	cfg := Load()
	if cfg.Platform.PublicBaseURL != "http://localhost:8081" || cfg.HTTP.Addr != ":9090" {
		t.Fatalf("got public=%q http=%q", cfg.Platform.PublicBaseURL, cfg.HTTP.Addr)
	}
	if cfg.Platform.BrokerHostPublic != "192.0.2.1" {
		t.Fatalf("broker=%q", cfg.Platform.BrokerHostPublic)
	}
}

func TestEnvPublicBaseURLWinsOverYAML(t *testing.T) {
	keys := []string{"PUBLIC_BASE_URL", "SERVER_PUBLIC_BASE_URL", "PLATFORM_PUBLIC_BASE_URL"}
	saved := map[string]*string{}
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			vv := v
			saved[k] = &vv
		}
		_ = os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for _, k := range keys {
			_ = os.Unsetenv(k)
		}
		for k, v := range saved {
			_ = os.Setenv(k, *v)
		}
	})
	t.Setenv("PUBLIC_BASE_URL", "http://from-env:1")
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("server:\n  public_base_url: \"http://from-yaml:8081\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	LoadYAML(path)
	if got := Load().Platform.PublicBaseURL; got != "http://from-env:1" {
		t.Fatalf("env must win, got %q", got)
	}
}
