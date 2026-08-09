package mqtt

import (
	"testing"

	"crearts-iot-platform/internal/protocol"
)

func TestTopicUnder(t *testing.T) {
	cases := []struct {
		topic, root string
		ok          bool
	}{
		{"platform/v1/events/#", protocol.RulesEventsRoot("platform/v1"), true},
		{"platform/v1/events/+/+/telemetry", protocol.RulesEventsRoot("platform/v1"), true},
		{"platform/v1/events/esp/cam/telemetry", protocol.RulesEventsRoot("platform/v1"), true},
		{"platform/v1/#", protocol.RulesEventsRoot("platform/v1"), false},
		{"platform/v1/commands/esp/cam/rpc", protocol.RulesEventsRoot("platform/v1"), false},
		{"platform/v1/commands/#", protocol.RulesCommandsRoot("platform/v1"), true},
		{protocol.PrefixShort + "#", protocol.PrefixShort, true},
		{protocol.PrefixShort + protocol.ShortTelemetry, protocol.PrefixShort, true},
		{protocol.PrefixShort + "+", protocol.PrefixShort, true},
		{"#", protocol.PrefixShort, false},
		{"+", protocol.PrefixShort, false},
		{protocol.FullUpRoot("p", "d") + "#", protocol.FullUpRoot("p", "d"), true},
		{protocol.FullUpRoot("p", "d") + protocol.UpTelemetry, protocol.FullUpRoot("p", "d"), true},
		{protocol.FullDownRoot("p", "d") + "rpc/request", protocol.FullUpRoot("p", "d"), false},
		{protocol.FullUpRoot("other", "d") + protocol.ShortTelemetry, protocol.FullUpRoot("p", "d"), false},
	}
	for _, c := range cases {
		if got := topicUnder(c.topic, c.root); got != c.ok {
			t.Errorf("topicUnder(%q, %q)=%v want %v", c.topic, c.root, got, c.ok)
		}
	}
}
