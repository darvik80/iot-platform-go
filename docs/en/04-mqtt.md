# MQTT protocol

The platform process **is** the MQTT broker ([mochi-mqtt](https://github.com/mochi-mqtt/server)). There is no external RabbitMQ.

### Connection

| Parameter | Value |
|-----------|-------|
| TCP | `MQTT_ADDR` (default `:1883`) |
| WebSocket | `MQTT_WS_ADDR` (default `:8083`, URL `ws://{BROKER_HOST_PUBLIC}:8083/`, subprotocol `mqtt`). `MQTT_WS_ADDR=off` to disable |
| TLS | `MQTT_TLS_ADDR` (default `:8883`) — see [05-mqtt-tls.md](05-mqtt-tls.md) |
| Username | `{product_id}.{device_id}` |
| Password | access token (generated in the dashboard) |
| Client ID | arbitrary; platform identifies the device by username |

After CONNECT the platform resolves the device by username and verifies the token (SHA-256 stored in SQLite). ACL is scoped to that device's topics.

---

## Addressing

Every device is identified by the **product_id / device_id** pair. The platform supports two topic formats — **full** and **short**. The device picks the format with its first published packet; downlinks reply in the same format. Until the first device packet, downlinks default to short.

### Full format

The topic explicitly contains product_id and device_id. Used when a single client (e.g. Node-RED or a gateway) talks to multiple devices.

```
iot/v1/{product_id}/{device_id}/{up|down}/{capability}/{operation}
```

Example: `iot/v1/sensors/temp-01/up/telemetry/data`

### Short format

The topic does not contain the device ID — the platform resolves it from the MQTT session username. Convenient for firmware: no need to embed product_id/device_id in every topic.

```
v1/{mnemonic}
```

Example: `v1/t` — telemetry from the current device.

The alias `v1/me/{mnemonic}` is equivalent to `v1/{mnemonic}`.

### Topic list

| Full topic | Short | Direction | Purpose |
|---|---|---|---|
| `…/up/status` | `v1/s` | ↑ device | Status (LWT only) |
| `…/up/telemetry/data` | `v1/t` | ↑ device | Telemetry |
| `…/up/events/post` | `v1/e` | ↑ device | Events |
| `…/up/attributes/report` | `v1/a` | ↑ device | Attributes (report) |
| `…/up/attributes/request` | `v1/a/req` | ↑ device | Attribute request |
| `…/down/attributes/response` | `v1/a/res` | ↓ platform | Attribute request response |
| `…/down/attributes/update` | `v1/a/upd` | ↓ platform | Desired attribute update |
| `…/down/rpc/request` | `v1/r/req` | ↓ platform | RPC call to device |
| `…/up/rpc/response` | `v1/r/res` | ↑ device | RPC result |
| `…/up/rpc/request` | `v1/r/creq` | ↑ device | RPC call from device |
| `…/down/rpc/response` | `v1/r/cres` | ↓ platform | Response to device RPC |
| `…/up/ntp/request` | `v1/n/req` | ↑ device | Time request (NTP) |
| `…/down/ntp/response` | `v1/n/res` | ↓ platform | Time response |
| `…/up/ota/version` | `v1/o/ver` | ↑ device | Current firmware version |
| `…/up/ota/query` | `v1/o/q` | ↑ device | Update query |
| `…/down/ota/update` | `v1/o/upd` | ↓ platform | Update command |
| `…/down/ota/cancel` | `v1/o/can` | ↓ platform | Cancel update |
| `…/up/ota/progress` | `v1/o/p` | ↑ device | Update progress |
| `…/up/logs/report` | `v1/l` | ↑ device | Device logs |

Full topic: `iot/v1/{product_id}/{device_id}/{up|down}/…`. The "Full topic" column shows the tail after `up/` or `down/`.

---

## What the device does

### Telemetry

Publish to `v1/t` (or `iot/v1/{p}/{d}/up/telemetry/data`):

```json
{"temperature": 23.5, "humidity": 60}
```

### Status (LWT)

On CONNECT the device publishes a retained message to `v1/s`:

```json
{"online": true}
```

Last Will and Testament: `{"online": false, "reason": "lwt"}`. The platform considers the device offline on session disconnect or LWT receipt.

### Attributes

- **Report** (`v1/a`): device reports current attributes — `{"firmware": "1.2.3", "model": "A"}`
- **Request** (`v1/a/req`): device requests desired attributes from the platform
- **Update** (`v1/a/upd`, downlink): platform sends desired attributes

### RPC

Platform calls a method on the device:

**Downlink** (`v1/r/req`):
```json
{"id": 1, "method": "reboot", "params": {"delay": 5}}
```

**Device response** (`v1/r/res`):
```json
{"id": 1, "code": 0, "data": {}}
```

`code: 0` — success. The `id` field correlates request and response.

The device can also call RPC on the platform via `v1/r/creq` (response arrives on `v1/r/cres`).

Built-in method **`rpc-list`** (alias `rpc_list`) returns the device method catalog. Each param: `{ type, required, default? }` (`int` / `bool` / `string` / `number`). The dashboard calls it automatically when the device is online.

### OTA (firmware update)

1. Device publishes its version: `v1/o/ver` → `{"version": "1.2.3"}`
2. Or queries for an update: `v1/o/q`
3. Platform replies with a command: `v1/o/upd` → `{"url": "https://…/ota/fw/{token}.bin"}`
4. Device downloads firmware and publishes progress: `v1/o/p` → `{"percent": 45}`
5. Cancel: `v1/o/can`

### Events and logs

- **Event** (`v1/e`): arbitrary event from the device
- **Logs** (`v1/l`): device log messages

### NTP

Device requests time: `v1/n/req` → platform replies `v1/n/res` with the current timestamp.

---

## Commands from external clients (Node-RED, plugins)

External clients publish to `platform/v1/commands/{product_id}/{device_id}/{kind}`:

| Kind | Purpose |
|------|---------|
| `rpc` | RPC call to device |
| `attributes/desired` | Set desired attributes |
| `ota` | Start OTA update |

Subscribe to events: `platform/v1/events/#`

---

## Device auth

1. Dashboard creates device → generates access token → SHA-256 stored in SQLite
2. MQTT CONNECT: `username={product}.{device}`, `password=token`
3. Token rotation → new hash in DB; active session is disconnected
4. Disable / delete device → session disconnected; reconnect fails

## Monitoring

Dashboard **MQTT** page (`GET /api/v1/mqtt`): live sessions (client id, username, IP, subscriptions) and an in-memory event trail (auth, connect/disconnect, ACL deny, ingest, publish).

Stdout logs (`slog`, `LOG_LEVEL`, default Info): `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe`, `ingest` (except telemetry), `command`, downlink `publish`. Telemetry is Debug. Docker: `docker compose logs -f platform`.

WebSocket event `mqtt` is broadcast on connect / disconnect / auth_fail / subscribe / ACL deny / kick.

## External clients (Node-RED, plugins)

External clients work through the rules bus — topics `platform/v1/events/#` and `platform/v1/commands/#`. Setup is described in [06-automation.md](06-automation.md).
