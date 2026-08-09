package mqtt

import "testing"

func TestNormalizeMQTTAddr(t *testing.T) {
	cases := map[string]string{
		"":               "0.0.0.0:1883",
		":1883":          "0.0.0.0:1883",
		"tcp://:1883":    "0.0.0.0:1883",
		"mqtt://:1884":   "0.0.0.0:1884",
		"127.0.0.1:1883": "127.0.0.1:1883",
		"1883":           "0.0.0.0:1883",
	}
	for in, want := range cases {
		if got := normalizeMQTTAddr(in); got != want {
			t.Errorf("normalizeMQTTAddr(%q)=%q want %q", in, got, want)
		}
	}
}
