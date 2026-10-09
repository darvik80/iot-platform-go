package mqtt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mochi-mqtt/server/v2/packets"

	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
	"cogitor-iot-platform/internal/services"
)

// newTBBroker provisions one thingsboard-style device in alice's account and
// captures downlinks. TB sessions connect with the raw access token as the
// MQTT username.
func newTBBroker(t *testing.T) (*Broker, *services.Platform, *models.Device, *capturePub, string) {
	t.Helper()
	b, plat, alice, _ := newIsolationBroker(t)
	d, creds, err := plat.CreateDevice(alice.AccountID, services.DeviceInput{
		ProductID:  "lamp",
		DeviceID:   "tb1",
		TopicStyle: protocol.StyleThingsBoard,
	})
	if err != nil {
		t.Fatal(err)
	}
	pub := newCapturePub()
	plat.SetPublisher(pub)
	return b, plat, d, pub, creds.AccessToken
}

func TestAuthThingsBoard(t *testing.T) {
	b, plat, d, _, token := newTBBroker(t)
	h := &platHook{b: b}
	remote := "127.0.0.1:1"

	if !h.OnConnectAuthenticate(fakeClient("c1", token, remote), connectPacket(token, "", "c1", "")) {
		t.Error("token username with empty password rejected")
	}
	if !h.OnConnectAuthenticate(fakeClient("c1", token, remote), connectPacket(token, "stray", "c1", "")) {
		t.Error("stray password rejected: TB sends no password")
	}
	if h.OnConnectAuthenticate(fakeClient("c1", "not-a-token", remote), connectPacket("not-a-token", "", "c1", "")) {
		t.Error("unknown token accepted")
	}
	// the native product.device username must not resolve for a TB device:
	// Usernames() is nil, so only the token map knows this device
	if h.OnConnectAuthenticate(fakeClient("c1", "lamp.tb1", remote), connectPacket("lamp.tb1", token, "c1", "")) {
		t.Error("native product.device username accepted for a thingsboard device")
	}

	if err := plat.SetDeviceEnabled(d.ID, false); err != nil {
		t.Fatal(err)
	}
	if h.OnConnectAuthenticate(fakeClient("c1", token, remote), connectPacket(token, "", "c1", "")) {
		t.Error("disabled device accepted")
	}
}

func TestACLThingsBoard(t *testing.T) {
	b, _, _, _, token := newTBBroker(t)
	h := &platHook{b: b}

	cases := []struct {
		name    string
		topic   string
		write   bool
		allowed bool
	}{
		{"telemetry publish", "v1/devices/me/telemetry", true, true},
		{"attributes publish", "v1/devices/me/attributes", true, true},
		{"attr request publish", "v1/devices/me/attributes/request/43", true, true},
		{"rpc response v1 publish", "v1/devices/me/rpc/response/7", true, true},
		{"rpc response v2 publish", "v2/devices/me/rpc/response/7", true, true},
		{"attributes subscribe", "v1/devices/me/attributes", false, true},
		{"rpc request v1 subscribe", "v1/devices/me/rpc/request/+", false, true},
		{"rpc request v2 subscribe", "v2/devices/me/rpc/request/+", false, true},
		{"rpc request publish", "v1/devices/me/rpc/request/7", true, false},
		{"short publish", "v1/me/t", true, false},
		{"short subscribe all", "v1/#", false, false},
		{"full publish", "up/lamp/tb1/telemetry/data", true, false},
		{"sys publish", "$SYS/x", true, false},
		{"random topic", "random/topic", true, false},
		{"empty user", "v1/devices/me/telemetry", true, false},
	}
	for _, tc := range cases {
		user := token
		if tc.name == "empty user" {
			user = ""
		}
		if got := h.aclOK(user, tc.topic, tc.write); got != tc.allowed {
			t.Errorf("%s: aclOK(%q, %q, write=%v)=%v want %v", tc.name, user, tc.topic, tc.write, got, tc.allowed)
		}
	}
}

func TestTBTelemetryIngest(t *testing.T) {
	b, plat, d, _, token := newTBBroker(t)

	b.handle("v1/devices/me/telemetry", []byte(`{"temp":21.5,"mode":"heat"}`), token)

	samples, err := plat.QueryTelemetry(d.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("want 2 samples, got %d: %+v", len(samples), samples)
	}
	if s := sampleOf(t, samples, "temp"); s.ValueNum == nil || *s.ValueNum != 21.5 {
		t.Errorf("temp=%+v", s)
	}
	if s := sampleOf(t, samples, "mode"); s.ValueText == nil || *s.ValueText != "heat" {
		t.Errorf("mode=%+v", s)
	}
	got, err := plat.GetDevice(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "online" {
		t.Errorf("status=%q want online", got.Status)
	}
	if got.TopicStyle != protocol.StyleThingsBoard {
		t.Errorf("topic_style=%q want thingsboard", got.TopicStyle)
	}
}

func TestTBAttributesIngest(t *testing.T) {
	b, plat, d, _, token := newTBBroker(t)

	b.handle("v1/devices/me/attributes", []byte(`{"fw":"1.0.0","model":"L-100"}`), token)

	reported, _, err := plat.GetAttributes(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reported["fw"] != "1.0.0" || reported["model"] != "L-100" {
		t.Errorf("reported=%+v", reported)
	}
}

func TestTBAttrRequestRoundTrip(t *testing.T) {
	b, plat, d, pub, token := newTBBroker(t)

	b.handle("v1/devices/me/attributes", []byte(`{"fw":"1.0.0"}`), token)
	if err := plat.SetDesired(d, map[string]any{"led_color": "warm-white"}, false); err != nil {
		t.Fatal(err)
	}

	b.handle("v1/devices/me/attributes/request/43", []byte(`{"sharedKeys":["led_color"],"clientKeys":["fw","missing"]}`), token)

	m := pub.wait(t)
	if m.topic != "v1/devices/me/attributes/response/43" {
		t.Errorf("downlink topic=%q want v1/devices/me/attributes/response/43", m.topic)
	}
	if m.devicePK != d.ID {
		t.Errorf("downlink devicePK=%d want %d (DownViaDevice)", m.devicePK, d.ID)
	}
	if m.username != "" {
		t.Errorf("downlink username=%q want empty (no static username)", m.username)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(m.payload, &body); err != nil {
		t.Fatal(err)
	}
	if body["client"]["fw"] != "1.0.0" {
		t.Errorf("client=%+v", body["client"])
	}
	if len(body["client"]) != 1 {
		t.Errorf("unknown key not dropped: %+v", body["client"])
	}
	if body["shared"]["led_color"] != "warm-white" {
		t.Errorf("shared=%+v", body["shared"])
	}
}

func TestTBRPCRoundTrip(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			b, plat, d, pub, token := newTBBroker(t)

			done := make(chan *models.RPCCall, 1)
			go func() {
				call, err := plat.InvokeRPC(d, services.RPCRequest{Method: "reboot", Params: map[string]any{"delay": 5}, TimeoutMS: 3000}, true)
				if err != nil {
					close(done)
					return
				}
				done <- call
			}()

			m := pub.wait(t)
			if m.devicePK != d.ID {
				t.Errorf("downlink devicePK=%d want %d (DownViaDevice)", m.devicePK, d.ID)
			}
			if m.username != "" {
				t.Errorf("downlink username=%q want empty", m.username)
			}
			rest, ok := strings.CutPrefix(m.topic, "v1/devices/me/rpc/request/")
			if !ok {
				t.Fatalf("downlink topic=%q want v1/devices/me/rpc/request/$id", m.topic)
			}
			if rest == "" || strings.ContainsAny(rest, "/") {
				t.Fatalf("request id=%q", rest)
			}
			var body struct {
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := json.Unmarshal(m.payload, &body); err != nil {
				t.Fatal(err)
			}
			if body.Method != "reboot" || body.Params["delay"] != float64(5) {
				t.Errorf("downlink body=%+v", body)
			}
			if strings.Contains(string(m.payload), `"id"`) {
				t.Errorf("TB v1 RPC body must drop the id: %s", m.payload)
			}

			b.handle(version+"/devices/me/rpc/response/"+rest, []byte(`{"result":"rebooting"}`), token)

			select {
			case call := <-done:
				if call == nil {
					t.Fatal("InvokeRPC failed")
				}
				if call.Status != "ok" {
					t.Errorf("status=%q want ok", call.Status)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("RPC waiter not released")
			}
		})
	}
}

func TestTBDesiredPushDownlink(t *testing.T) {
	_, plat, d, pub, _ := newTBBroker(t)

	if err := plat.SetDesired(d, map[string]any{"led_color": "warm-white"}, true); err != nil {
		t.Fatal(err)
	}

	m := pub.wait(t)
	if m.topic != "v1/devices/me/attributes" {
		t.Errorf("downlink topic=%q want v1/devices/me/attributes", m.topic)
	}
	if m.devicePK != d.ID {
		t.Errorf("downlink devicePK=%d want %d (DownViaDevice)", m.devicePK, d.ID)
	}
	var attrs map[string]any
	if err := json.Unmarshal(m.payload, &attrs); err != nil {
		t.Fatal(err)
	}
	if attrs["led_color"] != "warm-white" {
		t.Errorf("payload=%+v", attrs)
	}
}

func TestTBUnknownSessionDropped(t *testing.T) {
	b, plat, d, _, _ := newTBBroker(t)

	b.handle("v1/devices/me/telemetry", []byte(`{"temp":1}`), "not-a-real-token")

	samples, err := plat.QueryTelemetry(d.ID, nil, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Errorf("unknown session stored %d samples", len(samples))
	}
}

// TB is pinned: neither a short-style Will topic, a subscription whose filter
// DetectStyle reads as "short" (v1/…), nor a short-shape publish may flip the
// stored style. The ACL blocks the short publish in production; here we check
// the guard itself. Style only — a TB dialect passes foreign payloads through,
// so samples may exist.
func TestTBStyleNotAutoFlipped(t *testing.T) {
	b, plat, d, _, token := newTBBroker(t)
	h := &platHook{b: b}
	remote := "127.0.0.1:1"

	if !h.OnConnectAuthenticate(fakeClient("c1", token, remote),
		connectPacket(token, "", "c1", protocol.ShortMe(protocol.ShortStatus))) {
		t.Fatal("connect with short-style will rejected")
	}
	h.OnSubscribed(fakeClient("c1", token, remote),
		packets.Packet{Filters: packets.Subscriptions{{Filter: "v1/devices/me/attributes"}}}, nil)

	b.handle("v1/me/t", []byte(`{"temp":1}`), token)

	got, err := plat.GetDevice(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TopicStyle != protocol.StyleThingsBoard {
		t.Errorf("topic_style=%q want thingsboard (pinned)", got.TopicStyle)
	}
}
