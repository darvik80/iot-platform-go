package mqtt

import (
	"testing"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
	"cogitor-iot-platform/internal/services"
)

// connectPacket builds the packet OnConnectAuthenticate receives on a real CONNECT.
func connectPacket(user, pass, clientID, willTopic string) packets.Packet {
	return packets.Packet{
		Connect: packets.ConnectParams{
			Username:         []byte(user),
			Password:         []byte(pass),
			ClientIdentifier: clientID,
			WillTopic:        willTopic,
			Keepalive:        60,
		},
	}
}

// fakeClient builds the mochi client the auth hook sees, without a live connection.
func fakeClient(id, user, remote string) *mochi.Client {
	return &mochi.Client{
		ID:   id,
		Net:  mochi.ClientConnection{Remote: remote},
		Properties: mochi.ClientProperties{Username: []byte(user)},
	}
}

func newAuthHook(t *testing.T) (*Broker, *services.Platform, *models.Device, string, *platHook) {
	t.Helper()
	b, plat, alice, _ := newIsolationBroker(t)
	creds, err := plat.RotateDeviceToken(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := &platHook{b: b}
	return b, plat, alice, creds.AccessToken, h
}

func TestAuthNativeDeviceToken(t *testing.T) {
	_, plat, alice, token, h := newAuthHook(t)
	user := alice.ProductID + "." + alice.DeviceID

	if !h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, token, "c1", "")) {
		t.Error("valid token rejected")
	}
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, "wrong", "c1", "")) {
		t.Error("bad token accepted")
	}
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket("", token, "c1", "")) {
		t.Error("empty username accepted")
	}
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, "", "c1", "")) {
		t.Error("empty password accepted")
	}
	if h.OnConnectAuthenticate(fakeClient("c1", "nosuch.dev", "127.0.0.1:1"), connectPacket("nosuch.dev", token, "c1", "")) {
		t.Error("unknown username accepted")
	}

	if err := plat.SetDeviceEnabled(alice.ID, false); err != nil {
		t.Fatal(err)
	}
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, token, "c1", "")) {
		t.Error("disabled device accepted")
	}
}

func TestAuthNativeTokenAsUsername(t *testing.T) {
	_, _, _, token, h := newAuthHook(t)
	// Some SDKs connect with the raw access token as username.
	if !h.OnConnectAuthenticate(fakeClient("c1", token, "127.0.0.1:1"), connectPacket(token, token, "c1", "")) {
		t.Error("token-as-username rejected")
	}
}

func TestAuthSuspendedAccountRejected(t *testing.T) {
	_, plat, alice, token, h := newAuthHook(t)
	user := alice.ProductID + "." + alice.DeviceID
	if _, err := plat.DB.Exec(`UPDATE accounts SET status='suspended' WHERE id=?`, alice.AccountID); err != nil {
		t.Fatal(err)
	}
	if h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"), connectPacket(user, token, "c1", "")) {
		t.Error("suspended account accepted")
	}
}

func TestAuthWillTopicDetectsStyle(t *testing.T) {
	_, _, alice, token, h := newAuthHook(t)
	user := alice.ProductID + "." + alice.DeviceID

	if !h.OnConnectAuthenticate(fakeClient("c1", user, "127.0.0.1:1"),
		connectPacket(user, token, "c1", protocol.FullUpRoot(alice.ProductID, alice.DeviceID)+protocol.UpStatus)) {
		t.Fatal("connect with full-style will rejected")
	}
	d, err := h.b.plat.GetDevice(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.TopicStyle != protocol.StyleFull {
		t.Errorf("style=%q want full (from WillTopic)", d.TopicStyle)
	}

	// Will on the short dialect switches the stored style back.
	if !h.OnConnectAuthenticate(fakeClient("c2", user, "127.0.0.1:1"),
		connectPacket(user, token, "c2", protocol.ShortMe(protocol.ShortStatus))) {
		t.Fatal("connect with short-style will rejected")
	}
	if d, err = h.b.plat.GetDevice(alice.ID); err != nil {
		t.Fatal(err)
	}
	if d.TopicStyle != protocol.StyleShort {
		t.Errorf("style=%q want short (from WillTopic)", d.TopicStyle)
	}
}

func TestACLNativeDevice(t *testing.T) {
	_, _, alice, token, h := newAuthHook(t)
	user := alice.ProductID + "." + alice.DeviceID
	foreign := alice.ProductID + ".other"
	up := protocol.FullUpRoot(alice.ProductID, alice.DeviceID)
	down := protocol.FullDownRoot(alice.ProductID, alice.DeviceID)

	cases := []struct {
		name    string
		user    string
		topic   string
		write   bool
		allowed bool
	}{
		{"own uplink", user, up + protocol.UpTelemetry, true, true},
		{"own uplink filter", user, up + "#", true, true},
		{"own downlink read", user, down + protocol.CapRPC + "/" + protocol.OpRequest, false, true},
		{"foreign device uplink", user, protocol.FullUpRoot("home", "h1") + protocol.UpTelemetry, true, false},
		{"foreign device downlink", user, protocol.FullDownRoot("home", "h1") + "x", false, false},
		{"own short publish", user, protocol.ShortMe(protocol.ShortTelemetry), true, true},
		{"short subscribe", user, protocol.PrefixShort + "#", false, true},
		{"sys topic", user, "$SYS/broker/uptime", false, false},
		{"sys publish", user, "$SYS/x", true, false},
		{"unknown user", foreign, up + protocol.UpTelemetry, true, false},
		{"empty user", "", up + protocol.UpTelemetry, true, false},
		{"unrelated prefix", user, "random/topic", true, false},
	}
	for _, tc := range cases {
		if got := h.aclOK(tc.user, tc.topic, tc.write); got != tc.allowed {
			t.Errorf("%s: aclOK(%q, %q, write=%v)=%v want %v", tc.name, tc.user, tc.topic, tc.write, got, tc.allowed)
		}
	}
	_ = token

	// OnACLCheck wraps aclOK with the same semantics per client.
	cl := fakeClient("c1", user, "127.0.0.1:1")
	if !h.OnACLCheck(cl, up+protocol.UpTelemetry, true) {
		t.Error("OnACLCheck denied own uplink")
	}
	if h.OnACLCheck(cl, protocol.FullUpRoot("home", "h1")+protocol.UpTelemetry, true) {
		t.Error("OnACLCheck allowed foreign uplink")
	}
	// Inline clients (platform itself) may publish anywhere.
	inline := fakeClient("inline", "platform", "")
	inline.Net.Inline = true
	if !h.OnACLCheck(inline, "any/topic", true) {
		t.Error("inline client denied")
	}
}
