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
		c := Config{MQTT: MQTTConfig{Addr: in}}
		if got := c.MQTT.PublicPort(); got != want {
			t.Errorf("MQTTPublicPort(%q)=%d want %d", in, got, want)
		}
	}
}

func TestMQTTWSPublicURL(t *testing.T) {
	c := Config{MQTT: MQTTConfig{WSAddr: ":8083"}, Platform: PlatformConfig{BrokerHostPublic: "192.168.1.22"}}
	if !c.MQTT.WSEnabled() || c.MQTT.WSPublicPort() != 8083 {
		t.Fatalf("enabled/port %+v", c)
	}
	if got := c.MQTT.WSPublicURL(c.Platform.BrokerHostPublic); got != "ws://192.168.1.22:8083/" {
		t.Fatalf("url=%q", got)
	}
	off := Config{MQTT: MQTTConfig{WSAddr: "off"}, Platform: PlatformConfig{BrokerHostPublic: "x"}}
	if off.MQTT.WSEnabled() || off.MQTT.WSPublicURL(off.Platform.BrokerHostPublic) != "" {
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
	c := Config{OTA: OTAConfig{Dir: "/custom/firmware"}}
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
	c = Config{OTA: OTAConfig{Dir: "   "}, DataDir: "data"}
	if got := c.FirmwareDir(); got != want {
		t.Errorf("FirmwareDir() = %q, want %q", got, want)
	}
}

func TestFirmwarePublicURL(t *testing.T) {
	c := Config{Platform: PlatformConfig{PublicBaseURL: "https://iot.example.com"}}
	got := c.FirmwarePublicURL("abc123")
	want := "https://iot.example.com/ota/fw/abc123.bin"
	if got != want {
		t.Errorf("FirmwarePublicURL() = %q, want %q", got, want)
	}

	// Trailing slash should be trimmed
	c = Config{Platform: PlatformConfig{PublicBaseURL: "https://iot.example.com/"}}
	got = c.FirmwarePublicURL("token")
	want = "https://iot.example.com/ota/fw/token.bin"
	if got != want {
		t.Errorf("FirmwarePublicURL() = %q, want %q", got, want)
	}
}

func TestGoogleCallbackURL(t *testing.T) {
	// With explicit redirect URL
	c := Config{Google: GoogleConfig{RedirectURL: "https://custom.example.com/callback"}}
	if got := c.GoogleCallbackURL(); got != "https://custom.example.com/callback" {
		t.Errorf("GoogleCallbackURL() = %q", got)
	}

	// Without redirect URL (should use PublicBaseURL)
	c = Config{Platform: PlatformConfig{PublicBaseURL: "https://iot.example.com"}}
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
	c := Config{Google: GoogleConfig{ClientID: "id", ClientSecret: "secret"}}
	if !c.GoogleEnabled() {
		t.Error("should be enabled with both ID and Secret")
	}
	c = Config{Google: GoogleConfig{ClientID: "id"}}
	if c.GoogleEnabled() {
		t.Error("should be disabled without Secret")
	}
	c = Config{Google: GoogleConfig{ClientSecret: "secret"}}
	if c.GoogleEnabled() {
		t.Error("should be disabled without ID")
	}
	c = Config{}
	if c.GoogleEnabled() {
		t.Error("should be disabled with neither")
	}
}

func TestMQTTTLSEnabled(t *testing.T) {
	c := Config{MQTT: MQTTConfig{TLSCert: "cert.pem", TLSKey: "key.pem"}}
	if !c.MQTT.TLSEnabled() {
		t.Error("should be enabled with cert and key")
	}
	c = Config{MQTT: MQTTConfig{TLSCert: "cert.pem"}}
	if c.MQTT.TLSEnabled() {
		t.Error("should be disabled without key")
	}
	c = Config{}
	if c.MQTT.TLSEnabled() {
		t.Error("should be disabled with neither")
	}
}

func TestMQTTTLSPublicURL(t *testing.T) {
	// MQTTTLSEnabled() requires both cert AND key
	c := Config{MQTT: MQTTConfig{TLSAddr: ":8883", TLSCert: "cert.pem", TLSKey: "key.pem"}, Platform: PlatformConfig{BrokerHostPublic: "mqtt.example.com"}}
	if !c.MQTT.TLSEnabled() {
		t.Fatal("should be enabled")
	}
	// Note: TLSPublicURL does NOT add trailing slash (unlike WSPublicURL)
	if got := c.MQTT.TLSPublicURL(c.Platform.BrokerHostPublic); got != "mqtts://mqtt.example.com:8883" {
		t.Errorf("MQTTTLSPublicURL() = %q", got)
	}

	// Disabled TLS
	c = Config{}
	if got := c.MQTT.TLSPublicURL(""); got != "" {
		t.Errorf("disabled should return empty: %q", got)
	}
}

func TestMQTTPublicURL(t *testing.T) {
	c := Config{MQTT: MQTTConfig{Addr: ":1883"}, Platform: PlatformConfig{BrokerHostPublic: "mqtt.example.com"}}
	if got := c.MQTT.PublicURL(c.Platform.BrokerHostPublic); got != "mqtt://mqtt.example.com:1883" {
		t.Errorf("MQTTPublicURL() = %q", got)
	}
}
