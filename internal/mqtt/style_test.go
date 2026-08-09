package mqtt

import "testing"

func TestDetectTopicStyle(t *testing.T) {
	cases := []struct {
		topic string
		want  string
	}{
		{"iot/v1/esp/cam/up/telemetry/data", "full"},
		{"iot/v1/esp/cam/down/#", "full"},
		{"v1/t", "short"},
		{"v1/#", "short"},
		{"v1/r/req", "short"},
		{"v1/s", "short"},
		{"v1/me/t", "short"},
		{"v1/me/telemetry", "short"},
		{"v1/dev/me/rpc/request/+", "short"},
		{"v1/devices/me/attributes", "short"},
		{"platform/v1/events/x", ""},
		{"", ""},
		{"$SYS/broker/clients", ""},
	}
	for _, c := range cases {
		if got := DetectTopicStyle(c.topic); got != c.want {
			t.Errorf("DetectTopicStyle(%q)=%q want %q", c.topic, got, c.want)
		}
	}
}

func TestShortCapOp(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"t", "t"},
		{"a/req", "a/req"},
		{"me/t", "t"},
		{"me/telemetry", "t"},
		{"me/attributes", "a"},
		{"me/attributes/request/1", "a/req"},
		{"me/rpc/response/9", "r/res"},
		{"dev/me/rpc/request/9", "r/creq"},
		{"me/telemetry", "t"},
	}
	for _, c := range cases {
		if got := shortCapOp(c.in); got != c.want {
			t.Errorf("shortCapOp(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
