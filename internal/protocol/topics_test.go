package protocol

import "testing"

func TestDetectStyle(t *testing.T) {
	cases := []struct {
		topic string
		want  string
	}{
		{PrefixFull + "esp/cam/" + DirUp + "/" + UpTelemetry, StyleFull},
		{PrefixFull + "esp/cam/" + DirDown + "/#", StyleFull},
		{PrefixShort + ShortTelemetry, StyleShort},
		{PrefixShort + "#", StyleShort},
		{PrefixShort + ShortRPCRequest, StyleShort},
		{PrefixShort + ShortStatus, StyleShort},
		{ShortMe(ShortTelemetry), StyleShort},
		{PrefixShort + "me/telemetry", StyleShort},
		{PrefixShort + "dev/me/rpc/request/+", StyleShort},
		{PrefixShort + "devices/me/attributes", StyleShort},
		{"platform/v1/events/x", ""},
		{"", ""},
		{"$SYS/broker/clients", ""},
	}
	for _, c := range cases {
		if got := DetectStyle(c.topic); got != c.want {
			t.Errorf("DetectStyle(%q)=%q want %q", c.topic, got, c.want)
		}
	}
}

func TestNormalizeCapOp(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{ShortTelemetry, KindTelemetry},
		{UpTelemetry, KindTelemetry},
		{"telemetry", KindTelemetry},
		{"me/" + ShortTelemetry, KindTelemetry},
		{"me/telemetry", KindTelemetry},
		{ShortAttrRequest, KindAttrRequest},
		{"me/attributes", KindAttrReport},
		{"me/attributes/request/1", KindAttrRequest},
		{"me/rpc/response/9", KindRPCResponse},
		{"dev/me/rpc/request/9", KindRPCRequest},
		{ShortStatus, KindStatus},
		{UpStatus, KindStatus},
		{ShortOTAProgress, KindOTAProgress},
		{UpOTAProgress, KindOTAProgress},
	}
	for _, c := range cases {
		if got := NormalizeCapOp(c.in); got != c.want {
			t.Errorf("NormalizeCapOp(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestDownTopic(t *testing.T) {
	if got := DownTopic(StyleFull, "p", "d", CapRPC, OpRequest); got != "iot/v1/p/d/down/rpc/request" {
		t.Fatal(got)
	}
	if got := DownTopic(StyleShort, "p", "d", CapAttributes, OpUpdate); got != "v1/me/a/upd" {
		t.Fatal(got)
	}
	if got := DownTopic(StyleShort, "p", "d", CapOTA, OpUpdate); got != ShortMe(ShortOTAUpdate) {
		t.Fatal(got)
	}
}

func TestRoots(t *testing.T) {
	if FullUpRoot("p", "d") != "iot/v1/p/d/up/" {
		t.Fatal(FullUpRoot("p", "d"))
	}
	if RulesEventsRoot("platform/v1") != "platform/v1/events/" {
		t.Fatal(RulesEventsRoot("platform/v1"))
	}
}

func TestValidStyle(t *testing.T) {
	if !ValidStyle(StyleFull) {
		t.Error("full should be valid")
	}
	if !ValidStyle(StyleShort) {
		t.Error("short should be valid")
	}
	if ValidStyle("invalid") {
		t.Error("invalid should not be valid")
	}
	if ValidStyle("") {
		t.Error("empty should not be valid")
	}
}

func TestUnwrapMe(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"devices/me/rpc/request", "rpc/request"},
		{"dev/me/status", "status"},
		{"me/telemetry", "telemetry"},
		{"rpc/request", "rpc/request"}, // no me prefix
		{"", ""},
	}
	for _, c := range cases {
		if got := UnwrapMe(c.in); got != c.want {
			t.Errorf("UnwrapMe(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestShortMe(t *testing.T) {
	if got := ShortMe(""); got != "v1/me" {
		t.Errorf("ShortMe('')=%q want 'v1/me'", got)
	}
	if got := ShortMe("t"); got != "v1/me/t" {
		t.Errorf("ShortMe('t')=%q want 'v1/me/t'", got)
	}
}

func TestRulesAccountRoots(t *testing.T) {
	if got := RulesEventsRootForAccount("platform/v1", "acme"); got != "platform/v1/events/acme/" {
		t.Errorf("got %q", got)
	}
	if got := RulesCommandsRootForAccount("platform/v1", "acme"); got != "platform/v1/commands/acme/" {
		t.Errorf("got %q", got)
	}
	// Trailing slash in prefix should be trimmed
	if got := RulesEventsRootForAccount("platform/v1/", "acme"); got != "platform/v1/events/acme/" {
		t.Errorf("got %q", got)
	}
}

func TestDownTopicFull(t *testing.T) {
	cases := []struct {
		product, device, cap, op string
		want                     string
	}{
		{"p", "d", CapRPC, OpRequest, "iot/v1/p/d/down/rpc/request"},
		{"p", "d", CapRPC, OpResponse, "iot/v1/p/d/down/rpc/response"},
		{"p", "d", CapAttributes, OpUpdate, "iot/v1/p/d/down/attributes/update"},
		{"p", "d", CapOTA, OpUpdate, "iot/v1/p/d/down/ota/update"},
		{"p", "d", CapOTA, OpCancel, "iot/v1/p/d/down/ota/cancel"},
		{"p", "d", CapNTP, OpResponse, "iot/v1/p/d/down/ntp/response"},
		{"p", "d", "custom", "", "iot/v1/p/d/down/custom"},
	}
	for _, c := range cases {
		got := DownTopic(StyleFull, c.product, c.device, c.cap, c.op)
		if got != c.want {
			t.Errorf("DownTopic(full,%s,%s,%s,%s)=%q want %q", c.product, c.device, c.cap, c.op, got, c.want)
		}
	}
}

func TestDownTopicShort(t *testing.T) {
	cases := []struct {
		cap, op string
		want    string
	}{
		{CapAttributes, OpResponse, "v1/me/a/res"},
		{CapAttributes, OpUpdate, "v1/me/a/upd"},
		{CapRPC, OpRequest, "v1/me/r/req"},
		{CapRPC, OpResponse, "v1/me/r/cres"},
		{CapNTP, OpResponse, "v1/me/n/res"},
		{CapOTA, OpUpdate, "v1/me/o/upd"},
		{CapOTA, OpCancel, "v1/me/o/can"},
		{"custom", "op", "v1/me/custom"},
	}
	for _, c := range cases {
		got := DownTopic(StyleShort, "p", "d", c.cap, c.op)
		if got != c.want {
			t.Errorf("DownTopic(short,%s,%s)=%q want %q", c.cap, c.op, got, c.want)
		}
	}
}
