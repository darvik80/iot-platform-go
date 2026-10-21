package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const (
	alinkTestProduct = "pk1"
	alinkTestDevice  = "dev1"
	alinkTestSecret  = "secret1"

	// Independent vector (openssl dgst -sha256 -hmac secret1) over
	// clientIddev1&pk1deviceNamedev1productKeypk1timestamp2524608000000
	alinkTestSign = "0CEDFEAFB214592BCF68907FDC9B998158CB78C421CF5E2363DD457149D44233"

	// clientId the firmware sends (alicloud_credentials.cpp).
	alinkTestClientID = "dev1&pk1|timestamp=2524608000000,ext=3,_ss=2,_v=1.0.0," +
		"_m=ESP32-S3,securemode=3,signmethod=hmacsha256,lan=cpp|"
)

// assertJSON compares parsed JSON, so key order and float formatting don't matter.
func assertJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("output is not valid JSON: %v (%s)", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestAlinkUsername(t *testing.T) {
	if got := AlinkUsername("pk1", "dev1"); got != "dev1&pk1" {
		t.Fatalf("got %q", got)
	}
}

func TestAlinkSign(t *testing.T) {
	if got := AlinkSign(alinkTestSecret, alinkTestProduct, alinkTestDevice, ""); got != alinkTestSign {
		t.Errorf("default timestamp: got %q want %q", got, alinkTestSign)
	}
	if got := AlinkSign(alinkTestSecret, alinkTestProduct, alinkTestDevice, AlinkDefaultTimestamp); got != alinkTestSign {
		t.Errorf("explicit timestamp: got %q want %q", got, alinkTestSign)
	}
	if got := AlinkSign(alinkTestSecret, alinkTestProduct, alinkTestDevice, "1234567890"); got == alinkTestSign {
		t.Error("different timestamp must change the signature")
	}
	if got := AlinkSign("other", alinkTestProduct, alinkTestDevice, ""); got == alinkTestSign {
		t.Error("different secret must change the signature")
	}
}

func TestAlinkVerifyAuth(t *testing.T) {
	cases := []struct {
		name     string
		password string
		secret   string
		clientID string
		want     bool
	}{
		{"ok", alinkTestSign, alinkTestSecret, alinkTestClientID, true},
		{"lowercase normalized", strings.ToLower(alinkTestSign), alinkTestSecret, alinkTestClientID, true},
		{"surrounding spaces", " " + alinkTestSign + " ", alinkTestSecret, alinkTestClientID, true},
		{"wrong password", "0000000000000000000000000000000000000000000000000000000000000000", alinkTestSecret, alinkTestClientID, false},
		{"empty password", "", alinkTestSecret, alinkTestClientID, false},
		{"empty secret", alinkTestSign, "", alinkTestClientID, false},
		{"custom timestamp in clientID", AlinkSign(alinkTestSecret, alinkTestProduct, alinkTestDevice, "1234567890"),
			alinkTestSecret, "dev1&pk1|timestamp=1234567890,ext=3|", true},
		{"default-ts sign vs custom-ts clientID", alinkTestSign, alinkTestSecret, "dev1&pk1|timestamp=1234567890,ext=3|", false},
		{"clientID without timestamp falls back", alinkTestSign, alinkTestSecret, "dev1&pk1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AlinkVerifyAuth(c.password, c.secret, alinkTestProduct, alinkTestDevice, c.clientID)
			if got != c.want {
				t.Errorf("AlinkVerifyAuth=%v want %v", got, c.want)
			}
		})
	}
}

func TestAlinkTimestampFromClientID(t *testing.T) {
	cases := []struct {
		clientID string
		want     string
	}{
		{alinkTestClientID, "2524608000000"},
		{"dev1&pk1|timestamp=1234567890,ext=3|", "1234567890"},
		{"x|timestamp=42|", "42"},
		{"no marker", AlinkDefaultTimestamp},
		{"timestamp=", AlinkDefaultTimestamp},
		{"timestamp=abc", AlinkDefaultTimestamp},
	}
	for _, c := range cases {
		if got := AlinkTimestampFromClientID(c.clientID); got != c.want {
			t.Errorf("AlinkTimestampFromClientID(%q)=%q want %q", c.clientID, got, c.want)
		}
	}
}

func TestAlinkParseTopic(t *testing.T) {
	cases := []struct {
		topic   string
		product string
		device  string
		kind    string
		aux     string
	}{
		{"/sys/pk/dn/thing/event/property/post", "pk", "dn", KindTelemetry, ""},
		{"/sys/pk/dn/thing/event/highTemp/post", "pk", "dn", KindEvent, "highTemp"},
		{"/sys/pk/dn/thing/property/desired/get", "pk", "dn", KindAttrGet, ""},
		{"/sys/pk/dn/thing/property/desired/delete", "pk", "dn", KindAttrDelete, ""},
		{"/sys/pk/dn/thing/service/property/set_reply", "pk", "dn", KindRPCResponse, "property/set"},
		{"/sys/pk/dn/thing/service/custom/reboot/_reply", "pk", "dn", KindRPCResponse, "custom/reboot"},
		{"/sys/pk/dn/thing/ota/firmware/get", "pk", "dn", KindOTAQuery, ""},
		{"/sys/pk/dn/thing/log/post", "pk", "dn", KindLogs, ""},
		{"/ext/ntp/pk/dn/request", "pk", "dn", KindNTPRequest, ""},
		{"/ota/device/inform/pk/dn", "pk", "dn", KindOTAVersion, ""},
		{"/ota/device/progress/pk/dn", "pk", "dn", KindOTAProgress, ""},
		// no leading slash is accepted
		{"sys/pk/dn/thing/event/property/post", "pk", "dn", KindTelemetry, ""},
		// surrounding whitespace is trimmed
		{" /sys/pk/dn/thing/event/property/post ", "pk", "dn", KindTelemetry, ""},
	}
	for _, c := range cases {
		at, ok := AlinkParseTopic(c.topic)
		if !ok {
			t.Errorf("AlinkParseTopic(%q) rejected", c.topic)
			continue
		}
		if at.ProductID != c.product || at.DeviceID != c.device || at.Kind != c.kind || at.Aux != c.aux {
			t.Errorf("AlinkParseTopic(%q)={%s %s %s %s} want {%s %s %s %s}",
				c.topic, at.ProductID, at.DeviceID, at.Kind, at.Aux, c.product, c.device, c.kind, c.aux)
		}
	}
}

func TestAlinkParseTopicRejects(t *testing.T) {
	for _, topic := range []string{
		"iot/v1/p/d/up/telemetry/data", // full style
		"v1/t",                         // short style
		"$SYS/broker/clients",
		"/sys/pk/dn",                            // no tail
		"/sys/pk/dn/",                           // empty tail
		"/sys/pk/dn/thing/service/property/set", // downlink
		"/sys/pk/dn/thing/property/desired/get_reply", // downlink
		"/sys/pk/dn/thing/log/post_reply",             // reply, not uplink
		"/sys/p+/dn/thing/event/property/post",        // wildcard id
		"/sys/pk/d#/thing/event/property/post",        // wildcard id
		"/sys//dn/thing/event/property/post",          // empty product
		"/sys/pk/dn/thing/event//post",                // empty event id
		"/sys/pk/dn/thing/service//_reply",            // empty service method
		"/ext/ntp/pk/dn/response",                     // downlink
		"/ext/ntp/pk/dn",                              // no suffix
		"/ota/device/upgrade/pk/dn",                   // downlink
		"/ota/device/other/pk/dn",                     // unknown suffix
		"/ota/device/inform/pk",                       // missing device
	} {
		if _, ok := AlinkParseTopic(topic); ok {
			t.Errorf("AlinkParseTopic(%q) accepted, want rejected", topic)
		}
	}
}

func TestAlinkToInternalTelemetry(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{
			"grouped by time",
			`{"id":"1","version":"1.0","method":"thing.event.property.post","params":` +
				`{"temp":{"value":22.5,"time":1000},"hum":{"value":60,"time":1000},"volt":{"value":3300,"time":2000}}}`,
			`[{"ts":1000,"values":{"hum":60,"temp":22.5}},{"ts":2000,"values":{"volt":3300}}]`,
		},
		{
			"untimed params get no ts so the platform stamps now",
			`{"id":"1","params":{"temp":22}}`,
			`[{"values":{"temp":22}}]`,
		},
		{
			"timed groups first, untimed last",
			`{"params":{"a":{"value":1,"time":100},"b":2}}`,
			`[{"ts":100,"values":{"a":1}},{"values":{"b":2}}]`,
		},
		{
			"wrapper without time",
			`{"params":{"temp":{"value":22}}}`,
			`[{"values":{"temp":22}}]`,
		},
		{
			"object value",
			`{"params":{"pos":{"value":{"lat":1,"lon":2},"time":50}}}`,
			`[{"ts":50,"values":{"pos":{"lat":1,"lon":2}}}]`,
		},
		{
			"string value",
			`{"params":{"state":{"value":"on","time":7}}}`,
			`[{"ts":7,"values":{"state":"on"}}]`,
		},
		{
			"empty params",
			`{"params":{}}`,
			`[]`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := AlinkToInternal(KindTelemetry, "", []byte(c.payload))
			if err != nil {
				t.Fatal(err)
			}
			assertJSON(t, got, c.want)
		})
	}
}

func TestAlinkToInternalTelemetryInvalid(t *testing.T) {
	if _, err := AlinkToInternal(KindTelemetry, "", []byte(`{`)); err == nil {
		t.Error("invalid JSON must fail")
	}
	if _, err := AlinkToInternal(KindTelemetry, "", []byte(`{"params":[1,2]}`)); err == nil {
		t.Error("params array must fail")
	}
}

func TestAlinkToInternalEvent(t *testing.T) {
	got, err := AlinkToInternal(KindEvent, "highTemp", []byte(
		`{"id":"ev-1","version":"1.0","method":"thing.event.highTemp.post","params":`+
			`{"value":{"temp":80},"time":1700000000000}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"id":"ev-1","type":"highTemp","ts":1700000000000,"data":{"temp":80}}`)

	got, err = AlinkToInternal(KindEvent, "evt", []byte(`{"id":"e2","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	// no time -> ts omitted so RecordEvent defaults to now; no value -> data omitted
	assertJSON(t, got, `{"id":"e2","type":"evt"}`)
}

func TestAlinkToInternalRPCReply(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{
			"code 200 maps to internal 0",
			`{"id":"17","code":200,"message":"success","data":{"ok":true}}`,
			`{"id":"17","code":0,"message":"success","data":{"ok":true}}`,
		},
		{
			"error code passes through",
			`{"id":"18","code":500,"message":"boom"}`,
			`{"id":"18","code":500,"message":"boom"}`,
		},
		{
			"missing code counts as success",
			`{"id":"19","data":{"x":1}}`,
			`{"id":"19","code":0,"message":"","data":{"x":1}}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := AlinkToInternal(KindRPCResponse, "custom/reboot", []byte(c.payload))
			if err != nil {
				t.Fatal(err)
			}
			assertJSON(t, got, c.want)
		})
	}
}

func TestAlinkToInternalDesired(t *testing.T) {
	got, err := AlinkToInternal(KindAttrGet, "", []byte(`{"id":"9","version":"1.0","params":["temp","hum"]}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"id":"9","keys":["temp","hum"]}`)

	got, err = AlinkToInternal(KindAttrGet, "", []byte(`{"id":"9","params":null}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"id":"9","keys":[]}`)

	// desired delete carries versions the firmware wants cleared; keys only
	got, err = AlinkToInternal(KindAttrDelete, "", []byte(`{"id":"8","params":{"temp":{},"hum":{"version":3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"id":"8","keys":["hum","temp"]}`)

	got, err = AlinkToInternal(KindAttrDelete, "", []byte(`{"id":"8","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"id":"8","keys":[]}`)
}

func TestAlinkToInternalNTP(t *testing.T) {
	got, err := AlinkToInternal(KindNTPRequest, "", []byte(`{"deviceSendTime":"123456"}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"deviceSendTime":123456}`)

	got, err = AlinkToInternal(KindNTPRequest, "", []byte(`{"deviceSendTime":654321}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"deviceSendTime":654321}`)
}

func TestAlinkToInternalOTA(t *testing.T) {
	got, err := AlinkToInternal(KindOTAQuery, "", []byte(
		`{"id":"1","version":"1.0","method":"thing.ota.firmware.get","params":{"module":"default"}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"module":"default"}`)

	got, err = AlinkToInternal(KindOTAVersion, "", []byte(`{"id":123,"params":{"version":"1.2.3","module":"mcu"}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"version":"1.2.3","module":"mcu"}`)

	// the firmware reports step as a STRING
	got, err = AlinkToInternal(KindOTAProgress, "", []byte(
		`{"id":"456","params":{"step":"3","desc":"downloading","module":"default"}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"module":"default","step":3,"desc":"downloading"}`)

	got, err = AlinkToInternal(KindOTAProgress, "", []byte(`{"params":{"step":-1,"module":"default"}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `{"module":"default","step":-1,"desc":""}`)
}

func TestAlinkToInternalUnsupported(t *testing.T) {
	if _, err := AlinkToInternal(KindStatus, "", []byte(`{}`)); err == nil {
		t.Error("unsupported kind must fail")
	}
}

func TestAlinkToInternalLogs(t *testing.T) {
	got, err := AlinkToInternal(KindLogs, "", []byte(
		`{"id":"1","version":"1.0","method":"thing.log.post","sys":{"ack":0},"params":[`+
			`{"utcTime":"2024-01-02T03:04:05.678Z","logLevel":"ERROR","module":"wifi","code":"E_TIMEOUT",`+
			`"traceContext":"trace-42","logContent":"Read timeout"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `[{"ts":1704164645678,"level":"ERROR","module":"wifi","code":"E_TIMEOUT","message":"Read timeout","context":{"traceContext":"trace-42"}}]`)

	// epoch-milliseconds utcTime, no trace context
	got, err = AlinkToInternal(KindLogs, "", []byte(
		`{"params":[{"utcTime":1700000000000,"logLevel":"INFO","module":"wifi","logContent":"Connected to AP"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `[{"ts":1700000000000,"level":"INFO","module":"wifi","code":"","message":"Connected to AP"}]`)

	// unparsable utcTime -> ts 0, the ingest stamps server time
	got, err = AlinkToInternal(KindLogs, "", []byte(
		`{"params":[{"utcTime":"yesterday","logLevel":"WARN","module":"m","logContent":"hm"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `[{"ts":0,"level":"WARN","module":"m","code":"","message":"hm"}]`)

	// empty batch is accepted
	got, err = AlinkToInternal(KindLogs, "", []byte(`{"params":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, got, `[]`)
}

func TestAlinkDownlinkRPC(t *testing.T) {
	topic, body, err := AlinkDownlink("pk", "dn", CapRPC, OpRequest, "99", []byte(
		`{"id":42,"method":"custom/reboot","params":{"delay":5}}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != "/sys/pk/dn/thing/service/custom/reboot" {
		t.Errorf("topic %q", topic)
	}
	assertJSON(t, body, `{"id":"42","version":"1.0","method":"thing.service.custom/reboot","params":{"delay":5}}`)

	// id only in the arg when the payload carries none
	_, body, err = AlinkDownlink("pk", "dn", CapRPC, OpRequest, "7", []byte(`{"method":"reboot"}`))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, body, `{"id":"7","version":"1.0","method":"thing.service.reboot","params":{}}`)

	if _, _, err := AlinkDownlink("pk", "dn", CapRPC, OpRequest, "", []byte(`{"method":"reboot"}`)); err == nil {
		t.Error("rpc without id must fail")
	}
	if _, _, err := AlinkDownlink("pk", "dn", CapRPC, OpRequest, "1", []byte(`{"id":1}`)); err == nil {
		t.Error("rpc without method must fail")
	}
}

func TestAlinkDownlinkPropertySet(t *testing.T) {
	topic, body, err := AlinkDownlink("pk", "dn", CapAttributes, OpUpdate, "123", []byte(
		`{"temp":80,"on":true,"ratio":1.5,"label":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != "/sys/pk/dn/thing/service/property/set" {
		t.Errorf("topic %q", topic)
	}
	// every param must be a string or the firmware silently drops it
	assertJSON(t, body, `{"id":"123","version":"1.0","method":"thing.service.property.set",`+
		`"params":{"temp":"80","on":"true","ratio":"1.5","label":"x"}}`)

	if _, _, err := AlinkDownlink("pk", "dn", CapAttributes, OpUpdate, "", []byte(`{"temp":80}`)); err == nil {
		t.Error("property set without id must fail")
	}
}

func TestAlinkDownlinkDesiredReplies(t *testing.T) {
	topic, body, err := AlinkDownlink("pk", "dn", CapAttributes, OpGetReply, "", []byte(
		`{"id":"5","desired":{"temp":30}}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != "/sys/pk/dn/thing/property/desired/get_reply" {
		t.Errorf("topic %q", topic)
	}
	assertJSON(t, body, `{"id":"5","code":200,"message":"success","data":{"temp":30}}`)

	topic, body, err = AlinkDownlink("pk", "dn", CapAttributes, OpDeleteReply, "", []byte(`{"id":"6"}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != "/sys/pk/dn/thing/property/desired/delete_reply" {
		t.Errorf("topic %q", topic)
	}
	assertJSON(t, body, `{"id":"6","code":200,"message":"success"}`)

	if _, _, err := AlinkDownlink("pk", "dn", CapAttributes, OpGetReply, "", []byte(`{"desired":{}}`)); err == nil {
		t.Error("desired reply without id must fail")
	}
}

func TestAlinkDownlinkNTP(t *testing.T) {
	topic, body, err := AlinkDownlink("pk", "dn", CapNTP, OpResponse, "", []byte(
		`{"id":9,"deviceSendTime":100,"serverRecvTime":101,"serverSendTime":102}`))
	if err != nil {
		t.Fatal(err)
	}
	if topic != "/ext/ntp/pk/dn/response" {
		t.Errorf("topic %q", topic)
	}
	// id is dropped: the firmware reads the three times only
	assertJSON(t, body, `{"deviceSendTime":100,"serverRecvTime":101,"serverSendTime":102}`)
}

func TestAlinkDownlinkOTA(t *testing.T) {
	internal := `{"version":"1.2.3","module":"default","size":1024,"url":"http://fw/1.2.3",` +
		`"sha256":"abc","sign":"sig","signMethod":"MD5","force":true}`

	topic, body, err := AlinkDownlink("pk", "dn", CapOTA, OpUpdate, "", []byte(internal))
	if err != nil {
		t.Fatal(err)
	}
	if topic != "/ota/device/upgrade/pk/dn" {
		t.Errorf("topic %q", topic)
	}
	// STRING code "1000", data passed through verbatim
	assertJSON(t, body, `{"code":"1000","data":`+internal+`}`)

	// cancel: firmware ignores anything but "1000"
	_, body, err = AlinkDownlink("pk", "dn", CapOTA, OpCancel, "", []byte(internal))
	if err != nil {
		t.Fatal(err)
	}
	assertJSON(t, body, `{"code":"-1","data":`+internal+`}`)
}

func TestAlinkDownlinkUnsupported(t *testing.T) {
	if _, _, err := AlinkDownlink("pk", "dn", CapRPC, OpResponse, "1", []byte(`{}`)); err == nil {
		t.Error("unsupported downlink must fail")
	}
}

func TestAlinkRoots(t *testing.T) {
	want := []string{"/sys/pk/dn/", "/ext/ntp/pk/dn/", "/ota/device/inform/pk/dn", "/ota/device/progress/pk/dn"}
	if got := AlinkUplinkRoots("pk", "dn"); !reflect.DeepEqual(got, want) {
		t.Errorf("uplink roots %v want %v", got, want)
	}
	want = []string{"/sys/pk/dn/", "/ext/ntp/pk/dn/", "/ota/device/upgrade/pk/dn"}
	if got := AlinkDownlinkRoots("pk", "dn"); !reflect.DeepEqual(got, want) {
		t.Errorf("downlink roots %v want %v", got, want)
	}
}

func TestDetectStyleAlink(t *testing.T) {
	cases := []struct {
		topic string
		want  string
	}{
		{"/sys/pk/dn/thing/event/property/post", StyleAlink},
		{"sys/pk/dn/thing/service/property/set_reply", StyleAlink},
		{"/ext/ntp/pk/dn/request", StyleAlink},
		{"/ota/device/inform/pk/dn", StyleAlink},
		{"/ota/device/progress/pk/dn", StyleAlink},
		// Known collision: a rules-bus prefix of "sys/…" would look like alink.
		// Accepted by design — DetectStyle only runs on device CONNECT will
		// topics and subscribe filters, and the alink ACL gates devices to
		// their own roots anyway.
		{"sys/v1/events/acme/dev1", StyleAlink},
		{"platform/v1/events/x", ""},
	}
	for _, c := range cases {
		if got := DetectStyle(c.topic); got != c.want {
			t.Errorf("DetectStyle(%q)=%q want %q", c.topic, got, c.want)
		}
	}
}

func TestValidStyleAlink(t *testing.T) {
	if !ValidStyle(StyleAlink) {
		t.Error("alink should be valid")
	}
}
