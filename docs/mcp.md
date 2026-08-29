# MCP (LLM → devices)

Streamable HTTP MCP is **embedded in the platform** at `/mcp` (same process, JWT/API token). There is no separate MCP binary or container.

## Tools

| Tool | Purpose |
|------|---------|
| `overview` | Online/offline counts, MQTT status |
| `list_devices` | Filter by product/status/query |
| `get_device` | Full detail (telemetry, attributes, schema) |
| `list_products` / `get_product` | Product schemas |
| `list_device_methods` | Product schema RPC methods (includes built-in `rpc-list`) |
| `invoke_rpc` | Call device method (`reboot`, custom, …) |
| `reboot_device` | Shortcut for product `reboot` RPC |
| `get_telemetry` | Latest samples |
| `get_attributes` | Reported + desired |
| `set_desired_attributes` | Push desired attrs |

Identify a device by platform `id` **or** `product_id` + `device_id`.

## Endpoint / auth

```
http://<platform-host>:9082/mcp
Authorization: Bearer <API token or JWT>
```

Prefer a long-lived **API token** (Settings → API tokens), not a login JWT.

1. Open dashboard → **Settings**
2. Create token (e.g. name `mcp`) — copy `cip_…` once
3. Cursor config:

```json
{
  "mcpServers": {
    "cogitor-iot": {
      "url": "http://192.168.1.100:9082/mcp",
      "headers": {
        "Authorization": "Bearer cip_…"
      }
    }
  }
}
```

Or via API:

```bash
# login JWT (short-lived) to create an API token
JWT=$(curl -s -X POST http://192.168.1.100:9082/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin","password":"admin"}' | jq -r .token)

curl -s -X POST http://192.168.1.100:9082/api/v1/me/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"mcp"}'
# → { "token": "cip_…", "token_prefix": "cip_…", … }
```

Write tools (`invoke_rpc`, `reboot_device`, `set_desired_attributes`) need operator/admin role on the token’s user.
## Example: ESP32 reboot

Product schema must include the predefined `reboot` method (ESP defaults / product editor). Firmware handles MQTT `down/rpc` → `reboot`.

### 1. List devices

Tool `list_devices`:

```json
{ "query": "esp32" }
```

### 2. Confirm methods

Tool `list_device_methods`:

```json
{ "product_id": "esp32-cam", "device_id": "cam-001" }
```

Expect `reboot` with param `delayMs`.

### 3. Restart

Preferred — tool `reboot_device`:

```json
{
  "product_id": "esp32-cam",
  "device_id": "cam-001",
  "delay_ms": 500
}
```

Equivalent — tool `invoke_rpc`:

```json
{
  "product_id": "esp32-cam",
  "device_id": "cam-001",
  "method": "reboot",
  "params": { "delayMs": 500 }
}
```

LLM prompt example:

> Reboot device `esp32-cam` / `cam-001` with 500ms delay using the Cogitor IoT MCP tools.

Platform publishes MQTT down RPC; device should ACK and restart.

## Deploy

MCP is part of `/platform`. No extra Docker service, port, or env.
