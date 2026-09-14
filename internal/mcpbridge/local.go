package mcpbridge

import (
	"context"
	"fmt"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/services"
)

// Local calls Platform services in-process (embedded HTTP MCP on the platform).
type Local struct {
	Plat *services.Platform
	Role models.Role // empty = no role gate (trusted stdio-style); set from JWT user when embedded
}

func NewLocal(plat *services.Platform, role models.Role) *Local {
	return &Local{Plat: plat, Role: role}
}

func (l *Local) requireWrite() error {
	if l.Role == "" {
		return nil
	}
	if !auth.CanWrite(l.Role) {
		return fmt.Errorf("write access required")
	}
	return nil
}

func (l *Local) Overview(_ context.Context) (any, error) {
	return l.Plat.Overview(0)
}

func (l *Local) ListDevices(_ context.Context, productID, status, q string) (any, error) {
	return l.Plat.ListDevices(0, productID, status, q)
}

func (l *Local) GetDevice(_ context.Context, id int64) (any, error) {
	d, err := l.Plat.GetDevice(id)
	if err != nil {
		return nil, err
	}
	reported, desired, _ := l.Plat.GetAttributes(id)
	latest, _ := l.Plat.LatestTelemetry(id)
	events, _ := l.Plat.ListEvents(0, id, "", 0, 0, 20)
	product, _ := l.Plat.GetProduct(d.ProductPK)
	return map[string]any{
		"device": d, "product": product, "reported": reported, "desired": desired,
		"latest_telemetry": latest, "recent_events": events,
		"connection": map[string]any{
			"client_id":    d.ProductID + "." + d.DeviceID,
			"username":     d.ProductID + "." + d.DeviceID,
			"broker_host":  l.Plat.Cfg.Platform.BrokerHostPublic,
			"broker_port":  l.Plat.Cfg.MQTT.PublicPort(),
			"topic_style":  d.TopicStyle,
			"token_prefix": d.TokenPrefix,
			"note":         "MQTT username = product.device; password = access token (shown only at create/rotate)",
			"mqtt_session": l.Plat.MQTTClientByUser(d.ProductID + "." + d.DeviceID),
		},
	}, nil
}

func (l *Local) FindDeviceID(_ context.Context, productID, deviceID string) (int64, error) {
	d, err := l.Plat.GetDeviceByIdentity(productID, deviceID)
	if err != nil {
		return 0, fmt.Errorf("device %s.%s not found", productID, deviceID)
	}
	return d.ID, nil
}

func (l *Local) ListProducts(_ context.Context) (any, error) {
	return l.Plat.ListProducts(0)
}

func (l *Local) GetProduct(_ context.Context, id int64) (any, error) {
	return l.Plat.GetProduct(id)
}

func (l *Local) InvokeRPC(_ context.Context, devicePK int64, method string, params map[string]any, timeoutMS int, wait bool) (any, error) {
	if err := l.requireWrite(); err != nil {
		return nil, err
	}
	d, err := l.Plat.GetDevice(devicePK)
	if err != nil {
		return nil, err
	}
	if params == nil {
		params = map[string]any{}
	}
	return l.Plat.InvokeRPC(d, services.RPCRequest{
		Method:    method,
		Params:    params,
		TimeoutMS: timeoutMS,
	}, wait)
}

func (l *Local) LatestTelemetry(_ context.Context, devicePK int64) (any, error) {
	return l.Plat.LatestTelemetry(devicePK)
}

func (l *Local) GetAttributes(_ context.Context, devicePK int64) (any, error) {
	reported, desired, err := l.Plat.GetAttributes(devicePK)
	if err != nil {
		return nil, err
	}
	return map[string]any{"reported": reported, "desired": desired}, nil
}

func (l *Local) SetDesired(_ context.Context, devicePK int64, attrs map[string]any) (any, error) {
	if err := l.requireWrite(); err != nil {
		return nil, err
	}
	d, err := l.Plat.GetDevice(devicePK)
	if err != nil {
		return nil, err
	}
	if err := l.Plat.SetDesired(d, attrs, true); err != nil {
		return nil, err
	}
	return attrs, nil
}
