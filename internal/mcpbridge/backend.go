package mcpbridge

import "context"

// Backend is the platform surface used by MCP tools.
// *Client talks over HTTP; *Local calls services in-process.
type Backend interface {
	Overview(ctx context.Context) (any, error)
	ListDevices(ctx context.Context, productID, status, q string) (any, error)
	GetDevice(ctx context.Context, id int64) (any, error)
	FindDeviceID(ctx context.Context, productID, deviceID string) (int64, error)
	ListProducts(ctx context.Context) (any, error)
	GetProduct(ctx context.Context, id int64) (any, error)
	InvokeRPC(ctx context.Context, devicePK int64, method string, params map[string]any, timeoutMS int, wait bool) (any, error)
	LatestTelemetry(ctx context.Context, devicePK int64) (any, error)
	GetAttributes(ctx context.Context, devicePK int64) (any, error)
	SetDesired(ctx context.Context, devicePK int64, attrs map[string]any) (any, error)
}
