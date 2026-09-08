# MCP — control devices via LLM

MCP (Model Context Protocol) lets an LLM directly interact with platform devices: read telemetry, invoke RPC, change attributes. The protocol is **Streamable HTTP**, embedded in the platform at the `/mcp` endpoint. No separate process, container, or port.

```
http://<platform-host>:8080/mcp
```

## How it works

An LLM client (Cursor, Claude Desktop, custom agent) connects to `/mcp` and calls tools. The platform translates calls into MQTT commands to devices and returns results.

```
LLM → MCP tool call → platform → MQTT downlink → device
          ← JSON result ← platform ← MQTT uplink ←
```

## Tools

### Read

| Tool | Returns |
|------|---------|
| `overview` | Online/offline counts, MQTT broker status, connected client count |
| `list_devices` | Device list with filtering by product, status (`online`/`offline`), text query |
| `get_device` | Full device card: telemetry, attributes, product schema, recent events |
| `list_products` | All products with RPC methods and attribute schemas |
| `get_product` | Single product with RPC method definitions (name, params) |
| `list_device_methods` | RPC methods of the device's product (always includes built-in `rpc-list`) |
| `get_telemetry` | Latest telemetry samples for the device |
| `get_attributes` | Reported and desired attributes |

### Write

| Tool | Does |
|------|------|
| `invoke_rpc` | Call an RPC method on the device (via MQTT downlink). Specify `method` and `params` |
| `reboot_device` | Quick device reboot (shortcut for `invoke_rpc` with `method=reboot`) |
| `set_desired_attributes` | Push desired attributes to the device |

All write tools require **operator** or **admin** role on the token's user.

### Device identification

Every tool accepts the device identifier in two ways:
- `id` — platform numeric ID
- `product_id` + `device_id` — identity string pair

Either one is sufficient.

## Connecting Cursor

1. Open dashboard → **Settings** → create an API token (e.g. name `mcp`)
2. Copy `cip_…` — shown once
3. Cursor config (Settings → MCP):

```json
{
  "mcpServers": {
    "cogitor-iot": {
      "url": "http://192.168.1.100:8080/mcp",
      "headers": {
        "Authorization": "Bearer cip_…"
      }
    }
  }
}
```

After that, `cogitor-iot.*` tools are available in Cursor chat.

## Getting an API token via API

To automate token creation:

```bash
# 1. Login JWT (short-lived)
JWT=$(curl -s -X POST http://192.168.1.100:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin","password":"admin"}' | jq -r .token)

# 2. Create a long-lived API token
curl -s -X POST http://192.168.1.100:8080/api/v1/me/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"mcp"}'
# → { "token": "cip_…", "token_prefix": "cip_…", … }
```

## Example: ESP32 reboot

Scenario: reboot device `esp32-cam` / `cam-001`.

### 1. Find the device

Call `list_devices`:
```json
{ "query": "esp32" }
```

### 2. Check available methods

Call `list_device_methods`:
```json
{ "product_id": "esp32-cam", "device_id": "cam-001" }
```

Response includes the product schema method list. Look for `reboot` with param `delayMs`.

### 3. Reboot

Via `reboot_device` (shortcut):
```json
{
  "product_id": "esp32-cam",
  "device_id": "cam-001",
  "delay_ms": 500
}
```

Or via `invoke_rpc` (universal):
```json
{
  "product_id": "esp32-cam",
  "device_id": "cam-001",
  "method": "reboot",
  "params": { "delayMs": 500 }
}
```

Platform publishes MQTT downlink RPC. Device ACKs and restarts.

Example LLM prompt:

> Reboot device esp32-cam / cam-001 with 500ms delay using Cogitor IoT MCP tools.

## Deploy

MCP is part of the `/platform` binary. No extra Docker service, port, or env variable. The `/mcp` endpoint is available on the same port as the HTTP API (`HTTP_ADDR`, default `:8080`).
