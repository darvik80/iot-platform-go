# Automation (Node-RED)

The platform does **not** embed a visual flow editor. Use Node-RED for CEP / multi-device logic.

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

Platform provisions RabbitMQ user `nodered` (password from `NODERED_MQTT_PASSWORD`, default `nodered`) with ACL on `platform.v1.events.#` / `platform.v1.commands.#`.

**Never** put device access tokens into Node-RED flows.

## Example flow

`deploy/nodered/flows.json` ships a temperature-threshold → desired-attribute example and an offline status debug node.

Dashboard → **Automation** opens the Node-RED editor URL from settings.
