package ws

import "testing"

func TestBroadcastScopedByAccount(t *testing.T) {
	h := NewHub()
	a := &client{send: make(chan []byte, 4), accountID: 1}
	b := &client{send: make(chan []byte, 4), accountID: 2}
	admin := &client{send: make(chan []byte, 4), accountID: 3, global: true}
	for _, c := range []*client{a, b, admin} {
		h.clients[c] = struct{}{}
	}

	h.Broadcast(1, "telemetry", nil)
	h.Broadcast(0, "mqtt", nil)

	if len(a.send) != 1 {
		t.Errorf("account 1 client got %d messages, want 1", len(a.send))
	}
	if len(b.send) != 0 {
		t.Errorf("account 2 client got %d messages, want 0", len(b.send))
	}
	if len(admin.send) != 1 {
		t.Errorf("platform admin got %d messages, want 1 (platform-wide only)", len(admin.send))
	}
}
