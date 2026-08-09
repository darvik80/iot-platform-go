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
