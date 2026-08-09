package mqtt

import "testing"

func TestTopicUnder(t *testing.T) {
	cases := []struct {
		topic string
		root  string
		ok    bool
	}{
		{"platform/v1/events/#", "platform/v1/events/", true},
		{"platform/v1/events/+/+/telemetry", "platform/v1/events/", true},
		{"platform/v1/events/esp/cam/telemetry", "platform/v1/events/", true},
		{"platform/v1/#", "platform/v1/events/", false},
		{"platform/v1/commands/esp/cam/rpc", "platform/v1/events/", false},
		{"platform/v1/commands/#", "platform/v1/commands/", true},
		{"v1/#", "v1/", true},
		{"v1/t", "v1/", true},
		{"v1/+", "v1/", true},
		{"#", "v1/", false},
		{"+", "v1/", false},
		{"iot/v1/p/d/up/#", "iot/v1/p/d/up/", true},
		{"iot/v1/p/d/up/telemetry/data", "iot/v1/p/d/up/", true},
		{"iot/v1/p/d/down/rpc/request", "iot/v1/p/d/up/", false},
		{"iot/v1/other/d/up/t", "iot/v1/p/d/up/", false},
	}
	for _, c := range cases {
		if got := topicUnder(c.topic, c.root); got != c.ok {
			t.Errorf("topicUnder(%q, %q)=%v want %v", c.topic, c.root, got, c.ok)
		}
	}
}
