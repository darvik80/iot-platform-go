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

After CONNECT the platform resolves the device by username and verifies the token (SHA-256 stored in the database). ACL: in the full format a device may only publish under its own `iot/v1/{product_id}/{device_id}/up/…` and subscribe under its own `…/down/…`; short topics (`v1/…`) are allowed and bound to the device by the session username.

---

## Addressing

Every device is identified by the **product_id / device_id** pair. The platform supports two topic formats — **full** and **short**. The device picks the format with its first published packet; downlinks reply in the same format. Until the first device packet, downlinks default to short.

### Full format

The topic explicitly contains product_id and device_id. A device may only use topics with its own product_id / device_id. Node-RED and other external clients do not use device topics — they work through the rules bus (see below).

```
iot/v1/{product_id}/{device_id}/{up|down}/{capability}/{operation}
```

Example: `iot/v1/sensors/temp-01/up/telemetry/data`

### Short format

The topic does not contain the device ID — the platform resolves it from the MQTT session username. Convenient for firmware: no need to embed product_id/device_id in every topic.

```
v1/me/{mnemonic}
```

Example: `v1/me/t` — telemetry from the current device.

`v1/me/{mnemonic}` is the canonical form. For uplinks the platform also accepts `v1/{mnemonic}` without `me`; downlinks are always sent to `v1/me/…`, so the device must subscribe to `v1/me/…` topics.

### Topic list

| Full topic | Short | Direction | Purpose |
|---|---|---|---|
| `…/up/status` | `v1/me/s` | ↑ device | Status (LWT only) |
| `…/up/telemetry/data` | `v1/me/t` | ↑ device | Telemetry |
| `…/up/events/post` | `v1/me/e` | ↑ device | Events |
| `…/up/attributes/report` | `v1/me/a` | ↑ device | Attributes (report) |
| `…/up/attributes/request` | `v1/me/a/req` | ↑ device | Attribute request |
| `…/down/attributes/response` | `v1/me/a/res` | ↓ platform | Attribute request response |
| `…/down/attributes/update` | `v1/me/a/upd` | ↓ platform | Desired attribute update |
| `…/down/rpc/request` | `v1/me/r/req` | ↓ platform | RPC call to device |
| `…/up/rpc/response` | `v1/me/r/res` | ↑ device | RPC result |
| `…/up/rpc/request` | `v1/me/r/creq` | ↑ device | RPC call from device |
| `…/down/rpc/response` | `v1/me/r/cres` | ↓ platform | Response to device RPC |
| `…/up/ntp/request` | `v1/me/n/req` | ↑ device | Time request (NTP) |
| `…/down/ntp/response` | `v1/me/n/res` | ↓ platform | Time response |
| `…/up/ota/version` | `v1/me/o/ver` | ↑ device | Current firmware version |
| `…/up/ota/query` | `v1/me/o/q` | ↑ device | Update query |
| `…/down/ota/update` | `v1/me/o/upd` | ↓ platform | Update command |
| `…/down/ota/cancel` | `v1/me/o/can` | ↓ platform | Cancel update |
| `…/up/ota/progress` | `v1/me/o/p` | ↑ device | Update progress |
| `…/up/logs/report` | `v1/me/l` | ↑ device | Device logs |

Full topic: `iot/v1/{product_id}/{device_id}/{up|down}/…`. The "Full topic" column shows the tail after `up/` or `down/`.

---

## What the device does

### Telemetry

Publish to `v1/me/t` (or `iot/v1/{p}/{d}/up/telemetry/data`):

```json
{"temperature": 23.5, "humidity": 60}
```

The platform accepts three payload formats:

**Flat object** — keys become samples directly:
```json
{"temperature": 23.5, "humidity": 60, "voltage": 3.3}
```

**Object with `values`** — keys inside `values`, optional timestamp:
```json
{"ts": 1710000000000, "values": {"temperature": 23.5, "humidity": 60}}
```

**Array of objects** — batch publish of multiple sets with different timestamps:
```json
[
  {"ts": 1710000000000, "values": {"temperature": 23.5}},
  {"ts": 1710000001000, "values": {"temperature": 23.7}}
]
```

Each key is stored as an individual sample with type: `value_num` (float64), `value_text` (string), `value_bool` (bool). The `ts` field is milliseconds; if absent, the platform substitutes the receive time.

### Status (LWT)

On CONNECT the device publishes a retained message to `v1/me/s`:

```json
{"online": true}
```

Last Will and Testament: `{"online": false, "reason": "lwt"}`. The platform considers the device offline on session disconnect or LWT receipt.

### Attributes

- **Report** (`v1/me/a`): device reports current attributes — `{"firmware": "1.2.3", "model": "A"}`
- **Request** (`v1/me/a/req`): device requests desired attributes from the platform
- **Update** (`v1/me/a/upd`, downlink): platform sends desired attributes

**Attribute report** — flat JSON object, each key is merged into the reported-attributes table:
```json
{"firmwareVersion": "1.2.3", "model": "A", "serial": "SN-001"}
```
The `firmwareVersion` key automatically updates the `fw_version` field in the devices table.

**Attribute request** — device requests stored attributes from the platform:
```json
{"id": 1, "reported": ["*"], "desired": ["*"]}
```
`id` is a correlation identifier for the response. `reported` / `desired` are arrays of key names; `["*"]` means "all keys". The platform responds on `v1/me/a/res`:
```json
{"id": 1, "reported": {"firmwareVersion": "1.2.3"}, "desired": {"interval": 30}}
```

### RPC

Platform calls a method on the device:

**Downlink** (`v1/me/r/req`):
```json
{"id": 1, "method": "reboot", "params": {"delay": 5}}
```

**Device response** (`v1/me/r/res`):
```json
{"id": 1, "code": 0, "data": {}}
```

`code: 0` — success. The `id` field correlates request and response.

The device can also call RPC on the platform via `v1/me/r/creq` (response arrives on `v1/me/r/cres`).

**Device-initiated RPC** (`v1/me/r/creq`):
```json
{"id": 1, "method": "issueUploadUrl", "params": {}}
```
Platform responds on `v1/me/r/cres`:
```json
{"id": 1, "code": 0, "message": "ok", "data": {"url": "https://…/upload/{product}/{device}", "expires_in": 3600}}
```

Built-in method **`rpc-list`** (alias `rpc_list`) returns the device method catalog. Each param: `{ type, required, default? }` (`int` / `bool` / `string` / `number`). The dashboard calls it automatically when the device is online.

### OTA (firmware update)

1. Device publishes its version: `v1/me/o/ver` → `{"version": "1.2.3", "module": "main"}`
2. Or queries for an update: `v1/me/o/q` → `{"version": "1.2.3", "module": "main"}`
3. Platform replies with a command: `v1/me/o/upd` → `{"url": "https://…/ota/fw/{token}.bin", "version": "1.3.0", "module": "main", "size": 524288, "sha256": "…", "signMethod": "sha256", "sign": "…", "force": false}`
4. Device downloads firmware and publishes progress: `v1/me/o/p` → `{"step": 45, "module": "main", "desc": "downloading"}`
5. Cancel: `v1/me/o/can`

**Firmware version** (`v1/me/o/ver`):
```json
{"version": "1.2.3", "module": "main"}
```
`module` is optional, defaults to `main`. The platform updates the device `fw_version` and checks for auto-OTA.

**Update query** (`v1/me/o/q`):
```json
{"version": "1.2.3", "module": "main"}
```
The platform looks for the latest firmware for this product/module; if the version differs from current, it starts an OTA job.

**Progress** (`v1/me/o/p`):
```json
{"step": 45, "module": "main", "desc": "downloading"}
```
| `step` value | Job status |
|--------------|------------|
| 0–100 | `running` — update in progress |
| < 0 | `failed` — error |
| ≥ 101 | `done` — completed |

### Events and logs

**Event** (`v1/me/e`) — a discrete incident:
```json
{
  "id": "evt-001",
  "ts": 1710000000000,
  "type": "alarm",
  "severity": "warning",
  "code": "OVERTEMP",
  "message": "Temperature exceeded threshold",
  "data": {"temperature": 85.3, "threshold": 80}
}
```
| Field | Required | Description |
|-------|----------|-------------|
| `id` | no | Event identifier (unique on the device) |
| `ts` | no | Milliseconds; if 0 or absent — receive time |
| `type` | no | Event type (`alarm`, `info`, …); defaults to `info` |
| `severity` | no | `info`, `warning`, `error`, `critical`; defaults to `info` |
| `code` | no | Event code (string) |
| `message` | no | Text description |
| `data` | no | Arbitrary object with additional data |

**Logs** (`v1/me/l`) — single message or array:
```json
[
  {"ts": 1710000000000, "level": "INFO", "module": "wifi", "message": "Connected to AP"},
  {"ts": 1710000001000, "level": "ERROR", "module": "sensor", "code": "E_TIMEOUT", "message": "Read timeout", "context": {"retries": 3}}
]
```
| Field | Required | Description |
|-------|----------|-------------|
| `ts` | no | Milliseconds; if 0 — receive time |
| `level` | no | Log level (`INFO`, `WARN`, `ERROR`, `DEBUG`) |
| `module` | no | Device module (`wifi`, `sensor`, …) |
| `code` | no | Event code (string) |
| `message` | no | Message text |
| `context` | no | Arbitrary object with context |

### NTP

Device requests time: `v1/me/n/req` → platform replies `v1/me/n/res` with the current timestamp.

**Device request:**
```json
{"id": 1, "deviceSendTime": 1710000000000}
```

**Platform response** (`v1/me/n/res`):
```json
{"id": 1, "deviceSendTime": 1710000000000, "serverRecvTime": 1710000000012, "serverSendTime": 1710000000015}
```
Three timestamps allow the device to calculate clock offset: `serverRecvTime` and `serverSendTime` are server-side receive/send times, `deviceSendTime` is echoed back from the request.

---

## Commands from external clients (Node-RED, plugins)

External clients publish to `platform/v1/commands/{account_slug}/{product_id}/{device_id}/{kind}`:

| Kind | Purpose |
|------|---------|
| `rpc` | RPC call to device |
| `attributes/desired` | Set desired attributes |
| `ota` | Start OTA update |

Events are published to `platform/v1/events/{account_slug}/{product_id}/{device_id}/{kind}`. Subscribe within your own account: `platform/v1/events/{account_slug}/#` (`platform/v1/events/#` is denied by ACL).

---

## Device auth

1. Dashboard creates device → generates access token → SHA-256 stored in the database
2. MQTT CONNECT: `username={product}.{device}`, `password=token`
3. Token rotation → new hash in DB; active session is disconnected
4. Disable / delete device → session disconnected; reconnect fails

## Monitoring

Dashboard **MQTT** page (`GET /api/v1/mqtt`): live sessions (client id, username, IP, subscriptions) and an in-memory event trail (auth, connect/disconnect, ACL deny, ingest, publish).

Stdout logs (`slog`, `LOG_LEVEL`, default Info): `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe`, `ingest`, `command`, downlink `publish`. Docker: `docker logs -f <platform-container>`.

WebSocket event `mqtt` is broadcast on connect / disconnect / auth_fail / subscribe / ACL deny / kick.

## External clients (Node-RED, plugins)

External clients work through the rules bus — topics `platform/v1/events/{account_slug}/…` and `platform/v1/commands/{account_slug}/…`. Setup is described in [06-automation.md](06-automation.md).
