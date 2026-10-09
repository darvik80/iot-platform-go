package services

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
)

// fakePub captures downlinks for both publish paths (full-style plain
// Publish and short-style PublishToUser).
type fakePub struct {
	mu     sync.Mutex
	out    []pubMsg
	notify chan struct{}
}

type pubMsg struct {
	username string
	devicePK int64
	topic    string
	payload  []byte
}

func newFakePub() *fakePub {
	return &fakePub{notify: make(chan struct{}, 8)}
}

func (f *fakePub) Publish(topic string, qos byte, retained bool, payload []byte) error {
	f.mu.Lock()
	f.out = append(f.out, pubMsg{topic: topic, payload: append([]byte(nil), payload...)})
	f.mu.Unlock()
	select {
	case f.notify <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakePub) PublishToUser(username, topic string, qos byte, payload []byte) error {
	f.mu.Lock()
	f.out = append(f.out, pubMsg{username: username, topic: topic, payload: append([]byte(nil), payload...)})
	f.mu.Unlock()
	select {
	case f.notify <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakePub) PublishToDevice(devicePK int64, topic string, qos byte, payload []byte) error {
	f.mu.Lock()
	f.out = append(f.out, pubMsg{devicePK: devicePK, topic: topic, payload: append([]byte(nil), payload...)})
	f.mu.Unlock()
	select {
	case f.notify <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakePub) Connected() bool { return true }

func (f *fakePub) last(t *testing.T) pubMsg {
	t.Helper()
	select {
	case <-f.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("no downlink published")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out[len(f.out)-1]
}

func newIngestPlatform(t *testing.T) (*Platform, *models.Device, *fakePub) {
	t.Helper()
	plat := newTestPlatform(t, config.Config{})
	pub := newFakePub()
	plat.SetPublisher(pub)
	if _, err := plat.CreateProduct(1, ProductInput{ProductID: "home", Name: "home"}); err != nil {
		t.Fatal(err)
	}
	d, _, err := plat.CreateDevice(1, DeviceInput{ProductID: "home", DeviceID: "dev1"})
	if err != nil {
		t.Fatal(err)
	}
	return plat, d, pub
}

func setStyle(t *testing.T, plat *Platform, d *models.Device, style string) {
	t.Helper()
	if _, err := plat.DB.Exec(`UPDATE devices SET topic_style=? WHERE id=?`, style, d.ID); err != nil {
		t.Fatal(err)
	}
	d.TopicStyle = style
}

func TestParseUint32ID(t *testing.T) {
	cases := []struct {
		raw     string
		want    uint32
		wantErr bool
	}{
		{`7`, 7, false},
		{`"7"`, 7, false},
		{`0`, 0, false},
		{`"abc"`, 0, true},
		{`null`, 0, false}, // json null unmarshals into uint32 as zero
		{`{"id":1}`, 0, true},
		{`4294967295`, 4294967295, false},
		{`-1`, 0, true},
	}
	for _, tc := range cases {
		got, err := parseUint32ID(json.RawMessage(tc.raw))
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseUint32ID(%s)=%d, want error", tc.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseUint32ID(%s): %v", tc.raw, err)
		} else if got != tc.want {
			t.Errorf("parseUint32ID(%s)=%d want %d", tc.raw, got, tc.want)
		}
	}
}

func TestEchoID(t *testing.T) {
	if v := echoID(json.RawMessage(`"q1"`)); v != "q1" {
		t.Errorf("string id echoed as %T %v", v, v)
	}
	if v := echoID(json.RawMessage(`5`)); v != float64(5) {
		t.Errorf("number id echoed as %T %v", v, v)
	}
}

func TestParseTelemetryForms(t *testing.T) {
	samples, err := parseTelemetry([]byte(`[{"ts":100,"values":{"a":1}},{"ts":200,"values":{"b":true}}]`), 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[0].TS != 100 || samples[1].TS != 200 {
		t.Errorf("array form: %+v", samples)
	}
	if samples[0].ValueNum == nil || *samples[0].ValueNum != 1 {
		t.Errorf("array form num: %+v", samples[0])
	}
	if samples[1].ValueBool == nil || !*samples[1].ValueBool {
		t.Errorf("array form bool: %+v", samples[1])
	}

	samples, err = parseTelemetry([]byte(`{"ts":300,"values":{"a":2}}`), 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].TS != 300 {
		t.Errorf("object form: %+v", samples)
	}

	// flat map gets `now` as ts; ts/values keys are not samples
	samples, err = parseTelemetry([]byte(`{"c":3,"ts":5,"values":9}`), 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Key != "c" || samples[0].TS != 999 {
		t.Errorf("flat form: %+v", samples)
	}

	if _, err := parseTelemetry([]byte(`not json`), 999); err == nil {
		t.Error("invalid json accepted")
	}
}

func TestStoreTelemetryDedup(t *testing.T) {
	plat, d, _ := newIngestPlatform(t)

	payload := []byte(`{"temp":1}`)
	if err := plat.StoreTelemetry(d, payload); err != nil {
		t.Fatal(err)
	}
	if err := plat.StoreTelemetry(d, payload); err != nil {
		t.Fatal(err)
	}

	samples, err := plat.QueryTelemetry(d.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("identical payload within dedup window stored %d times", len(samples))
	}
}

func TestSetDesiredDownlinkShort(t *testing.T) {
	plat, d, pub := newIngestPlatform(t)

	if err := plat.SetDesired(d, map[string]any{"color": "red"}, true); err != nil {
		t.Fatal(err)
	}
	m := pub.last(t)
	if m.username != "home.dev1" {
		t.Errorf("username=%q want home.dev1", m.username)
	}
	if m.topic != "v1/me/a/upd" {
		t.Errorf("topic=%q want v1/me/a/upd", m.topic)
	}
	var attrs map[string]any
	if err := json.Unmarshal(m.payload, &attrs); err != nil {
		t.Fatal(err)
	}
	if attrs["color"] != "red" {
		t.Errorf("payload=%+v", attrs)
	}
}

func TestSetDesiredDownlinkFull(t *testing.T) {
	plat, d, pub := newIngestPlatform(t)
	setStyle(t, plat, d, protocol.StyleFull)

	if err := plat.SetDesired(d, map[string]any{"color": "red"}, true); err != nil {
		t.Fatal(err)
	}
	m := pub.last(t)
	if m.username != "" {
		t.Errorf("full style must use plain Publish, got username=%q", m.username)
	}
	if m.topic != "iot/v1/home/dev1/down/attributes/update" {
		t.Errorf("topic=%q", m.topic)
	}
}

func TestSetDesiredNoPublishWhenPubNil(t *testing.T) {
	plat, d, _ := newIngestPlatform(t)
	plat.SetPublisher(nil)
	if err := plat.SetDesired(d, map[string]any{"color": "red"}, true); err != nil {
		t.Fatalf("SetDesired without publisher must not fail: %v", err)
	}
	_, desired, err := plat.GetAttributes(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if desired["color"] != "red" {
		t.Errorf("desired=%+v", desired)
	}
}

func TestInvokeRPCDownlinkShort(t *testing.T) {
	plat, d, pub := newIngestPlatform(t)

	call, err := plat.InvokeRPC(d, RPCRequest{Method: "setLed", Params: map[string]any{"on": true}}, false)
	if err != nil {
		t.Fatal(err)
	}
	m := pub.last(t)
	if m.username != "home.dev1" {
		t.Errorf("username=%q want home.dev1", m.username)
	}
	if m.topic != "v1/me/r/req" {
		t.Errorf("topic=%q want v1/me/r/req", m.topic)
	}
	var body struct {
		ID     uint32         `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != call.RequestID {
		t.Errorf("payload id=%d want %d", body.ID, call.RequestID)
	}
	if body.Method != "setLed" || body.Params["on"] != true {
		t.Errorf("payload=%+v", body)
	}
}

func TestInvokeRPCDownlinkFull(t *testing.T) {
	plat, d, pub := newIngestPlatform(t)
	setStyle(t, plat, d, protocol.StyleFull)

	if _, err := plat.InvokeRPC(d, RPCRequest{Method: "reboot"}, false); err != nil {
		t.Fatal(err)
	}
	m := pub.last(t)
	if m.username != "" {
		t.Errorf("full style must use plain Publish, got username=%q", m.username)
	}
	if m.topic != "iot/v1/home/dev1/down/rpc/request" {
		t.Errorf("topic=%q", m.topic)
	}
}

func TestInvokeRPCRequiresMethod(t *testing.T) {
	plat, d, _ := newIngestPlatform(t)
	if _, err := plat.InvokeRPC(d, RPCRequest{}, false); err == nil {
		t.Error("empty method accepted")
	}
}

func TestInvokeRPCTimeout(t *testing.T) {
	plat, d, _ := newIngestPlatform(t)

	call, err := plat.InvokeRPC(d, RPCRequest{Method: "slow", TimeoutMS: 60}, true)
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != "timeout" {
		t.Errorf("status=%q want timeout", call.Status)
	}
	if call.Code == nil || *call.Code != 408 {
		t.Errorf("code=%v want 408", call.Code)
	}
	rows, err := plat.ListRPC(d.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != "timeout" || rows[0].Code == nil || *rows[0].Code != 408 {
		t.Errorf("rpc_calls row=%+v", rows)
	}
}

func TestInvokeRPCWithoutPublisherFails(t *testing.T) {
	plat, d, _ := newIngestPlatform(t)
	plat.SetPublisher(nil)
	if _, err := plat.InvokeRPC(d, RPCRequest{Method: "x"}, false); err == nil {
		t.Error("expected error when publisher is nil")
	}
}

func TestMergeReportedUpdatesFirmwareVersion(t *testing.T) {
	plat, d, _ := newIngestPlatform(t)

	if err := plat.MergeReported(d, map[string]any{"firmwareVersion": "2.0.1", "color": "red"}); err != nil {
		t.Fatal(err)
	}
	got, err := plat.GetDevice(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FwVersion != "2.0.1" {
		t.Errorf("fw_version=%q want 2.0.1", got.FwVersion)
	}
	reported, _, err := plat.GetAttributes(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reported["firmwareVersion"] != "2.0.1" || reported["color"] != "red" {
		t.Errorf("reported=%+v", reported)
	}
}

func TestReplyAttributeRequestFiltering(t *testing.T) {
	plat, d, pub := newIngestPlatform(t)

	if err := plat.MergeReported(d, map[string]any{"fw": "1.0", "color": "red"}); err != nil {
		t.Fatal(err)
	}
	if err := plat.SetDesired(d, map[string]any{"color": "blue"}, false); err != nil {
		t.Fatal(err)
	}

	req := []byte(`{"id":"r1","reported":["fw","nope"],"desired":["*"]}`)
	if err := plat.ReplyAttributeRequest(d, req); err != nil {
		t.Fatal(err)
	}
	m := pub.last(t)
	if m.topic != "v1/me/a/res" {
		t.Errorf("topic=%q want v1/me/a/res", m.topic)
	}
	var resp struct {
		ID       string         `json:"id"`
		Reported map[string]any `json:"reported"`
		Desired  map[string]any `json:"desired"`
	}
	if err := json.Unmarshal(m.payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "r1" {
		t.Errorf("id=%q want r1", resp.ID)
	}
	if len(resp.Reported) != 1 || resp.Reported["fw"] != "1.0" {
		t.Errorf("reported=%+v (unknown key 'nope' must be dropped)", resp.Reported)
	}
	if len(resp.Desired) != 1 || resp.Desired["color"] != "blue" {
		t.Errorf("desired=%+v (\"*\" must expand to all keys)", resp.Desired)
	}
}
