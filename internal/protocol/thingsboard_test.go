package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"cogitor-iot-platform/internal/models"
)

func tbTestDialect() Dialect { return DialectFor(StyleThingsBoard) }

func TestTBParseUplink(t *testing.T) {
	cases := []struct {
		topic string
		kind  string
		aux   string
	}{
		{tbTelemetryTopic, KindTelemetry, ""},
		{tbAttributesTopic, KindAttrReport, ""},
		{tbAttrRequestTopic + "43", KindAttrRequest, "43"},
		{tbAttrRequestTopic + "led", KindAttrRequest, "led"},
		{tbRPCResponseV1 + "7", KindRPCResponse, "7"},
		{tbRPCResponseV2 + "7", KindRPCResponse, "7"},
	}
	for _, c := range cases {
		up, ok := thingsboardDialect{}.ParseUplink(c.topic)
		if !ok {
			t.Errorf("ParseUplink(%q) rejected", c.topic)
			continue
		}
		if up.Kind != c.kind || up.Aux != c.aux {
			t.Errorf("ParseUplink(%q)={%s %s} want {%s %s}", c.topic, up.Kind, up.Aux, c.kind, c.aux)
		}
		if up.ProductID != "" || up.DeviceID != "" {
			t.Errorf("ParseUplink(%q) carries identity %q/%q: TB topics are session-scoped", c.topic, up.ProductID, up.DeviceID)
		}
	}
}

func TestTBParseUplinkRejects(t *testing.T) {
	for _, topic := range []string{
		"v1/devices/me/attributes/response/43", // downlink shape
		"v1/devices/me/attributes/request/",    // empty id
		"v1/devices/me/attributes/request/4/3", // nested id
		"v1/devices/me/attributes/request/+",   // wildcard id
		"v1/devices/me/attributes/request/#",
		"v1/devices/me/rpc/request/7",          // downlink shape
		"v1/devices/me/rpc/response/",          // empty id
		"v1/devices/me/rpc/response/1/2",       // nested id
		"v2/devices/me/rpc/request/7",          // downlink shape, v2
		"v2/devices/me/telemetry",              // v2 telemetry is protobuf, out of scope
		"v2/devices/me/attributes",             // v2 attributes only exist as requests
		"v1/devices/me",
		"v1/devices/me/",
		"iot/v1/p/d/up/telemetry/data", // full style
		"v1/me/t",                      // short style
		"$SYS/broker/uptime",
		"random/topic",
	} {
		if _, ok := (thingsboardDialect{}).ParseUplink(topic); ok {
			t.Errorf("ParseUplink(%q) accepted, want rejected", topic)
		}
	}
}

// TB topics nest inside the short prefix (v1/…), so the registry must try the
// thingsboard dialect first.
func TestTBTopicPriorityOverShort(t *testing.T) {
	for _, topic := range []string{tbTelemetryTopic, tbAttributesTopic, tbRPCResponseV1 + "1", tbRPCResponseV2 + "1"} {
		up, dl, ok := ParseUplinkTopic(topic)
		if !ok || dl.Name() != StyleThingsBoard {
			t.Errorf("ParseUplinkTopic(%q) resolved to %v, want thingsboard", topic, dl)
			continue
		}
		if up.ProductID != "" || up.DeviceID != "" {
			t.Errorf("ParseUplinkTopic(%q) carries identity", topic)
		}
	}
}

func TestTBID(t *testing.T) {
	cases := []struct {
		s  string
		ok bool
	}{
		{"43", true},
		{"led-color_1", true},
		{"", false},
		{"4/3", false},
		{"+", false},
		{"#", false},
	}
	for _, c := range cases {
		if got := tbID(c.s); got != c.ok {
			t.Errorf("tbID(%q)=%v want %v", c.s, got, c.ok)
		}
	}
}

func TestTBUplinkPassthrough(t *testing.T) {
	for _, kind := range []string{KindTelemetry, KindAttrReport} {
		got, err := thingsboardDialect{}.Uplink(kind, "", []byte(`{"temp":22}`))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != `{"temp":22}` {
			t.Errorf("kind %s: got %s, want passthrough", kind, got)
		}
	}
}

func TestTBAttrRequestToInternal(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"shared array", `{"sharedKeys":["led_color","mode"]}`, `{"id":"7","desired":["led_color","mode"]}`},
		{"client array", `{"clientKeys":["fw"]}`, `{"id":"7","reported":["fw"]}`},
		{"comma string form", `{"sharedKeys":"a , b ,,","clientKeys":"c,d"}`, `{"id":"7","reported":["c","d"],"desired":["a","b"]}`},
		{"no keys asks for all shared", `{}`, `{"id":"7","desired":["*"]}`},
		{"null keys ask for all shared", `{"sharedKeys":null,"clientKeys":null}`, `{"id":"7","desired":["*"]}`},
		{"empty arrays ask for all shared", `{"sharedKeys":[],"clientKeys":[]}`, `{"id":"7","desired":["*"]}`},
		{"internal shape passes through", `{"id":"x","reported":[],"desired":[]}`, `{"id":"x","reported":[],"desired":[]}`},
		{"internal shape reported only", `{"reported":["x"]}`, `{"reported":["x"]}`},
		{"non-object tolerated", `hello`, `{"id":"7","desired":["*"]}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := thingsboardDialect{}.Uplink(KindAttrRequest, "7", []byte(c.payload))
			if err != nil {
				t.Fatal(err)
			}
			assertJSON(t, got, c.want)
		})
	}
}

func TestTBRPCResponseToInternal(t *testing.T) {
	cases := []struct {
		name    string
		aux     string
		payload string
		want    string
	}{
		{"numeric id", "5", `{"result":"rebooting"}`, `{"id":5,"code":0,"data":{"result":"rebooting"}}`},
		{"string id", "abc", `{"x":1}`, `{"id":"abc","code":0,"data":{"x":1}}`},
		{"scalar data", "5", `"ok"`, `{"id":5,"code":0,"data":"ok"}`},
		{"internal shape passes through", "5", `{"id":5,"code":0,"message":"ok"}`, `{"id":5,"code":0,"message":"ok"}`},
		{"internal shape error code", "5", `{"id":5,"code":500,"message":"boom"}`, `{"id":5,"code":500,"message":"boom"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := thingsboardDialect{}.Uplink(KindRPCResponse, c.aux, []byte(c.payload))
			if err != nil {
				t.Fatal(err)
			}
			assertJSON(t, got, c.want)
		})
	}

	// an empty payload still carries id+code so CompleteRPC can correlate
	got, err := thingsboardDialect{}.Uplink(KindRPCResponse, "5", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"id":5,"code":0,"data":null}`)
}

func TestTBUplinkUnsupported(t *testing.T) {
	if _, err := (thingsboardDialect{}).Uplink(KindEvent, "", []byte(`{}`)); err == nil {
		t.Error("unsupported kind must fail")
	}
}

func TestTBDownlinkAttributePush(t *testing.T) {
	topic, body, err := thingsboardDialect{}.Downlink(nil, CapAttributes, OpUpdate, "123", []byte(`{"temp":30}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != tbAttributesTopic {
		t.Errorf("topic=%q want %q", topic, tbAttributesTopic)
	}
	if string(body) != `{"temp":30}` {
		t.Errorf("body=%s, want passthrough", body)
	}
}

func TestTBDownlinkAttrResponse(t *testing.T) {
	topic, body, err := thingsboardDialect{}.Downlink(nil, CapAttributes, OpResponse, "77", []byte(
		`{"id":"43","desired":{"led_color":"warm"},"reported":{"fw":"1.0"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != tbAttrResponseTopic+"43" {
		t.Errorf("topic=%q want %q43", topic, tbAttrResponseTopic)
	}
	assertJSON(t, body, `{"shared":{"led_color":"warm"},"client":{"fw":"1.0"}}`)

	// numeric payload id, arg id as fallback when the payload carries none
	topic, _, err = thingsboardDialect{}.Downlink(nil, CapAttributes, OpResponse, "77", []byte(`{"id":43,"desired":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != tbAttrResponseTopic+"43" {
		t.Errorf("topic=%q", topic)
	}
	topic, _, err = thingsboardDialect{}.Downlink(nil, CapAttributes, OpResponse, "77", []byte(`{"desired":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != tbAttrResponseTopic+"77" {
		t.Errorf("fallback topic=%q", topic)
	}

	if _, _, err := (thingsboardDialect{}).Downlink(nil, CapAttributes, OpResponse, "", []byte(`{"desired":{}}`)); err == nil {
		t.Error("attr response without id must fail")
	}
	if _, _, err := (thingsboardDialect{}).Downlink(nil, CapAttributes, OpResponse, "1", []byte(`{`)); err == nil {
		t.Error("attr response with invalid JSON must fail")
	}
}

func TestTBDownlinkRPCRequest(t *testing.T) {
	topic, body, err := thingsboardDialect{}.Downlink(nil, CapRPC, OpRequest, "99", []byte(
		`{"id":42,"method":"reboot","params":{"delay":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	// the payload id seeds the topic so the device's response correlates
	if topic != tbRPCRequestV1+"42" {
		t.Errorf("topic=%q want %q42", topic, tbRPCRequestV1)
	}
	assertJSON(t, body, `{"method":"reboot","params":{"delay":5}}`)

	// id from the arg when the payload carries none
	topic, body, err = thingsboardDialect{}.Downlink(nil, CapRPC, OpRequest, "7", []byte(`{"method":"reboot"}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != tbRPCRequestV1+"7" {
		t.Errorf("topic=%q", topic)
	}
	assertJSON(t, body, `{"method":"reboot","params":{}}`)

	// payload id 0 counts as absent
	topic, _, err = thingsboardDialect{}.Downlink(nil, CapRPC, OpRequest, "9", []byte(`{"id":0,"method":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != tbRPCRequestV1+"9" {
		t.Errorf("topic=%q", topic)
	}

	if _, _, err := (thingsboardDialect{}).Downlink(nil, CapRPC, OpRequest, "1", []byte(`{}`)); err == nil {
		t.Error("rpc without method must fail")
	}
	if _, _, err := (thingsboardDialect{}).Downlink(nil, CapRPC, OpRequest, "", []byte(`{"method":"reboot"}`)); err == nil {
		t.Error("rpc without id must fail")
	}
	if _, _, err := (thingsboardDialect{}).Downlink(nil, CapRPC, OpRequest, "1", []byte(`nonsense`)); err == nil {
		t.Error("rpc with invalid JSON must fail")
	}
}

func TestTBDownlinkUnsupported(t *testing.T) {
	if _, _, err := (thingsboardDialect{}).Downlink(nil, CapOTA, OpUpdate, "1", []byte(`{}`)); err == nil {
		t.Error("unsupported downlink must fail")
	}
}

func TestTBRawIDString(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		fallback string
		want     string
	}{
		{"absent", "", "99", "99"},
		{"null", "null", "99", "99"},
		{"string", `"43"`, "99", "43"},
		{"number", `43`, "99", "43"},
		{"float", `43.5`, "99", "43.5"},
		{"empty string falls back", `""`, "99", "99"},
		{"bool falls back", `true`, "99", "99"},
		{"array falls back", `[1]`, "99", "99"},
	}
	for _, c := range cases {
		if got := tbRawIDString(json.RawMessage(c.raw), c.fallback); got != c.want {
			t.Errorf("%s: tbRawIDString(%s)=%q want %q", c.name, c.raw, got, c.want)
		}
	}
}

func TestTBDialectMetadata(t *testing.T) {
	dl := tbTestDialect()
	if dl.Name() != StyleThingsBoard {
		t.Errorf("name=%q", dl.Name())
	}
	if got := dl.Usernames(&models.Device{}); got != nil {
		t.Errorf("TB has no static usernames, got %v", got)
	}
	// the username is the credential, so any password verifies
	if ok, reason := dl.VerifyPassword(&models.Device{}, "", "c1"); !ok || reason != "" {
		t.Errorf("VerifyPassword(empty)=(%v,%q), want (true,\"\")", ok, reason)
	}
	if ok, _ := dl.VerifyPassword(&models.Device{}, "anything", "c1"); !ok {
		t.Error("VerifyPassword(anything)=false, want true")
	}
	if got := dl.CredsUsername("pk", "dn", "tok123"); got != "tok123" {
		t.Errorf("CredsUsername=%q want the raw credential", got)
	}
	if dl.UsesSecret() {
		t.Error("TB credentials are hashed tokens, not raw secrets")
	}
	if !dl.Pinned() {
		t.Error("TB must be pinned: the raw-token username is load-bearing")
	}
	if !dl.DownViaDevice(nil) {
		t.Error("TB downlink topics are shared across devices, delivery must resolve sessions")
	}
	if got := dl.DownTarget(nil); got != "" {
		t.Errorf("DownTarget=%q, want no static username", got)
	}
	wantPub := []string{tbTelemetryTopic, tbAttributesTopic, tbRPCResponseV1, tbRPCResponseV2}
	if got := dl.PublishRoots(nil); !reflect.DeepEqual(got, wantPub) {
		t.Errorf("PublishRoots=%v want %v", got, wantPub)
	}
	wantSub := []string{tbAttributesTopic, tbRPCRequestV1, tbRPCRequestV2}
	if got := dl.SubscribeRoots(nil); !reflect.DeepEqual(got, wantSub) {
		t.Errorf("SubscribeRoots=%v want %v", got, wantSub)
	}
	if !ValidStyle(StyleThingsBoard) {
		t.Error("thingsboard should be valid")
	}
}

// A pinned dialect is provisioned, never detected — DetectStyle must never
// answer "thingsboard". TB topics nest under the short v1/ prefix, so they may
// detect as "short"; ObserveTopicStyle's pinned guard keeps TB devices from
// flipping either way.
func TestDetectStyleThingsBoard(t *testing.T) {
	for _, topic := range []string{
		tbTelemetryTopic,
		tbAttributesTopic,
		tbAttrRequestTopic + "43",
		tbRPCRequestV1 + "5",
		tbRPCResponseV2 + "5",
	} {
		if got := DetectStyle(topic); got == StyleThingsBoard {
			t.Errorf("DetectStyle(%q)=%q, want anything but thingsboard (pinned dialect is not auto-detected)", topic, got)
		}
	}
}

func TestTBSnippet(t *testing.T) {
	got := thingsboardDialect{}.Snippet("pk", "dn", "mqtt.example:1883", "tok123")
	for _, want := range []string{"mqtt.example:1883", "tok123", tbTelemetryTopic} {
		if !strings.Contains(got, want) {
			t.Errorf("snippet %q missing %q", got, want)
		}
	}
}
