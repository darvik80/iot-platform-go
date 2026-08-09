package mqtt

import (
	"crypto/subtle"
	"fmt"
	"strings"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	"crearts-iot-platform/internal/auth"
	"crearts-iot-platform/internal/models"
	"crearts-iot-platform/internal/protocol"
)

type platHook struct {
	mochi.HookBase
	b *Broker
}

func (h *platHook) ID() string { return "crearts-auth" }

func (h *platHook) Provides(b byte) bool {
	switch b {
	case mochi.OnConnect, mochi.OnConnectAuthenticate, mochi.OnACLCheck,
		mochi.OnSessionEstablished, mochi.OnDisconnect, mochi.OnPublished, mochi.OnSubscribed:
		return true
	default:
		return false
	}
}

func (h *platHook) OnConnect(cl *mochi.Client, pk packets.Packet) error {
	if cl.Net.Inline {
		return nil
	}
	user := string(pk.Connect.Username)
	cid := pk.Connect.ClientIdentifier
	h.b.note("info", "connect_attempt", cid, user, cl.Net.Remote, pk.Connect.WillTopic,
		fmt.Sprintf("proto=%d keepalive=%d clean=%v will=%v pass=%v",
			pk.ProtocolVersion, pk.Connect.Keepalive, pk.Connect.Clean, pk.Connect.WillFlag, len(pk.Connect.Password) > 0), 0)
	return nil
}

func (h *platHook) OnConnectAuthenticate(cl *mochi.Client, pk packets.Packet) bool {
	if cl.Net.Inline {
		return true
	}
	user := string(pk.Connect.Username)
	pass := string(pk.Connect.Password)
	remote := cl.Net.Remote
	clientID := pk.Connect.ClientIdentifier
	if clientID == "" {
		clientID = cl.ID
	}
	if user == "" || pass == "" {
		h.b.note("warn", "auth_fail", clientID, user, remote, "", "missing username or password", 0)
		return false
	}
	if user == h.b.cfg.NodeRedMQTTUser {
		want := h.b.cfg.NodeRedMQTTPassword
		if want == "" || subtle.ConstantTimeCompare([]byte(pass), []byte(want)) != 1 {
			h.b.note("warn", "auth_fail", clientID, user, remote, "", "nodered bad password", 0)
			return false
		}
		return true
	}
	d := h.deviceFor(user)
	if d == nil {
		h.b.note("warn", "auth_fail", clientID, user, remote, "", "unknown device", 0)
		return false
	}
	if !d.Enabled {
		h.b.note("warn", "auth_fail", clientID, user, remote, "", "device disabled", 0)
		return false
	}
	if d.AccessTokenHash == "" {
		h.b.note("warn", "auth_fail", clientID, user, remote, "", "no token hash", 0)
		return false
	}
	got := auth.HashToken(pass)
	if subtle.ConstantTimeCompare([]byte(got), []byte(d.AccessTokenHash)) != 1 {
		h.b.note("warn", "auth_fail", clientID, user, remote, "", "bad token", 0)
		return false
	}
	if style := protocol.DetectStyle(pk.Connect.WillTopic); style != "" {
		h.b.plat.ObserveTopicStyle(d, style)
	}
	return true
}

func (h *platHook) OnACLCheck(cl *mochi.Client, topic string, write bool) bool {
	if cl.Net.Inline {
		return true
	}
	user := string(cl.Properties.Username)
	ok := h.aclOK(user, topic, write)
	if !ok {
		action := "subscribe"
		if write {
			action = "publish"
		}
		h.b.note("warn", "acl_deny", cl.ID, user, cl.Net.Remote, topic, action+" denied", 0)
	}
	return ok
}

func (h *platHook) aclOK(user, topic string, write bool) bool {
	if user == "" || strings.HasPrefix(topic, "$") {
		return false
	}
	if user == h.b.cfg.NodeRedMQTTUser {
		prefix := strings.TrimRight(h.b.cfg.RulesBusPrefix, "/")
		if write {
			return topicUnder(topic, protocol.RulesCommandsRoot(prefix))
		}
		return topicUnder(topic, protocol.RulesEventsRoot(prefix))
	}
	d := h.deviceFor(user)
	if d == nil || !d.Enabled {
		return false
	}
	up := protocol.FullUpRoot(d.ProductID, d.DeviceID)
	down := protocol.FullDownRoot(d.ProductID, d.DeviceID)
	if write {
		return topicUnder(topic, up) || topicUnder(topic, protocol.PrefixShort)
	}
	return topicUnder(topic, down) || topicUnder(topic, protocol.PrefixShort)
}

func (h *platHook) OnSessionEstablished(cl *mochi.Client, _ packets.Packet) {
	if cl.Net.Inline {
		return
	}
	user := string(cl.Properties.Username)
	remote := cl.Net.Remote
	h.b.touchSession(cl.ID)
	h.b.note("info", "connect", cl.ID, user, remote, "", h.b.clientKind(user), 0)
	if user == "" || user == h.b.cfg.NodeRedMQTTUser {
		return
	}
	d := h.deviceFor(user)
	if d == nil || !d.Enabled {
		return
	}
	if d.Status != "online" {
		h.b.plat.MarkOnline(d)
	}
	h.b.plat.RememberMQTTPeer(d, remote)
}

func (h *platHook) OnDisconnect(cl *mochi.Client, err error, _ bool) {
	if cl.Net.Inline {
		return
	}
	user := string(cl.Properties.Username)
	reason := "disconnect"
	if err != nil {
		reason = err.Error()
	}
	h.b.note("info", "disconnect", cl.ID, user, cl.Net.Remote, "", reason, 0)
	h.b.forgetSession(cl.ID)
	if user == "" || user == h.b.cfg.NodeRedMQTTUser {
		return
	}
	d := h.deviceFor(user)
	if d == nil || d.Status != "online" {
		return
	}
	h.b.plat.MarkOffline(d, "disconnect")
}

func (h *platHook) OnSubscribed(cl *mochi.Client, pk packets.Packet, _ []byte) {
	if cl.Net.Inline {
		return
	}
	filters := make([]string, 0, len(pk.Filters))
	for _, f := range pk.Filters {
		filters = append(filters, f.Filter)
	}
	h.b.bumpSession(cl.ID)
	user := string(cl.Properties.Username)
	h.b.note("info", "subscribe", cl.ID, user, cl.Net.Remote, strings.Join(filters, ","), "", 0)
	if user == "" || user == h.b.cfg.NodeRedMQTTUser {
		return
	}
	for _, f := range filters {
		if style := protocol.DetectStyle(f); style != "" {
			if d := h.deviceFor(user); d != nil {
				h.b.plat.ObserveTopicStyle(d, style)
			}
			break
		}
	}
}

func (h *platHook) OnPublished(cl *mochi.Client, pk packets.Packet) {
	if cl.Net.Inline {
		return
	}
	topic := pk.TopicName
	if h.b.cfg.RulesBusEnabled && strings.HasPrefix(topic, protocol.RulesEventsRoot(h.b.cfg.RulesBusPrefix)) {
		return
	}
	user := string(cl.Properties.Username)
	payload := append([]byte(nil), pk.Payload...)
	h.b.bumpSession(cl.ID)
	go h.b.handle(topic, payload, user)
}

func (h *platHook) deviceFor(username string) *models.Device {
	if d := h.b.plat.ResolveByClientID(username); d != nil {
		return d
	}
	return h.b.plat.ResolveByToken(username)
}

// topicUnder reports whether topicOrFilter is inside root (MQTT filter #/+ allowed).
func topicUnder(topicOrFilter, root string) bool {
	root = strings.TrimSuffix(root, "/")
	if root == "" || topicOrFilter == "#" || topicOrFilter == "+" {
		return false
	}
	trimmed := strings.TrimSuffix(topicOrFilter, "/#")
	trimmed = strings.TrimSuffix(trimmed, "/+")
	if trimmed == root {
		return true
	}
	return strings.HasPrefix(topicOrFilter, root+"/")
}
