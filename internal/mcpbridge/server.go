package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"

	"cogitor-iot-platform/internal/models"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer builds an MCP server exposing Cogitor IoT device tools.
func NewServer(backend Backend) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "cogitor-iot",
		Version: "1.0.0",
	}, nil)

	type emptyIn struct{}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "overview",
		Description: "Platform overview: online/offline device counts, MQTT broker status and live MQTT client count, recent event count.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, any, error) {
		out, err := backend.Overview(ctx)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	type listDevicesIn struct {
		ProductID string `json:"product_id,omitempty" jsonschema:"filter by product_id"`
		Status    string `json:"status,omitempty" jsonschema:"filter: online|offline|unknown"`
		Query     string `json:"query,omitempty" jsonschema:"search device_id/name/product_id"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_devices",
		Description: "List registered IoT devices (id, product_id, device_id, status, last_seen).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listDevicesIn) (*mcp.CallToolResult, any, error) {
		out, err := backend.ListDevices(ctx, in.ProductID, in.Status, in.Query)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	type getDeviceIn struct {
		ID        int64  `json:"id,omitempty" jsonschema:"platform device primary key"`
		ProductID string `json:"product_id,omitempty" jsonschema:"product_id when resolving by identity"`
		DeviceID  string `json:"device_id,omitempty" jsonschema:"device_id when resolving by identity"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_device",
		Description: "Get full device detail: connection info, reported/desired attributes, latest telemetry, product schema, recent events. Identify by id OR product_id+device_id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getDeviceIn) (*mcp.CallToolResult, any, error) {
		id, err := resolveDeviceID(ctx, backend, in.ID, in.ProductID, in.DeviceID)
		if err != nil {
			return errResult(err), nil, nil
		}
		out, err := backend.GetDevice(ctx, id)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_products",
		Description: "List products with registered RPC methods, metrics and attributes schemas.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, any, error) {
		out, err := backend.ListProducts(ctx)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	type getProductIn struct {
		ID int64 `json:"id" jsonschema:"product primary key"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_product",
		Description: "Get one product including RPC method definitions (name, params) for invoke_rpc.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getProductIn) (*mcp.CallToolResult, any, error) {
		if in.ID == 0 {
			return errResult(fmt.Errorf("id required")), nil, nil
		}
		out, err := backend.GetProduct(ctx, in.ID)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	type listMethodsIn struct {
		ID        int64  `json:"id,omitempty" jsonschema:"device primary key"`
		ProductID string `json:"product_id,omitempty"`
		DeviceID  string `json:"device_id,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_device_methods",
		Description: "List RPC methods from the product schema (always includes built-in rpc-list). For the live firmware catalog, invoke_rpc method=rpc-list (alias rpc_list). Use before invoke_rpc.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listMethodsIn) (*mcp.CallToolResult, any, error) {
		id, err := resolveDeviceID(ctx, backend, in.ID, in.ProductID, in.DeviceID)
		if err != nil {
			return errResult(err), nil, nil
		}
		detail, err := backend.GetDevice(ctx, id)
		if err != nil {
			return errResult(err), nil, nil
		}
		m, _ := detail.(map[string]any)
		if m == nil {
			return textOK(detail)
		}
		product, _ := m["product"].(map[string]any)
		if product == nil {
			// product may be a struct encoded as map via json round-trip in HTTP client;
			// for Local it is *models.Product — expose methods via typed field when possible.
			if pr, ok := m["product"].(*models.Product); ok && pr != nil {
				return textOK(map[string]any{"device_id": m["device"], "methods": pr.Methods})
			}
			return errResult(fmt.Errorf("device has no product schema")), nil, nil
		}
		return textOK(map[string]any{
			"device_id": m["device"],
			"methods":   product["methods"],
		})
	})

	type invokeIn struct {
		ID        int64          `json:"id,omitempty" jsonschema:"device primary key"`
		ProductID string         `json:"product_id,omitempty"`
		DeviceID  string         `json:"device_id,omitempty"`
		Method    string         `json:"method" jsonschema:"RPC method name, e.g. reboot"`
		Params    map[string]any `json:"params,omitempty" jsonschema:"method parameters object"`
		TimeoutMS int            `json:"timeout_ms,omitempty" jsonschema:"wait timeout in ms (default platform RPC timeout)"`
		NoWait    bool           `json:"no_wait,omitempty" jsonschema:"if true, enqueue only and return without waiting for device response"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "invoke_rpc",
		Description: "Invoke an RPC method on a device (MQTT down/rpc). Prefer list_device_methods first. Example: method=reboot params={delayMs:500}.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in invokeIn) (*mcp.CallToolResult, any, error) {
		if in.Method == "" {
			return errResult(fmt.Errorf("method required")), nil, nil
		}
		id, err := resolveDeviceID(ctx, backend, in.ID, in.ProductID, in.DeviceID)
		if err != nil {
			return errResult(err), nil, nil
		}
		out, err := backend.InvokeRPC(ctx, id, in.Method, in.Params, in.TimeoutMS, !in.NoWait)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	type rebootIn struct {
		ID        int64  `json:"id,omitempty"`
		ProductID string `json:"product_id,omitempty"`
		DeviceID  string `json:"device_id,omitempty"`
		DelayMS   int    `json:"delay_ms,omitempty" jsonschema:"delay before reboot, default 500"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "reboot_device",
		Description: "Convenience: invoke predefined reboot RPC on a device.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in rebootIn) (*mcp.CallToolResult, any, error) {
		id, err := resolveDeviceID(ctx, backend, in.ID, in.ProductID, in.DeviceID)
		if err != nil {
			return errResult(err), nil, nil
		}
		delay := in.DelayMS
		if delay <= 0 {
			delay = 500
		}
		out, err := backend.InvokeRPC(ctx, id, "reboot", map[string]any{"delayMs": delay}, 0, true)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	type telemIn struct {
		ID        int64  `json:"id,omitempty"`
		ProductID string `json:"product_id,omitempty"`
		DeviceID  string `json:"device_id,omitempty"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_telemetry",
		Description: "Latest telemetry samples for a device.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in telemIn) (*mcp.CallToolResult, any, error) {
		id, err := resolveDeviceID(ctx, backend, in.ID, in.ProductID, in.DeviceID)
		if err != nil {
			return errResult(err), nil, nil
		}
		out, err := backend.LatestTelemetry(ctx, id)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_attributes",
		Description: "Reported and desired attributes for a device.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in telemIn) (*mcp.CallToolResult, any, error) {
		id, err := resolveDeviceID(ctx, backend, in.ID, in.ProductID, in.DeviceID)
		if err != nil {
			return errResult(err), nil, nil
		}
		out, err := backend.GetAttributes(ctx, id)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	type setDesiredIn struct {
		ID         int64          `json:"id,omitempty"`
		ProductID  string         `json:"product_id,omitempty"`
		DeviceID   string         `json:"device_id,omitempty"`
		Attributes map[string]any `json:"attributes" jsonschema:"desired attribute key/value map to push to the device"`
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "set_desired_attributes",
		Description: "Set desired attributes and push update to the device.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setDesiredIn) (*mcp.CallToolResult, any, error) {
		if len(in.Attributes) == 0 {
			return errResult(fmt.Errorf("attributes required")), nil, nil
		}
		id, err := resolveDeviceID(ctx, backend, in.ID, in.ProductID, in.DeviceID)
		if err != nil {
			return errResult(err), nil, nil
		}
		out, err := backend.SetDesired(ctx, id, in.Attributes)
		if err != nil {
			return errResult(err), nil, nil
		}
		return textOK(out)
	})

	return s
}

func resolveDeviceID(ctx context.Context, b Backend, id int64, productID, deviceID string) (int64, error) {
	if id > 0 {
		return id, nil
	}
	if productID != "" && deviceID != "" {
		return b.FindDeviceID(ctx, productID, deviceID)
	}
	return 0, fmt.Errorf("provide id or product_id+device_id")
}

func textOK(v any) (*mcp.CallToolResult, any, error) {
	s, err := textResult(v)
	if err != nil {
		return errResult(err), nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: s}},
	}, v, nil
}

func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

func textResult(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
