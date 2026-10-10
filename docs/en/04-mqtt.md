# MQTT protocol

The platform process **is** the MQTT broker ([mochi-mqtt](https://github.com/mochi-mqtt/server)). There is no external RabbitMQ.

### Connection

| Parameter | Value |
|-----------|-------|
| TCP | `MQTT_ADDR` (default `:1883`) |
| WebSocket | `MQTT_WS_ADDR` (default `:8083`, URL `ws://{BROKER_HOST_PUBLIC}:8083/` or `MQTT_WS_PUBLIC_URL` behind a reverse proxy, subprotocol `mqtt`). `MQTT_WS_ADDR=off` to disable |
| TLS | `MQTT_TLS_ADDR` (default `:8883`) — see [05-mqtt-tls.md](05-mqtt-tls.md) |
| Username | `{product_id}.{device_id}` |
| Password | access token (generated in the dashboard) |
| Client ID | arbitrary; platform identifies the device by username |

After CONNECT the platform resolves the device by username and verifies the token (SHA-256 stored in the database). ACL: in the full format a device may only publish under its own `iot/v1/{product_id}/{device_id}/up/…` and subscribe under its own `…/down/…`; short topics (`v1/…`) are allowed and bound to the device by the session username.

These are the parameters of the native protocol (`short` / `full` styles). The **alink** and **ThingsBoard** dialects use different CONNECT parameters — see [Device styles (dialects)](#device-styles-dialects).

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

## Device styles (dialects)

The platform supports four MQTT protocol dialects. The style (`topic_style`) is set on the device at creation time or inherited from the product's **Topic style default** field; it can be viewed and changed on the device page.

| Dialect (`topic_style`) | Compatibility | Authentication | Topics |
|---|---|---|---|
| `short` (default) | native | `username={product}.{device}`, `password=token` | `v1/me/…` + full |
| `full` | native | `username={product}.{device}`, `password=token` | `iot/v1/…` + short |
| `alink` | Alibaba Cloud IoT (Alink) | HMAC signature of the deviceSecret | `/sys/…`, `/ext/ntp/…`, `/ota/device/…` |
| `thingsboard` | ThingsBoard MQTT Device API | `username=access token`, no password | `v1/devices/me/…` |

Style selection rules:

- **Auto-detection** works only between `short` and `full`: a Will topic, subscription, or first packet with a foreign prefix flips the device's style.
- **`alink` and `thingsboard` are pinned**: they are never auto-detected on or off, because the style defines the authentication model (secret/token). Pick them explicitly when creating a device — or set the product default.
- Downlinks (RPC, desired attributes, OTA) are always sent in the device's dialect format; uplinks are translated into the internal format, so the dashboard, automation, and plugins work identically for every dialect.

The sections below describe the native protocol (`short` / `full`); the dialect specifics are in [alink](#alink-dialect-alibaba-cloud-iot) and [thingsboard](#thingsboard-dialect).

### Alink dialect (Alibaba Cloud IoT)

For firmware built against the Alink protocol (Alicloud IoT embed component). At create/rotate time the platform generates a **deviceSecret** — the HMAC key, shown once.

Connection:

| Parameter | Value |
|-----------|-------|
| Username | `{deviceName}&{productKey}` (the native `{product}.{device}` is also accepted) |
| Password | HMAC-SHA256 of `clientId{username}deviceName{dn}productKey{pk}timestamp{ts}` keyed by the deviceSecret, uppercase hex |
| Client ID | `{deviceName}&{productKey}\|timestamp=…,ext=…\|`; timestamp from the KV section, default `2524608000000` |

Topics:

| Topic | Dir | Purpose |
|---|---|---|
| `/sys/{pk}/{dn}/thing/event/property/post` | ↑ | Telemetry |
| `/sys/{pk}/{dn}/thing/event/{eventId}/post` | ↑ | Event |
| `/sys/{pk}/{dn}/thing/property/desired/get` | ↑ | Desired attribute request |
| `/sys/{pk}/{dn}/thing/property/desired/delete` | ↑ | Desired attribute deletion |
| `/sys/{pk}/{dn}/thing/service/property/set_reply` | ↑ | Reply to property/set |
| `/sys/{pk}/{dn}/thing/service/{method}/_reply` | ↑ | RPC reply |
| `/sys/{pk}/{dn}/thing/ota/firmware/get` | ↑ | OTA query (pull) |
| `/sys/{pk}/{dn}/thing/log/post` | ↑ | Device logs |
| `/ota/device/inform/{pk}/{dn}` | ↑ | Firmware version |
| `/ota/device/progress/{pk}/{dn}` | ↑ | OTA progress |
| `/ext/ntp/{pk}/{dn}/request` | ↑ | Time request (NTP) |
| `/sys/{pk}/{dn}/thing/service/property/set` | ↓ | Desired attribute update |
| `/sys/{pk}/{dn}/thing/property/desired/{get,delete}_reply` | ↓ | Desired get/delete reply |
| `/sys/{pk}/{dn}/thing/service/{method}` | ↓ | Platform RPC call |
| `/ota/device/upgrade/{pk}/{dn}` | ↓ | OTA command (and cancel) |
| `/ext/ntp/{pk}/{dn}/response` | ↓ | Time response |

Payload examples. Telemetry — values with an optional timestamp:
```json
{"id": "1", "version": "1.0", "params": {"temperature": {"value": 23.5, "time": 1710000000000}}}
```

Platform RPC call (id is a string):
```json
{"id": "1", "version": "1.0", "method": "thing.service.reboot", "params": {"delay": 5}}
```
The device replies on `/sys/{pk}/{dn}/thing/service/reboot/_reply`; code `200` means success:
```json
{"id": "1", "code": 200, "message": "success", "data": {}}
```

Desired attribute update — all params values are sent as strings (the firmware reads strings only):
```json
{"id": "2", "version": "1.0", "method": "thing.service.property.set", "params": {"interval": "30"}}
```

OTA command (`code: "1000"` — update, `"-1"` — cancel):
```json
{"code": "1000", "data": {"url": "https://…/ota/fw/{token}.bin", "version": "1.3.0", "module": "main", "size": 524288}}
```

Logs — a batch of up to 40 entries; `utcTime` is epoch-ms or an ISO string `yyyy-MM-ddTHH:mm:ss.SSSZ`, `traceContext` is optional:
```json
{"id": "1", "version": "1.0", "method": "thing.log.post", "params": [{"utcTime": "2024-01-02T03:04:05.678Z", "logLevel": "ERROR", "module": "wifi", "code": "E_TIMEOUT", "traceContext": "trace-42", "logContent": "Read timeout"}]}
```

Not available in the alink style: LWT status, reported-attribute report, device-to-platform RPC. Telemetry, events, logs, desired attributes, NTP, and OTA work as in the native style.

### ThingsBoard dialect

For firmware built against the ThingsBoard MQTT Device API (JSON, v1; the v2 RPC protobuf forms and `v2/fw/…` topics are out of scope). Authentication: **the MQTT username is the device access token**, no password. The token is generated at create/rotate time and shown once.

Topics:

| Topic | Dir | Purpose |
|---|---|---|
| `v1/devices/me/telemetry` | ↑ | Telemetry (same payload formats as the native style) |
| `v1/devices/me/attributes` | ↑ | Attributes report |
| `v1/devices/me/attributes/request/{id}` | ↑ | Attribute request |
| `v1/devices/me/attributes/response/{id}` | ↓ | Attribute request response |
| `v1/devices/me/rpc/request/{id}` | ↓ | Platform RPC call |
| `v1/devices/me/rpc/response/{id}` | ↑ | RPC reply |
| `v2/devices/me/rpc/response/{id}` | ↑ | RPC reply (v2 topic form) |

The device subscribes to `v1/devices/me/attributes` (desired updates arrive as a flat object) and `v1/devices/me/rpc/request/+`.

Payload examples. Platform RPC call — the id lives in the topic, not the body:
```json
{"method": "reboot", "params": {"delay": 5}}
```
The device replies on `v1/devices/me/rpc/response/1` with arbitrary JSON:
```json
{"status": "ok"}
```

Attribute request: `sharedKeys` — desired (server-side), `clientKeys` — reported (client-side); a JSON array or a comma-separated string is accepted:
```json
{"sharedKeys": "interval", "clientKeys": "fw"}
```
The reply arrives on `v1/devices/me/attributes/response/1`:
```json
{"client": {"fw": "1.2.3"}, "shared": {"interval": 30}}
```

Not available in the thingsboard style: LWT status, events, logs, NTP, device-side OTA, device-to-platform RPC. Telemetry and attributes work as in the native style.

> ThingsBoard downlink topics (`v1/devices/me/…`) look the same for every device — the platform delivers each message directly to the target device's session, like the short native style.

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

This is the native model. `alink` devices authenticate with an HMAC signature of the deviceSecret, `thingsboard` devices with the access token as username and no password (see [Device styles (dialects)](#device-styles-dialects)).

## Monitoring

Dashboard **MQTT** page (`GET /api/v1/mqtt`): live sessions (client id, username, IP, subscriptions) and an in-memory event trail (auth, connect/disconnect, ACL deny, ingest, publish).

Stdout logs (`slog`, `LOG_LEVEL`, default Info): `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe`, `ingest`, `command`, downlink `publish`. Docker: `docker logs -f <platform-container>`.

WebSocket event `mqtt` is broadcast on connect / disconnect / auth_fail / subscribe / ACL deny / kick.

## External clients (Node-RED, plugins)

External clients work through the rules bus — topics `platform/v1/events/{account_slug}/…` and `platform/v1/commands/{account_slug}/…`. Setup is described in [06-automation.md](06-automation.md).
