package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
	"cogitor-iot-platform/internal/services"
)

func (b *Broker) handle(topic string, payload []byte, userID string) {
	recv := db.NowMS()
	if strings.HasPrefix(userID, "extplg_") {
		b.handleCommand(topic, payload, userID)
		return
	}
	dev, kind, aux, err := b.resolve(topic, userID)
	if err != nil || dev == nil {
		msg := "unresolved"
		if err != nil {
			msg = err.Error()
		}
		b.note("warn", "drop", "", userID, "", topic, msg, len(payload))
		return
	}
	dl := protocol.DialectFor(dev.TopicStyle)
	payload, err = dl.Uplink(kind, aux, payload)
	if err != nil {
		b.note("warn", "drop", "", userID, "", topic, "uplink: "+err.Error(), len(payload))
		return
	}
	b.note("info", "ingest", "", userID, "", topic, kind, len(payload))
	switch kind {
	case protocol.KindStatus:
		var st struct {
			Online bool   `json:"online"`
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(payload, &st)
		if !st.Online {
			reason := st.Reason
			if reason == "" {
				reason = "lwt"
			}
			b.plat.MarkOffline(dev, reason)
		}
	case protocol.KindTelemetry:
		_ = b.plat.StoreTelemetry(dev, payload)
	case protocol.KindEvent:
		if err := b.plat.StoreEvent(dev, payload); err != nil {
			b.note("warn", "event_error", "", userID, "", topic, err.Error(), len(payload))
		}
	case protocol.KindAttrReport:
		var attrs map[string]any
		if err := json.Unmarshal(payload, &attrs); err == nil {
			_ = b.plat.MergeReported(dev, attrs)
		}
	case protocol.KindAttrGet:
		if err := b.plat.ReplyDesiredGet(dev, payload); err != nil {
			b.note("warn", "desired_get_error", "", userID, "", topic, err.Error(), len(payload))
		}
	case protocol.KindAttrDelete:
		if err := b.plat.DeleteDesired(dev, payload); err != nil {
			b.note("warn", "desired_delete_error", "", userID, "", topic, err.Error(), len(payload))
		}
	case protocol.KindAttrRequest:
		_ = b.plat.ReplyAttributeRequest(dev, payload)
	case protocol.KindRPCResponse:
		_ = b.plat.CompleteRPC(dev, payload)
	case protocol.KindRPCRequest:
		_ = b.plat.HandleClientRPC(dev, payload)
	case protocol.KindNTPRequest:
		if err := b.plat.ReplyNTP(dev, payload, recv); err != nil {
			b.note("warn", "ntp_reply", "", userID, "", topic, err.Error(), len(payload))
		}
	case protocol.KindOTAVersion:
		_ = b.plat.HandleOTAVersion(dev, payload)
	case protocol.KindOTAQuery:
		_ = b.plat.HandleOTAQuery(dev, payload)
	case protocol.KindOTAProgress:
		_ = b.plat.HandleOTAProgress(dev, payload)
	case protocol.KindLogs:
		_ = b.plat.StoreLogs(dev, payload)
	default:
		b.note("debug", "unhandled", "", userID, "", topic, kind, len(payload))
	}
}

// resolve maps an uplink topic to the session's device and the internal
// kind. Identity comes from the authenticated session, not from the topic:
// product/device ids are only unique within an account. For dialects whose
// topics encode identity it is still verified against the session.
func (b *Broker) resolve(topic, userID string) (*models.Device, string, string, error) {
	up, dl, ok := protocol.ParseUplinkTopic(topic)
	if !ok {
		return nil, "", "", fmt.Errorf("unknown topic shape")
	}
	var d *models.Device
	if userID != "" && !strings.HasPrefix(userID, "extplg_") {
		d = b.plat.ResolveByClientID(userID)
		if d == nil {
			d = b.plat.ResolveByToken(userID)
		}
	}
	if d == nil || (up.ProductID != "" && (d.ProductID != up.ProductID || d.DeviceID != up.DeviceID)) {
		return nil, "", "", fmt.Errorf("topic does not belong to session %q", userID)
	}
	if !d.Enabled {
		return nil, "", "", fmt.Errorf("disabled")
	}
	if d.Status != "online" {
		b.plat.MarkOnline(d)
	}
	b.plat.ObserveTopicStyle(d, dl.Name())
	return d, up.Kind, up.Aux, nil
}

// handleCommand executes {prefix}/commands/{accountSlug}/{productID}/{deviceID}/{kind}
// published by an external plugin; the device must belong to the plugin's account.
func (b *Broker) handleCommand(topic string, payload []byte, user string) {
	slug := extPluginSlug(user)
	root := protocol.RulesCommandsRootForAccount(strings.TrimRight(b.cfg.Rules.Prefix, "/"), slug)
	rest, ok := strings.CutPrefix(topic, root)
	if slug == "" || !ok {
		b.note("warn", "command", "", user, "", topic, "not under "+root, len(payload))
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		b.note("warn", "command", "", user, "", topic, "want {product}/{device}/{kind}", len(payload))
		return
	}
	productID, deviceID := parts[0], parts[1]
	kind := strings.Join(parts[2:], "/")
	d, err := b.plat.GetDeviceByAccountIdentity(slug, productID, deviceID)
	if err != nil {
		b.note("warn", "command", "", user, "", topic, "unknown device "+slug+"/"+productID+"."+deviceID, len(payload))
		return
	}
	b.note("info", "command", "", user, "", topic, kind, len(payload))
	switch kind {
	case protocol.CmdRPC:
		var req services.RPCRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return
		}
		_, _ = b.plat.InvokeRPC(d, req, false)
	case protocol.CmdAttributesDesired:
		var attrs map[string]any
		if err := json.Unmarshal(payload, &attrs); err != nil {
			return
		}
		_ = b.plat.SetDesired(d, attrs, true)
	case protocol.CmdOTA:
		var body struct {
			FirmwareID int64 `json:"firmware_id"`
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return
		}
		_, _ = b.plat.StartOTAJob(d.ID, body.FirmwareID, false)
	}
}
