package mqtt

import (
	"encoding/json"
	"fmt"
	"strings"

	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/models"
	"crearts-iot-platform/internal/services"
)

func (b *Broker) handle(topic string, payload []byte, userID string) {
	recv := db.NowMS()
	if strings.Contains(topic, "/commands/") {
		b.handleCommand(topic, payload)
		return
	}
	dev, capOp, err := b.resolve(topic, userID)
	if err != nil || dev == nil {
		msg := "unresolved"
		if err != nil {
			msg = err.Error()
		}
		b.note("warn", "drop", "", userID, "", topic, msg, len(payload))
		return
	}
	level := "info"
	if capOp == "telemetry/data" || capOp == "t" {
		level = "debug"
	}
	b.note(level, "ingest", "", userID, "", topic, capOp, len(payload))
	switch capOp {
	case "status", "s":
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
	case "telemetry/data", "t":
		_ = b.plat.StoreTelemetry(dev, payload)
	case "events/post", "e":
		_ = b.plat.StoreEvent(dev, payload)
	case "attributes/report", "a":
		var attrs map[string]any
		if err := json.Unmarshal(payload, &attrs); err == nil {
			_ = b.plat.MergeReported(dev, attrs)
		}
	case "attributes/request", "a/req":
		_ = b.plat.ReplyAttributeRequest(dev, payload)
	case "rpc/response", "r/res":
		_ = b.plat.CompleteRPC(dev, payload)
	case "rpc/request", "r/creq":
		_ = b.plat.HandleClientRPC(dev, payload)
	case "ntp/request", "n/req":
		_ = b.plat.ReplyNTP(dev, payload, recv)
	case "ota/version", "o/ver":
		_ = b.plat.HandleOTAVersion(dev, payload)
	case "ota/query", "o/q":
		_ = b.plat.HandleOTAQuery(dev, payload)
	case "ota/progress", "o/p":
		_ = b.plat.HandleOTAProgress(dev, payload)
	case "logs/report", "l":
		_ = b.plat.StoreLogs(dev, payload)
	default:
		b.note("debug", "unhandled", "", userID, "", topic, capOp, len(payload))
	}
}

func (b *Broker) resolve(topic, userID string) (*models.Device, string, error) {
	parts := strings.Split(topic, "/")
	if len(parts) >= 6 && parts[0] == "iot" && parts[1] == "v1" && parts[4] == "up" {
		productID, deviceID := parts[2], parts[3]
		capOp := strings.Join(parts[5:], "/")
		d, err := b.plat.GetDeviceByIdentity(productID, deviceID)
		if err != nil {
			return nil, "", err
		}
		if !d.Enabled {
			return nil, "", fmt.Errorf("disabled")
		}
		if d.Status != "online" {
			b.plat.MarkOnline(d)
		}
		b.plat.ObserveTopicStyle(d, "full")
		return d, capOp, nil
	}
	if len(parts) >= 2 && parts[0] == "v1" {
		capOp := shortCapOp(strings.Join(parts[1:], "/"))
		var d *models.Device
		if userID != "" && userID != b.cfg.NodeRedMQTTUser {
			d = b.plat.ResolveByClientID(userID)
			if d == nil {
				d = b.plat.ResolveByToken(userID)
			}
		}
		if d == nil {
			return nil, "", fmt.Errorf("cannot resolve short-topic device (no username)")
		}
		if !d.Enabled {
			return nil, "", fmt.Errorf("disabled")
		}
		if d.Status != "online" {
			b.plat.MarkOnline(d)
		}
		b.plat.ObserveTopicStyle(d, "short")
		return d, capOp, nil
	}
	return nil, "", fmt.Errorf("unknown topic shape")
}

func (b *Broker) handleCommand(topic string, payload []byte) {
	parts := strings.Split(topic, "/")
	idx := -1
	for i, p := range parts {
		if p == "commands" {
			idx = i
			break
		}
	}
	if idx < 0 || len(parts) < idx+3 {
		return
	}
	productID := parts[idx+1]
	deviceID := parts[idx+2]
	kind := strings.Join(parts[idx+3:], "/")
	d, err := b.plat.GetDeviceByIdentity(productID, deviceID)
	if err != nil {
		b.note("warn", "command", "", "", "", topic, "unknown device "+productID+"."+deviceID, len(payload))
		return
	}
	b.note("info", "command", "", "", "", topic, kind, len(payload))
	switch kind {
	case "rpc":
		var req services.RPCRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return
		}
		_, _ = b.plat.InvokeRPC(d, req, false)
	case "attributes/desired":
		var attrs map[string]any
		if err := json.Unmarshal(payload, &attrs); err != nil {
			return
		}
		_ = b.plat.SetDesired(d, attrs, true)
	case "ota":
		var body struct {
			FirmwareID int64 `json:"firmware_id"`
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return
		}
		_, _ = b.plat.StartOTAJob(d.ID, body.FirmwareID, false)
	}
}
