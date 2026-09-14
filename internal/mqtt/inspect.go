package mqtt

import (
	"sort"
	"strings"

	mochi "github.com/mochi-mqtt/server/v2"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

const mqttEventCap = 250

type sessMeta struct {
	connectedAt int64
	lastSeen    int64
}

func (b *Broker) note(level, kind, clientID, username, remote, topic, msg string, nbytes int) {
	ev := models.MQTTEvent{
		TS:       db.NowMS(),
		Level:    level,
		Kind:     kind,
		ClientID: clientID,
		Username: username,
		Remote:   remote,
		Topic:    topic,
		Message:  msg,
		Bytes:    nbytes,
	}
	b.pushEvent(ev)

	attrs := []any{"kind", kind}
	if clientID != "" {
		attrs = append(attrs, "client_id", clientID)
	}
	if username != "" {
		attrs = append(attrs, "user", username)
	}
	if remote != "" {
		attrs = append(attrs, "remote", remote)
	}
	if topic != "" {
		attrs = append(attrs, "topic", topic)
	}
	if nbytes > 0 {
		attrs = append(attrs, "bytes", nbytes)
	}
	if msg != "" {
		attrs = append(attrs, "msg", msg)
	}
	switch level {
	case "error":
		b.log.Error("mqtt", attrs...)
	case "warn":
		b.log.Warn("mqtt", attrs...)
	case "debug":
		b.log.Debug("mqtt", attrs...)
	default:
		b.log.Info("mqtt", attrs...)
	}

	switch kind {
	case "connect", "disconnect", "auth_fail", "subscribe", "acl_deny", "kick":
		if b.plat != nil && b.plat.Hub != nil {
			b.plat.Hub.Broadcast("mqtt", ev)
		}
	}
}

func (b *Broker) pushEvent(ev models.MQTTEvent) {
	b.evMu.Lock()
	defer b.evMu.Unlock()
	b.events = append(b.events, ev)
	if len(b.events) > mqttEventCap {
		b.events = append([]models.MQTTEvent(nil), b.events[len(b.events)-mqttEventCap:]...)
	}
}

func (b *Broker) touchSession(clientID string) {
	now := db.NowMS()
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	if b.sessions == nil {
		b.sessions = map[string]*sessMeta{}
	}
	if s := b.sessions[clientID]; s != nil {
		s.lastSeen = now
		return
	}
	b.sessions[clientID] = &sessMeta{connectedAt: now, lastSeen: now}
}

func (b *Broker) bumpSession(clientID string) {
	now := db.NowMS()
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	if b.sessions == nil {
		b.sessions = map[string]*sessMeta{}
	}
	if s := b.sessions[clientID]; s != nil {
		s.lastSeen = now
		return
	}
	b.sessions[clientID] = &sessMeta{connectedAt: now, lastSeen: now}
}

func (b *Broker) forgetSession(clientID string) {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	delete(b.sessions, clientID)
}

func (b *Broker) sessionTimes(clientID string) (connectedAt, lastSeen int64) {
	b.sessMu.Lock()
	defer b.sessMu.Unlock()
	if s := b.sessions[clientID]; s != nil {
		return s.connectedAt, s.lastSeen
	}
	return 0, 0
}

func (b *Broker) clientKind(username string) string {
	if username == "" {
		return "other"
	}
	if strings.HasPrefix(username, "extplg_") {
		return "plugin"
	}
	if b.plat == nil {
		return "other"
	}
	if b.plat.ResolveByClientID(username) != nil || b.plat.ResolveByToken(username) != nil {
		return "device"
	}
	return "other"
}

func clientSubscriptions(cl *mochi.Client) []string {
	if cl == nil || cl.State.Subscriptions == nil {
		return []string{}
	}
	all := cl.State.Subscriptions.GetAll()
	out := make([]string, 0, len(all))
	for filter := range all {
		out = append(out, filter)
	}
	sort.Strings(out)
	return out
}

func (b *Broker) Status() models.MQTTStatus {
	addr := b.cfg.MQTT.PublicURL(b.cfg.Platform.BrokerHostPublic)
	if addr == "" {
		addr = b.listenAddr
	}
	if addr == "" {
		addr = b.cfg.MQTT.Addr
	}
	st := models.MQTTStatus{
		Listening: b.ok.Load(),
		Addr:      addr,
		Clients:   []models.MQTTClient{},
		Events:    []models.MQTTEvent{},
	}
	if b.cfg.MQTT.WSEnabled() {
		st.WSAddr = b.cfg.MQTT.WSPublicURL(b.cfg.Platform.BrokerHostPublic)
		if st.WSAddr == "" {
			st.WSAddr = b.cfg.MQTT.WSAddr
		}
	}
	if b.cfg.MQTT.TLSEnabled() {
		st.TLSAddr = b.cfg.MQTT.TLSPublicURL(b.cfg.Platform.BrokerHostPublic)
		if st.TLSAddr == "" {
			st.TLSAddr = b.tlsListenAddr
		}
	}
	if b.server != nil {
		for _, cl := range b.server.Clients.GetAll() {
			if cl.Net.Inline {
				continue
			}
			user := string(cl.Properties.Username)
			kind := b.clientKind(user)
			var devicePK int64
			if kind == "device" && b.plat != nil {
				if d := b.plat.ResolveByClientID(user); d != nil {
					devicePK = d.ID
				} else if d := b.plat.ResolveByToken(user); d != nil {
					devicePK = d.ID
				}
			}
			connectedAt, lastSeen := b.sessionTimes(cl.ID)
			st.Clients = append(st.Clients, models.MQTTClient{
				ClientID:      cl.ID,
				Username:      user,
				Remote:        cl.Net.Remote,
				Listener:      cl.Net.Listener,
				Kind:          kind,
				DevicePK:      devicePK,
				Protocol:      cl.Properties.ProtocolVersion,
				Clean:         cl.Properties.Clean,
				Subscriptions: clientSubscriptions(cl),
				ConnectedAt:   connectedAt,
				LastSeenAt:    lastSeen,
			})
		}
		sort.Slice(st.Clients, func(i, j int) bool {
			if st.Clients[i].Kind != st.Clients[j].Kind {
				return st.Clients[i].Kind < st.Clients[j].Kind
			}
			return st.Clients[i].Username < st.Clients[j].Username
		})
	}
	st.ClientCount = len(st.Clients)

	b.evMu.Lock()
	if n := len(b.events); n > 0 {
		st.Events = make([]models.MQTTEvent, n)
		for i, ev := range b.events {
			st.Events[n-1-i] = ev
		}
	}
	b.evMu.Unlock()
	return st
}
