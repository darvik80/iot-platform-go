package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
	"cogitor-iot-platform/internal/services"
)

// newAlinkBroker adds an alink-style device (lamp.esp1, owned by alice) to the
// isolation platform and captures its downlinks.
func newAlinkBroker(t *testing.T) (*Broker, *services.Platform, *models.Device, *capturePub, string) {
	t.Helper()
	b, plat, alice, _ := newIsolationBroker(t)
	const secret = "alink-secret-1"
	d, _, err := plat.CreateDevice(alice.AccountID, services.DeviceInput{
		ProductID: "lamp", DeviceID: "esp1",
		TopicStyle: protocol.StyleAlink, AlinkSecret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	pub := newCapturePub()
	plat.SetPublisher(pub)
	return b, plat, d, pub, secret
}

func alinkUser(d *models.Device) string {
	return protocol.AlinkUsername(d.ProductID, d.DeviceID)
}

// alinkCID builds the KV-suffixed clientId the alink firmware connects with.
func alinkCID(user, ts string) string {
	return user + "|timestamp=" + ts + ",ext=3,_ss=2,_v=1.0.0,_m=ESP32-S3,securemode=3,signmethod=hmacsha256,lan=cpp|"
}

func TestAuthAlinkHMAC(t *testing.T) {
	b, plat, d, _, secret := newAlinkBroker(t)
	h := &platHook{b: b}
	user := alinkUser(d)
	cid := alinkCID(user, protocol.AlinkDefaultTimestamp)
	pass := protocol.AlinkSign(secret, d.ProductID, d.DeviceID, "")

	if !h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, pass, cid, "")) {
		t.Error("valid alink sign rejected")
	}
	if !h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, strings.ToLower(pass), cid, "")) {
		t.Error("lowercase alink sign rejected")
	}
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, "wrong", cid, "")) {
		t.Error("bad alink sign accepted")
	}
	// sign timestamp comes from the clientId KV section, not only the default
	ts := "1712000000000"
	if !h.OnConnectAuthenticate(fakeClient("c2", user, "127.0.0.1:1"),
		connectPacket(user, protocol.AlinkSign(secret, d.ProductID, d.DeviceID, ts), alinkCID(user, ts), "")) {
		t.Error("custom timestamp sign rejected")
	}
	if err := plat.SetDeviceEnabled(d.ID, false); err != nil {
		t.Fatal(err)
	}
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, pass, cid, "")) {
		t.Error("disabled alink device accepted")
	}
}

func TestAuthAlinkCredentialCrossRejected(t *testing.T) {
	b, _, d, _, secret := newAlinkBroker(t)
	h := &platHook{b: b}
	user := alinkUser(d)

	// an access token must not authenticate an alink device
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"),
		connectPacket(user, "tok_notarealtoken", alinkCID(user, protocol.AlinkDefaultTimestamp), "")) {
		t.Error("token password accepted for alink device")
	}
	// the alink secret must not authenticate a token-style device
	if h.OnConnectAuthenticate(fakeClient("c1", "lamp.l1", "127.0.0.1:1"), connectPacket("lamp.l1", secret, "c1", "")) {
		t.Error("alink secret accepted for token device")
	}
}

func TestACLAlinkDevice(t *testing.T) {
	b, _, d, _, _ := newAlinkBroker(t)
	h := &platHook{b: b}
	user := alinkUser(d)
	sys := "/sys/" + d.ProductID + "/" + d.DeviceID
	ntp := "/ext/ntp/" + d.ProductID + "/" + d.DeviceID

	cases := []struct {
		name    string
		topic   string
		write   bool
		allowed bool
	}{
		{"telemetry publish", sys + "/thing/event/property/post", true, true},
		{"event publish", sys + "/thing/event/fault/post", true, true},
		{"desired get publish", sys + "/thing/property/desired/get", true, true},
		{"ntp request publish", ntp + "/request", true, true},
		{"ota inform publish", "/ota/device/inform/" + d.ProductID + "/" + d.DeviceID, true, true},
		{"ota progress publish", "/ota/device/progress/" + d.ProductID + "/" + d.DeviceID, true, true},
		{"service subscribe", sys + "/thing/service/+/+", false, true},
		{"sys root filter subscribe", sys + "/#", false, true},
		{"ntp response subscribe", ntp + "/response", false, true},
		{"ota upgrade subscribe", "/ota/device/upgrade/" + d.ProductID + "/" + d.DeviceID, false, true},
		{"ota upgrade publish is downlink-only", "/ota/device/upgrade/" + d.ProductID + "/" + d.DeviceID, true, false},
		{"foreign device sys publish", "/sys/home/h1/thing/event/property/post", true, false},
		{"foreign device ota subscribe", "/ota/device/upgrade/home/h1", false, false},
		{"short publish", protocol.ShortMe(protocol.ShortTelemetry), true, false},
		{"full publish", protocol.FullUpRoot(d.ProductID, d.DeviceID) + protocol.UpTelemetry, true, false},
		{"sys topic", "$SYS/broker/uptime", false, false},
		{"unrelated topic", "random/topic", true, false},
	}
	for _, tc := range cases {
		if got := h.aclOK(user, tc.topic, tc.write); got != tc.allowed {
			t.Errorf("%s: aclOK(%q, write=%v)=%v want %v", tc.name, tc.topic, tc.write, got, tc.allowed)
		}
	}
}

func TestAlinkTelemetryIngest(t *testing.T) {
	b, plat, d, _, _ := newAlinkBroker(t)

	b.handle("/sys/"+d.ProductID+"/"+d.DeviceID+"/thing/event/property/post",
		[]byte(`{"id":"1","version":"1.0","params":{"temp":{"value":21.5,"time":150},"mode":{"value":"heat"},"on":{"value":true}}}`), alinkUser(d))

	samples, err := plat.QueryTelemetry(d.ID, nil, 0, 0, 100)
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
	got, err := plat.GetDevice(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "online" {
		t.Errorf("status=%q want online", got.Status)
	}
	if got.TopicStyle != protocol.StyleAlink {
		t.Errorf("topic_style=%q want alink", got.TopicStyle)
	}
}

func TestAlinkEventIngest(t *testing.T) {
	b, plat, d, _, _ := newAlinkBroker(t)

	b.handle("/sys/"+d.ProductID+"/"+d.DeviceID+"/thing/event/fault/post",
		[]byte(`{"id":"9","version":"1.0","params":{"value":{"code":7},"time":150}}`), alinkUser(d))

	events, err := plat.ListEvents(d.AccountID, d.ID, "", 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	var fault *models.Event
	for i := range events {
		if events[i].Type == "fault" {
			fault = &events[i]
		}
	}
	if fault == nil {
		t.Fatalf("no fault event in %+v", events)
	}
	if fault.EventID != "9" || fault.TS != 150 {
		t.Errorf("event=%+v", fault)
	}
}

func TestAlinkDesiredGetRoundTrip(t *testing.T) {
	b, plat, d, pub, _ := newAlinkBroker(t)
	if err := plat.SetDesired(d, map[string]any{"temp": 20.0}, false); err != nil {
		t.Fatal(err)
	}

	b.handle("/sys/"+d.ProductID+"/"+d.DeviceID+"/thing/property/desired/get",
		[]byte(`{"id":"5","version":"1.0","params":["temp"]}`), alinkUser(d))

	m := pub.wait(t)
	if m.topic != "/sys/"+d.ProductID+"/"+d.DeviceID+"/thing/property/desired/get_reply" {
		t.Errorf("topic=%q", m.topic)
	}
	if m.username != alinkUser(d) {
		t.Errorf("username=%q", m.username)
	}
	var body struct {
		ID      string         `json:"id"`
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "5" {
		t.Errorf("id=%q want string 5 (firmware requires STRING ids)", body.ID)
	}
	if body.Code != 200 {
		t.Errorf("code=%d want 200", body.Code)
	}
	if body.Data["temp"] != 20.0 {
		t.Errorf("data=%+v", body.Data)
	}
}

func TestAlinkDesiredDeleteRoundTrip(t *testing.T) {
	b, plat, d, pub, _ := newAlinkBroker(t)
	if err := plat.SetDesired(d, map[string]any{"temp": 20.0, "mode": "heat"}, false); err != nil {
		t.Fatal(err)
	}

	b.handle("/sys/"+d.ProductID+"/"+d.DeviceID+"/thing/property/desired/delete",
		[]byte(`{"id":"6","version":"1.0","params":{"temp":null}}`), alinkUser(d))

	m := pub.wait(t)
	if m.topic != "/sys/"+d.ProductID+"/"+d.DeviceID+"/thing/property/desired/delete_reply" {
		t.Errorf("topic=%q", m.topic)
	}
	var body struct {
		ID   string `json:"id"`
		Code int    `json:"code"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "6" || body.Code != 200 {
		t.Errorf("body=%+v", body)
	}
	if got := desiredOf(t, plat, d); got["temp"] != nil || got["mode"] != "heat" {
		t.Errorf("desired after delete=%v", got)
	}
}

func TestAlinkNTPRoundTrip(t *testing.T) {
	b, _, d, pub, _ := newAlinkBroker(t)

	b.handle("/ext/ntp/"+d.ProductID+"/"+d.DeviceID+"/request",
		[]byte(`{"deviceSendTime":"123"}`), alinkUser(d))

	m := pub.wait(t)
	if m.topic != "/ext/ntp/"+d.ProductID+"/"+d.DeviceID+"/response" {
		t.Errorf("topic=%q", m.topic)
	}
	if m.username != alinkUser(d) {
		t.Errorf("username=%q", m.username)
	}
	var body struct {
		DeviceSendTime int64 `json:"deviceSendTime"`
		ServerRecvTime int64 `json:"serverRecvTime"`
		ServerSendTime int64 `json:"serverSendTime"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.DeviceSendTime != 123 || body.ServerRecvTime <= 0 || body.ServerSendTime < body.ServerRecvTime {
		t.Errorf("ntp body=%+v", body)
	}
}

func TestAlinkPropertySetDownlink(t *testing.T) {
	_, plat, d, pub, _ := newAlinkBroker(t)

	if err := plat.SetDesired(d, map[string]any{"power": true, "level": 3}, true); err != nil {
		t.Fatal(err)
	}

	m := pub.wait(t)
	if m.topic != "/sys/"+d.ProductID+"/"+d.DeviceID+"/thing/service/property/set" {
		t.Errorf("topic=%q", m.topic)
	}
	if m.username != alinkUser(d) {
		t.Errorf("username=%q", m.username)
	}
	var body struct {
		ID      string            `json:"id"`
		Version string            `json:"version"`
		Method  string            `json:"method"`
		Params  map[string]string `json:"params"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatalf("property/set payload is not all-strings (firmware drops non-string params): %v", err)
	}
	if body.ID == "" {
		t.Error("empty id")
	}
	if body.Version != "1.0" || body.Method != "thing.service.property.set" {
		t.Errorf("version=%q method=%q payload=%s", body.Version, body.Method, m.payload)
	}
	if body.Params["power"] != "true" || body.Params["level"] != "3" {
		t.Errorf("params=%+v want stringified values", body.Params)
	}
}

func TestAlinkRPCRoundTrip(t *testing.T) {
	b, plat, d, pub, _ := newAlinkBroker(t)
	user := alinkUser(d)
	sys := "/sys/" + d.ProductID + "/" + d.DeviceID

	done := make(chan *models.RPCCall, 1)
	go func() {
		call, err := plat.InvokeRPC(d, services.RPCRequest{Method: "reboot", Params: map[string]any{"on": true}, TimeoutMS: 3000}, true)
		if err != nil {
			close(done)
			return
		}
		done <- call
	}()

	m := pub.wait(t)
	if m.topic != sys+"/thing/service/reboot" {
		t.Errorf("topic=%q", m.topic)
	}
	if m.username != user {
		t.Errorf("username=%q", m.username)
	}
	var body struct {
		ID      string         `json:"id"`
		Version string         `json:"version"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params"`
	}
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.ID == "" {
		t.Error("empty id")
	}
	if body.Version != "1.0" || body.Method != "thing.service.reboot" {
		t.Errorf("body=%+v", body)
	}
	if body.Params["on"] != true {
		t.Errorf("params=%+v (service params pass through unmodified)", body.Params)
	}

	// firmware replies on thing/service/{method}/_reply with code 200 and a STRING id
	b.handle(sys+"/thing/service/reboot/_reply",
		fmt.Appendf(nil, `{"id":%q,"code":200,"message":"success"}`, body.ID), user)

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

func TestAlinkOTAInform(t *testing.T) {
	b, plat, d, _, _ := newAlinkBroker(t)

	b.handle("/ota/device/inform/"+d.ProductID+"/"+d.DeviceID,
		[]byte(`{"id":"0","params":{"version":"1.2.3","module":"default"}}`), alinkUser(d))

	got, err := plat.GetDevice(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.FwVersion != "1.2.3" {
		t.Errorf("fw_version=%q", got.FwVersion)
	}
}

func TestAlinkSessionBoundToDevice(t *testing.T) {
	b, plat, d, _, _ := newAlinkBroker(t)
	topic := "/sys/" + d.ProductID + "/" + d.DeviceID + "/thing/event/property/post"
	payload := []byte(`{"id":"1","version":"1.0","params":{"temp":{"value":1}}}`)

	// bob's token session cannot speak as alice's alink device
	b.handle(topic, payload, "home.h1")
	// neither can alice's own token device from the same product
	b.handle(topic, payload, "lamp.l1")

	samples, err := plat.QueryTelemetry(d.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Errorf("foreign session ingested as alink device: %+v", samples)
	}
}

func TestAlinkStyleNotAutoFlipped(t *testing.T) {
	b, plat, d, _, secret := newAlinkBroker(t)
	h := &platHook{b: b}
	user := alinkUser(d)
	pass := protocol.AlinkSign(secret, d.ProductID, d.DeviceID, "")

	// a short-dialect Will on an otherwise valid alink CONNECT must not flip
	// the stored style (that would strand the HMAC credential model)
	if !h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"),
		connectPacket(user, pass, alinkCID(user, protocol.AlinkDefaultTimestamp), protocol.ShortMe(protocol.ShortStatus))) {
		t.Fatal("valid alink connect with short will rejected")
	}
	got, err := plat.GetDevice(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TopicStyle != protocol.StyleAlink {
		t.Errorf("topic_style=%q want alink (will topic must not flip it)", got.TopicStyle)
	}

	// a short-dialect publish from the alink session is dropped, style stays
	b.handle(protocol.ShortMe(protocol.ShortTelemetry), []byte(`{"temp":1}`), user)
	if got, err = plat.GetDevice(d.ID); err != nil {
		t.Fatal(err)
	}
	if got.TopicStyle != protocol.StyleAlink {
		t.Errorf("topic_style=%q want alink (short publish must not flip it)", got.TopicStyle)
	}
	samples, err := plat.QueryTelemetry(d.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Errorf("short publish ingested for alink device: %+v", samples)
	}
}
