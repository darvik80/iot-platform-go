# Automation (Node-RED)

Node-RED stays a separate process (compose / Synology). The dashboard **Automation** page embeds the editor in an iframe (`NODERED_URL`, e.g. `http://192.168.1.100:1880`). Same-origin reverse proxy is optional; iframe is enough for the lab if Node-RED allows `frame-ancestors` (see `deploy/nodered/settings.js`).

## Rules bus

### Events (platform → Node-RED)

```
platform/v1/events/{product_id}/{device_id}/status
platform/v1/events/{product_id}/{device_id}/telemetry
platform/v1/events/{product_id}/{device_id}/attributes/reported
platform/v1/events/{product_id}/{device_id}/attributes/desired
platform/v1/events/{product_id}/{device_id}/event
platform/v1/events/{product_id}/{device_id}/rpc/result
platform/v1/events/{product_id}/{device_id}/ota
platform/v1/events/{product_id}/{device_id}/log
```

Example telemetry envelope:

```json
{
  "ts": 1451649600512,
  "product_id": "esp32-cam",
  "device_id": "cam-001",
  "keys": { "temperature": 22.5, "humidity": 61 }
}
```

### Commands (Node-RED → platform)

```
platform/v1/commands/{product_id}/{device_id}/rpc
platform/v1/commands/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{product_id}/{device_id}/ota
```

RPC command (platform allocates device-facing `id`):

```json
{
  "method": "reboot",
  "params": { "delayMs": 1000 },
  "timeout_ms": 30000,
  "correlation_id": "nr-flow-1"
}
```

Alternatively call REST `POST /api/v1/devices/{id}/rpc` with a Bearer token.

## MQTT user

Connect Node-RED to the **platform** MQTT port (`MQTT_ADDR`, default `:1883`), not a separate broker.

User `nodered` (password from `NODERED_MQTT_PASSWORD`, default `nodered`) may subscribe to `platform/v1/events/#` and publish to `platform/v1/commands/#`.

**Never** put device access tokens into Node-RED flows.

## Example flow

`deploy/nodered/flows.json` ships a temperature-threshold → desired-attribute example and an offline status debug node. In compose the MQTT broker host is `platform`.

Dashboard → **Automation** shows the editor inline (and “Open in new tab”). `NODERED_URL` must be reachable **from the browser**, not the Docker hostname `nodered`.
