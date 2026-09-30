package ws

import (
	"net/http/httptest"
	"testing"
)

func TestOriginAllowed(t *testing.T) {
	h := NewHub()
	h.AllowedHosts = []string{"iot.example.com"}
	cases := []struct {
		host, origin string
		ok           bool
	}{
		{"platform:8080", "", true},
		{"platform:8080", "http://platform:8080", true},
		{"platform:8080", "http://localhost:5173", true},
		{"platform:8080", "https://iot.example.com", true},
		{"platform:8080", "https://evil.example.com", false},
		{"platform:8080", "http://platform:9999", false},
		{"platform:8080", "null", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://"+c.host+"/api/v1/ws", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := h.OriginAllowed(r); got != c.ok {
			t.Errorf("host=%s origin=%q: allowed=%v want %v", c.host, c.origin, got, c.ok)
		}
	}
}
