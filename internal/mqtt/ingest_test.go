package mqtt

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
	"cogitor-iot-platform/internal/services"
)

// capturePub records every downlink the platform emits, both the plain
// Publish path (full style) and the PublishToUser path (short style).
type capturePub struct {
	ch chan downMsg
}

type downMsg struct {
	username string // set for PublishToUser; empty for plain Publish
	devicePK int64  // set for PublishToDevice
	topic    string
	payload  []byte
}

func newCapturePub() *capturePub {
	return &capturePub{ch: make(chan downMsg, 16)}
}

func (c *capturePub) Publish(topic string, qos byte, retained bool, payload []byte) error {
	c.ch <- downMsg{topic: topic, payload: append([]byte(nil), payload...)}
	return nil
}

func (c *capturePub) PublishToUser(username, topic string, qos byte, payload []byte) error {
	c.ch <- downMsg{username: username, topic: topic, payload: append([]byte(nil), payload...)}
	return nil
}

func (c *capturePub) PublishToDevice(devicePK int64, topic string, qos byte, payload []byte) error {
	c.ch <- downMsg{devicePK: devicePK, topic: topic, payload: append([]byte(nil), payload...)}
	return nil
}

func (c *capturePub) Connected() bool { return true }

func (c *capturePub) wait(t *testing.T) downMsg {
	t.Helper()
	select {
	case m := <-c.ch:
		return m
	case <-time.After(2 * time.Second):
		t.Fatal("no downlink arrived")
		return downMsg{}
	}
}

func (c *capturePub) empty() bool {
	select {
	case <-c.ch:
		return false
	default:
		return true
	}
}

func newIngestBroker(t *testing.T) (*Broker, *services.Platform, *models.Device, *capturePub) {
	t.Helper()
	b, plat, alice, _ := newIsolationBroker(t)
	pub := newCapturePub()
	plat.SetPublisher(pub)
	return b, plat, alice, pub
}

func sampleOf(t *testing.T, samples []models.TelemetrySample, key string) *models.TelemetrySample {
	t.Helper()
	for i := range samples {
		if samples[i].Key == key {
			return &samples[i]
		}
	}
	t.Fatalf("no telemetry sample for key %q in %+v", key, samples)
	return nil
}

func TestIngestShortTelemetryStoresSamples(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	b.handle("v1/me/t", []byte(`{"temp":21.5,"mode":"heat","on":true}`), "lamp.l1")

	samples, err := plat.QueryTelemetry(alice.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 3 {
		t.Fatalf("want 3 samples, got %d: %+v", len(samples), samples)
	}
	if s := sampleOf(t, samples, "temp"); s.ValueNum == nil || *s.ValueNum != 21.5 {
		t.Errorf("temp=%+v", s)
	}
	if s := sampleOf(t, samples, "mode"); s.ValueText == nil || *s.ValueText != "heat" {
		t.Errorf("mode=%+v", s)
	}
	if s := sampleOf(t, samples, "on"); s.ValueBool == nil || !*s.ValueBool {
		t.Errorf("on=%+v", s)
	}
	// resolve() marks the session device online and records the short dialect.
	d, err := plat.GetDevice(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "online" {
		t.Errorf("status=%q want online", d.Status)
	}
	if d.TopicStyle != protocol.StyleShort {
		t.Errorf("topic_style=%q want short", d.TopicStyle)
	}
}

func TestIngestTelemetryPayloadForms(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	// array form with per-item ts
	b.handle("v1/me/t", []byte(`[{"ts":100,"values":{"a":1}},{"ts":200,"values":{"a":2}}]`), "lamp.l1")
	// single object form
	b.handle("v1/me/t", []byte(`{"ts":300,"values":{"b":3}}`), "lamp.l1")
	// flat map form (ts assigned at ingest)
	b.handle("v1/me/t", []byte(`{"c":4}`), "lamp.l1")

	samples, err := plat.QueryTelemetry(alice.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	byTS := map[int64]int{}
	for _, s := range samples {
		byTS[s.TS]++
		if s.TS <= 0 {
			t.Errorf("sample %q has ts=%d", s.Key, s.TS)
		}
	}
	if byTS[100] != 1 || byTS[200] != 1 || byTS[300] != 1 {
		t.Errorf("array/object ts handling: %+v", byTS)
	}
	if len(samples) != 4 {
		t.Fatalf("want 4 samples, got %d: %+v", len(samples), samples)
	}
}

func TestIngestTelemetryDedup(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	payload := []byte(`{"temp":1}`)
	b.handle("v1/me/t", payload, "lamp.l1")
	b.handle("v1/me/t", payload, "lamp.l1")

	samples, err := plat.QueryTelemetry(alice.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("identical payload within dedup window stored %d times", len(samples))
	}
}

func TestIngestShortEvent(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	b.handle("v1/me/e", []byte(`{"ts":123,"id":"ev1","type":"fault","severity":"warning","code":"E1","message":"overheat"}`), "lamp.l1")

	events, err := plat.ListEvents(0, alice.ID, "", 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	var e *models.Event
	for i := range events {
		if events[i].EventID == "ev1" {
			e = &events[i]
		}
	}
	if e == nil {
		t.Fatalf("event ev1 not stored: %+v", events)
	}
	if e.EventID != "ev1" || e.Type != "fault" || e.Severity != "warning" || e.Code != "E1" || e.Message != "overheat" || e.TS != 123 {
		t.Errorf("event=%+v", e)
	}
	if e.ProductID != "lamp" || e.DeviceID != "l1" {
		t.Errorf("event identity=%q/%q", e.ProductID, e.DeviceID)
	}
}

func TestIngestShortLogs(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	b.handle("v1/me/l", []byte(`[{"ts":1,"level":"info","module":"wifi","code":"W1","message":"up","context":{"rssi":-60}}]`), "lamp.l1")

	logs, err := plat.ListLogs(0, alice.ID, "", 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("want 1 log, got %d", len(logs))
	}
	l := logs[0]
	if l.Level != "info" || l.Module != "wifi" || l.Code != "W1" || l.Message != "up" || l.TS != 1 {
		t.Errorf("log=%+v", l)
	}
	if l.ContextJSON == nil || !containsJSONKey(*l.ContextJSON, "rssi") {
		t.Errorf("context=%+v", l.ContextJSON)
	}
}

func containsJSONKey(js, key string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func TestIngestShortAttrReport(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	b.handle("v1/me/a", []byte(`{"fw":"1.2.3","color":"red"}`), "lamp.l1")

	reported, _, err := plat.GetAttributes(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reported["fw"] != "1.2.3" || reported["color"] != "red" {
		t.Errorf("reported=%+v", reported)
	}
}

func TestIngestShortAttrRequestRepliesDownlink(t *testing.T) {
	b, _, _, pub := newIngestBroker(t)

	b.handle("v1/me/a", []byte(`{"fw":"1.2.3"}`), "lamp.l1")
	b.handle("v1/me/a/req", []byte(`{"id":"attr1","reported":["*"],"desired":["fw","missing"]}`), "lamp.l1")

	m := pub.wait(t)
	if m.topic != "v1/me/a/res" {
		t.Errorf("downlink topic=%q want v1/me/a/res", m.topic)
	}
	if m.username != "lamp.l1" {
		t.Errorf("downlink username=%q want lamp.l1", m.username)
	}
	var resp struct {
		ID       string         `json:"id"`
		Reported map[string]any `json:"reported"`
		Desired  map[string]any `json:"desired"`
	}
	if err := json.Unmarshal(m.payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "attr1" {
		t.Errorf("id=%q want attr1", resp.ID)
	}
	if resp.Reported["fw"] != "1.2.3" {
		t.Errorf("reported=%+v", resp.Reported)
	}
	if len(resp.Desired) != 0 {
		t.Errorf("desired filter should drop unknown keys, got %+v", resp.Desired)
	}
}

func TestIngestShortStatusMarksOffline(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	b.handle("v1/me/s", []byte(`{"online":false,"reason":"lwt"}`), "lamp.l1")

	d, err := plat.GetDevice(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "offline" {
		t.Errorf("status=%q want offline", d.Status)
	}
}

func TestIngestFullStyleTelemetryAndStyleSwitch(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	topic := protocol.FullUpRoot("lamp", "l1") + protocol.UpTelemetry
	b.handle(topic, []byte(`{"ts":5,"values":{"v":1}}`), "lamp.l1")

	samples, err := plat.QueryTelemetry(alice.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Key != "v" || samples[0].TS != 5 {
		t.Fatalf("samples=%+v", samples)
	}
	d, err := plat.GetDevice(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.TopicStyle != protocol.StyleFull {
		t.Errorf("topic_style=%q want full", d.TopicStyle)
	}
}

func TestIngestOTAVersionUpdatesDevice(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	b.handle("v1/me/o/ver", []byte(`{"version":"1.2.0","module":"main"}`), "lamp.l1")

	d, err := plat.GetDevice(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.FwVersion != "1.2.0" {
		t.Errorf("fw_version=%q", d.FwVersion)
	}
	reported, _, err := plat.GetAttributes(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reported["firmwareVersion"] != "1.2.0" {
		t.Errorf("reported firmwareVersion=%v", reported["firmwareVersion"])
	}
}

func TestRPCRoundTripShort(t *testing.T) {
	b, plat, alice, pub := newIngestBroker(t)

	done := make(chan *models.RPCCall, 1)
	go func() {
		call, err := plat.InvokeRPC(alice, services.RPCRequest{Method: "setLed", Params: map[string]any{"on": true}, TimeoutMS: 3000}, true)
		if err != nil {
			close(done)
			return
		}
		done <- call
	}()

	m := pub.wait(t)
	if m.topic != "v1/me/r/req" {
		t.Errorf("downlink topic=%q want v1/me/r/req", m.topic)
	}
	if m.username != "lamp.l1" {
		t.Errorf("downlink username=%q", m.username)
	}
	var body struct {
		ID     uint32         `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.Method != "setLed" || body.Params["on"] != true {
		t.Errorf("downlink body=%+v", body)
	}

	// numeric id reply
	b.handle("v1/me/r/res", fmt.Appendf(nil, `{"id":%d,"code":0,"message":"ok","data":{"done":true}}`, body.ID), "lamp.l1")

	select {
	case call := <-done:
		if call == nil {
			t.Fatal("InvokeRPC failed")
		}
		if call.Status != "ok" {
			t.Errorf("status=%q want ok", call.Status)
		}
		if call.Code == nil || *call.Code != 0 {
			t.Errorf("code=%v", call.Code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RPC waiter not released")
	}
	calls, err := plat.ListRPC(alice.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Status != "ok" {
		t.Fatalf("rpc_calls row=%+v", calls)
	}
}

func TestRPCRoundTripStringID(t *testing.T) {
	b, plat, alice, pub := newIngestBroker(t)

	done := make(chan *models.RPCCall, 1)
	go func() {
		call, err := plat.InvokeRPC(alice, services.RPCRequest{Method: "reboot", TimeoutMS: 3000}, true)
		if err != nil {
			close(done)
			return
		}
		done <- call
	}()

	m := pub.wait(t)
	var body struct {
		ID uint32 `json:"id"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}

	// id echoed as string (alink-style envelope): must still route to the waiter
	b.handle("v1/me/r/res", fmt.Appendf(nil, `{"id":"%d","code":0,"message":"success"}`, body.ID), "lamp.l1")

	select {
	case call := <-done:
		if call == nil {
			t.Fatal("InvokeRPC failed")
		}
		if call.Status != "ok" {
			t.Errorf("status=%q want ok", call.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RPC waiter not released (string id not parsed)")
	}
}

func TestClientRPCIssueUploadUrl(t *testing.T) {
	b, _, _, pub := newIngestBroker(t)

	b.handle("v1/me/r/creq", []byte(`{"id":"q1","method":"issueUploadUrl","params":{}}`), "lamp.l1")

	m := pub.wait(t)
	if m.topic != "v1/me/r/cres" {
		t.Errorf("downlink topic=%q want v1/me/r/cres", m.topic)
	}
	var resp struct {
		ID      string `json:"id"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			URL       string `json:"url"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(m.payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "q1" {
		t.Errorf("id=%q want q1 (echoed as string)", resp.ID)
	}
	if resp.Code != 0 || resp.Message != "ok" {
		t.Errorf("code=%d message=%q", resp.Code, resp.Message)
	}
	if resp.Data.URL == "" || resp.Data.ExpiresIn != 3600 {
		t.Errorf("data=%+v", resp.Data)
	}
}

func TestNTPRequestRepliesDownlink(t *testing.T) {
	b, _, _, pub := newIngestBroker(t)

	b.handle("v1/me/n/req", []byte(`{"id":"n1","deviceSendTime":123}`), "lamp.l1")

	m := pub.wait(t)
	if m.topic != "v1/me/n/res" {
		t.Errorf("downlink topic=%q want v1/me/n/res", m.topic)
	}
	var resp struct {
		ID             string `json:"id"`
		DeviceSendTime int64  `json:"deviceSendTime"`
		ServerRecvTime  int64 `json:"serverRecvTime"`
		ServerSendTime  int64 `json:"serverSendTime"`
	}
	if err := json.Unmarshal(m.payload, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != "n1" || resp.DeviceSendTime != 123 {
		t.Errorf("echo fields=%+v", resp)
	}
	if resp.ServerRecvTime <= 0 || resp.ServerSendTime < resp.ServerRecvTime {
		t.Errorf("server times=%+v", resp)
	}
}

func TestIngestForeignSessionRejected(t *testing.T) {
	b, plat, alice, _ := newIngestBroker(t)

	// bob's session publishing short telemetry still targets bob's own device
	b.handle("v1/me/t", []byte(`{"x":1}`), "home.h1")

	bobRows, err := plat.QueryTelemetry(bobID(t, plat), nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	aliceRows, err := plat.QueryTelemetry(alice.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(bobRows) != 1 {
		t.Errorf("bob's telemetry not stored: %+v", bobRows)
	}
	if len(aliceRows) != 0 {
		t.Errorf("alice's rows changed: %+v", aliceRows)
	}
}

func bobID(t *testing.T, plat *services.Platform) int64 {
	t.Helper()
	d, err := plat.GetDeviceByIdentity("home", "h1")
	if err != nil {
		t.Fatal(err)
	}
	return d.ID
}
