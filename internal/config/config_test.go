package config

import "testing"

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
