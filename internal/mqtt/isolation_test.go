package mqtt

import (
	"testing"

	"cogitor-iot-platform/internal/config"
)

func TestFilterMatches(t *testing.T) {
	cases := []struct {
		filter, topic string
		ok            bool
	}{
		{"v1/me/r/req", "v1/me/r/req", true},
		{"v1/me/#", "v1/me/r/req", true},
		{"v1/#", "v1/me/a/upd", true},
		{"v1/me/+/req", "v1/me/r/req", true},
		{"v1/me/+", "v1/me/r/req", false},
		{"v1/me/r/req", "v1/me/r/res", false},
		{"v1/me/r", "v1/me/r/req", false},
		{"#", "v1/me/r/req", true},
		{"#", "$SYS/x", false},
		{"v1/me/#", "v1/me", true},
	}
	for _, c := range cases {
		if got := filterMatches(c.filter, c.topic); got != c.ok {
			t.Errorf("filterMatches(%q, %q)=%v want %v", c.filter, c.topic, got, c.ok)
		}
	}
}

func TestPluginACLScopedToOwnAccount(t *testing.T) {
	cfg := config.Config{}
	cfg.Rules.Prefix = "platform/v1"
	h := &platHook{b: &Broker{cfg: cfg}}
	user := "extplg_acme_telegram-bot"
	cases := []struct {
		topic string
		write bool
		ok    bool
	}{
		{"platform/v1/commands/acme/esp32-cam/cam-001/rpc", true, true},
		{"platform/v1/commands/beta/esp32-cam/cam-001/rpc", true, false},
		{"platform/v1/commands/esp32-cam/cam-001/rpc", true, false},
		{"platform/v1/events/acme/#", false, true},
		{"platform/v1/events/beta/#", false, false},
		{"platform/v1/events/#", false, false},
		{"v1/me/r/req", true, false},
		{"v1/me/#", false, false},
	}
	for _, c := range cases {
		if got := h.aclOK(user, c.topic, c.write); got != c.ok {
			t.Errorf("aclOK(%q, %q, write=%v)=%v want %v", user, c.topic, c.write, got, c.ok)
		}
	}
}
