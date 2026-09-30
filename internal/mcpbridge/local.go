package mcpbridge

import (
	"context"
	"errors"
	"fmt"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/services"
)

var errNotFound = errors.New("not found")

// Local calls Platform services in-process (embedded HTTP MCP on the platform).
// Every call is scoped to AccountID; callers must resolve an active membership first.
type Local struct {
	Plat        *services.Platform
	Role        models.Role
	AccountID   int64
	AccountSlug string
}

func NewLocal(plat *services.Platform, role models.Role, accountID int64, accountSlug string) *Local {
	return &Local{Plat: plat, Role: role, AccountID: accountID, AccountSlug: accountSlug}
}

func (l *Local) requireWrite() error {
	if !auth.CanWrite(l.Role) {
		return fmt.Errorf("write access required")
	}
	return nil
}

func (l *Local) device(id int64) (*models.Device, error) {
	d, err := l.Plat.GetDevice(id)
	if err != nil || d.AccountID != l.AccountID {
		return nil, errNotFound
	}
	return d, nil
}

func (l *Local) Overview(_ context.Context) (any, error) {
	return l.Plat.Overview(l.AccountID)
}

func (l *Local) ListDevices(_ context.Context, productID, status, q string) (any, error) {
	return l.Plat.ListDevices(l.AccountID, productID, status, q)
}

func (l *Local) GetDevice(_ context.Context, id int64) (any, error) {
	d, err := l.device(id)
	if err != nil {
		return nil, err
	}
	reported, desired, _ := l.Plat.GetAttributes(id)
	latest, _ := l.Plat.LatestTelemetry(id)
	events, _ := l.Plat.ListEvents(l.AccountID, id, "", 0, 0, 20)
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
	d, err := l.Plat.GetDeviceByAccountIdentity(l.AccountSlug, productID, deviceID)
	if err != nil || d.AccountID != l.AccountID {
		return 0, fmt.Errorf("device %s.%s not found", productID, deviceID)
	}
	return d.ID, nil
}

func (l *Local) ListProducts(_ context.Context) (any, error) {
	return l.Plat.ListProducts(l.AccountID)
}

func (l *Local) GetProduct(_ context.Context, id int64) (any, error) {
	p, err := l.Plat.GetProduct(id)
	if err != nil || p.AccountID != l.AccountID {
		return nil, errNotFound
	}
	return p, nil
}

func (l *Local) InvokeRPC(_ context.Context, devicePK int64, method string, params map[string]any, timeoutMS int, wait bool) (any, error) {
	if err := l.requireWrite(); err != nil {
		return nil, err
	}
	d, err := l.device(devicePK)
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
	if _, err := l.device(devicePK); err != nil {
		return nil, err
	}
	return l.Plat.LatestTelemetry(devicePK)
}

func (l *Local) GetAttributes(_ context.Context, devicePK int64) (any, error) {
	if _, err := l.device(devicePK); err != nil {
		return nil, err
	}
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
	d, err := l.device(devicePK)
	if err != nil {
		return nil, err
	}
	if err := l.Plat.SetDesired(d, attrs, true); err != nil {
		return nil, err
	}
	return attrs, nil
}
