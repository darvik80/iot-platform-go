package config

import (
	"path/filepath"
	"testing"
)

func TestTimezoneOrUTC(t *testing.T) {
	if (Config{}).TimezoneOrUTC() != "UTC" {
		t.Fatal("empty → UTC")
	}
	if (Config{Timezone: "Asia/Bangkok"}).TimezoneOrUTC() != "Asia/Bangkok" {
		t.Fatal("valid IANA")
	}
	if (Config{Timezone: "Not/AZone"}).TimezoneOrUTC() != "UTC" {
		t.Fatal("invalid → UTC")
	}
}

func TestSlogLevel(t *testing.T) {
	if (Config{LogLevel: "debug"}).SlogLevel().String() != "DEBUG" {
		t.Fatal("debug")
	}
	if (Config{}).SlogLevel().String() != "INFO" {
		t.Fatal("default info")
	}
}

func TestMQTTPublicPort(t *testing.T) {
	cases := map[string]int{
		"":               1883,
		":1883":          1883,
		"0.0.0.0:1883":   1883,
		"tcp://:1884":    1884,
		"mqtt://:1885":   1885,
		"127.0.0.1:1886": 1886,
		"1887":           1887,
	}
	for in, want := range cases {
		c := Config{MQTTAddr: in}
		if got := c.MQTTPublicPort(); got != want {
			t.Errorf("MQTTPublicPort(%q)=%d want %d", in, got, want)
		}
	}
}

func TestMQTTWSPublicURL(t *testing.T) {
	c := Config{MQTTWSAddr: ":8083", BrokerHostPublic: "192.168.1.22"}
	if !c.MQTTWSEnabled() || c.MQTTWSPublicPort() != 8083 {
		t.Fatalf("enabled/port %+v", c)
	}
	if got := c.MQTTWSPublicURL(); got != "ws://192.168.1.22:8083/" {
		t.Fatalf("url=%q", got)
	}
	off := Config{MQTTWSAddr: "off", BrokerHostPublic: "x"}
	if off.MQTTWSEnabled() || off.MQTTWSPublicURL() != "" {
		t.Fatal("off should disable")
	}
}

func TestParseListenPort(t *testing.T) {
	cases := map[string]int{
		"":               1883,
		":1883":          1883,
		"0.0.0.0:1883":   1883,
		"tcp://:1884":    1884,
		"mqtt://:1885":   1885,
		"ws://:8083":     8083,
		"wss://:8084":    8084,
		"127.0.0.1:1886": 1886,
		"1887":           1887,
		"invalid":        1883,
		":abc":           1883,
		":0":             1883,
		":-1":            1883,
	}
	for in, want := range cases {
		if got := parseListenPort(in, 1883); got != want {
			t.Errorf("parseListenPort(%q)=%d want %d", in, got, want)
		}
	}
}

func TestFirmwareDir(t *testing.T) {
	// With OTADir set
	c := Config{OTADir: "/custom/firmware"}
	if got := c.FirmwareDir(); got != "/custom/firmware" {
		t.Errorf("FirmwareDir() = %q, want '/custom/firmware'", got)
	}

	// Without OTADir (should use DataDir/firmware) - use filepath.Join for cross-platform
	c = Config{DataDir: "data"}
	want := filepath.Join("data", "firmware")
	if got := c.FirmwareDir(); got != want {
		t.Errorf("FirmwareDir() = %q, want %q", got, want)
	}

	// Empty OTADir with spaces (should use DataDir)
	c = Config{OTADir: "   ", DataDir: "data"}
	if got := c.FirmwareDir(); got != want {
		t.Errorf("FirmwareDir() = %q, want %q", got, want)
	}
}

func TestFirmwarePublicURL(t *testing.T) {
	c := Config{PublicBaseURL: "https://iot.example.com"}
	got := c.FirmwarePublicURL("abc123")
	want := "https://iot.example.com/ota/fw/abc123.bin"
	if got != want {
		t.Errorf("FirmwarePublicURL() = %q, want %q", got, want)
	}

	// Trailing slash should be trimmed
	c = Config{PublicBaseURL: "https://iot.example.com/"}
	got = c.FirmwarePublicURL("token")
	want = "https://iot.example.com/ota/fw/token.bin"
	if got != want {
		t.Errorf("FirmwarePublicURL() = %q, want %q", got, want)
	}
}

func TestGoogleCallbackURL(t *testing.T) {
	// With explicit redirect URL
	c := Config{GoogleRedirectURL: "https://custom.example.com/callback"}
	if got := c.GoogleCallbackURL(); got != "https://custom.example.com/callback" {
		t.Errorf("GoogleCallbackURL() = %q", got)
	}

	// Without redirect URL (should use PublicBaseURL)
	c = Config{PublicBaseURL: "https://iot.example.com"}
	if got := c.GoogleCallbackURL(); got != "https://iot.example.com/api/v1/auth/google/callback" {
		t.Errorf("GoogleCallbackURL() = %q", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("a", "b", "c"); got != "a" {
		t.Errorf("firstNonEmpty('a','b','c') = %q, want 'a'", got)
	}
	if got := firstNonEmpty("", "b", "c"); got != "b" {
		t.Errorf("firstNonEmpty('','b','c') = %q, want 'b'", got)
	}
	if got := firstNonEmpty("", "", "c"); got != "c" {
		t.Errorf("firstNonEmpty('','','c') = %q, want 'c'", got)
	}
	if got := firstNonEmpty("", "", ""); got != "" {
		t.Errorf("firstNonEmpty('','','') = %q, want ''", got)
	}
	if got := firstNonEmpty("  ", "b"); got != "b" {
		t.Errorf("firstNonEmpty('  ','b') = %q, want 'b'", got)
	}
}

func TestGoogleEnabled(t *testing.T) {
	c := Config{GoogleClientID: "id", GoogleClientSecret: "secret"}
	if !c.GoogleEnabled() {
		t.Error("should be enabled with both ID and Secret")
	}
	c = Config{GoogleClientID: "id"}
	if c.GoogleEnabled() {
		t.Error("should be disabled without Secret")
	}
	c = Config{GoogleClientSecret: "secret"}
	if c.GoogleEnabled() {
		t.Error("should be disabled without ID")
	}
	c = Config{}
	if c.GoogleEnabled() {
		t.Error("should be disabled with neither")
	}
}

func TestMQTTTLSEnabled(t *testing.T) {
	c := Config{MQTTTLSCert: "cert.pem", MQTTTLSKey: "key.pem"}
	if !c.MQTTTLSEnabled() {
		t.Error("should be enabled with cert and key")
	}
	c = Config{MQTTTLSCert: "cert.pem"}
	if c.MQTTTLSEnabled() {
		t.Error("should be disabled without key")
	}
	c = Config{}
	if c.MQTTTLSEnabled() {
		t.Error("should be disabled with neither")
	}
}

func TestMQTTTLSPublicURL(t *testing.T) {
	// MQTTTLSEnabled() requires both cert AND key
	c := Config{MQTTTLSAddr: ":8883", MQTTTLSCert: "cert.pem", MQTTTLSKey: "key.pem", BrokerHostPublic: "mqtt.example.com"}
	if !c.MQTTTLSEnabled() {
		t.Fatal("should be enabled")
	}
	// Note: MQTTTLSPublicURL does NOT add trailing slash (unlike MQTTWSPublicURL)
	if got := c.MQTTTLSPublicURL(); got != "mqtts://mqtt.example.com:8883" {
		t.Errorf("MQTTTLSPublicURL() = %q", got)
	}

	// Disabled TLS
	c = Config{}
	if got := c.MQTTTLSPublicURL(); got != "" {
		t.Errorf("disabled should return empty: %q", got)
	}
}

func TestMQTTPublicURL(t *testing.T) {
	c := Config{MQTTAddr: ":1883", BrokerHostPublic: "mqtt.example.com"}
	if got := c.MQTTPublicURL(); got != "mqtt://mqtt.example.com:1883" {
		t.Errorf("MQTTPublicURL() = %q", got)
	}
}
