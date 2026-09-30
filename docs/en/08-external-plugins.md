# External Plugins

> **Implementation status (v1.0.x)**
>
> **Implemented:** the **MQTT transport only** (rules bus). An admin registers a plugin
> (`POST /api/v1/plugins/external` or Settings → External Plugins) and gets MQTT credentials
> (`mqtt_user` = `extplg_{account_slug}_{plugin_id}`, one-time `mqtt_pass`; no API token).
> The plugin subscribes to `{rules_bus}/events/{account_slug}/#` and publishes to
> `{rules_bus}/commands/…`. Also implemented: list / delete / `regenerate-mqtt`, and
> `online` / `offline` status driven by the MQTT connection.
>
> **Planned, not implemented:** webhook, polling and gRPC transports; heartbeat; the JSON-RPC
> endpoint `/api/plugins/external/rpc`; `plugin.*` notifications and `platform.*` methods;
> the `endpoint` registration field; plugin API tokens (hashing, rate limit); the
> `endpoint` / `auth_token_hash` / `mode` / `last_heartbeat_at` DB columns; binding external
> plugins to devices and delivering `tick` / `apply` to them. Sections describing these are
> kept as the roadmap and marked **(planned)**.
>
> Commands must use `{rules_bus}/commands/{account_slug}/{product_id}/{device_id}/…` with the plugin's
> own slug; the device is resolved inside that account. See
> [06-automation.md → Multi-account topics](06-automation.md#multi-account-topics).

## 1. Overview

External plugins extend IoT Platform logic **without recompiling the Go binary**.
They run as separate processes (any language) and communicate with the platform
via **MQTT** (rules bus subscription). **HTTP** (webhook/polling) and **gRPC** transports are planned.

The planned full contract mirrors the internal `plugins.Instance`; with the
MQTT transport available today the plugin sees rules bus events of its account
and sends commands, but has no bindings, `tick`, `apply` or config delivery.

**When to use external plugins:**
- Logic that should run independently of the platform process
- Code in Python, Java, C++, or any language with HTTP/MQTT/gRPC
- Cross-device rules that don't fit Node-RED
- Integration with third-party services

**When to use internal plugins instead:**
- Simple per-device automation (LED effects, schedules)
- Minimum latency (in-process, no network hop)
- See [07-internal-plugins.md](07-internal-plugins.md)

| Transport | Direction | When to use | Status |
|-----------|-----------|-------------|--------|
| **Webhook** (HTTP push) | Platform → Plugin | Low latency, plugin has public IP | planned |
| **Polling** (HTTP pull) | Plugin → Platform | Plugin behind NAT/firewall | planned |
| **MQTT** (rules bus) | Plugin subscribes to broker | Cross-device events, Node-RED style | **implemented** |
| **gRPC** | Platform → Plugin | High throughput, strict typing | planned |

Planned mapping (only the registration row exists today):

| Internal (in-process)                 | External (out-of-process)                   |
|---------------------------------------|---------------------------------------------|
| `Register(Descriptor, Factory)`       | `POST /api/v1/plugins/external`             |
| `ApplyConfig(json)`                   | Platform → Plugin: `plugin.apply_config`    |
| `OnEvent(Runtime, Event)`             | Platform → Plugin: `plugin.on_event`        |
| `Runtime.InvokeRPC(method, params)`   | Plugin → Platform: `platform.invoke_rpc`    |
| `IntervalMS()` / `Auto()`             | Declared in `Descriptor` at registration    |

---

## 2. Architecture (planned)

> The diagrams in this section show the planned webhook/polling/heartbeat design. In v1.0.x the only path is: device → ingest → rules bus MQTT topic `{rules_bus}/events/{account_slug}/…` → plugin's MQTT subscription; plugin → `{rules_bus}/commands/…` → broker → device downlink.

> **Diagram**: [External Plugin Architecture](14-diagrams.md#external-plugin-architecture)

```mermaid
graph TB
    subgraph Platform["IoT Platform (Go)"]
        subgraph Ingest[" "]
            Bus["Device Bus<br/>(MQTT ingest)"]
            Hub["PluginHub<br/>(ticker, dispatch)"]
        end

        subgraph Support[" "]
            Reg["External Plugin<br/>Registry"]
            Queue["Event Queue<br/>(for polling)"]
            HB["Heartbeat<br/>Monitor"]
        end

        HTTP["HTTP / gRPC client"]
    end

    subgraph Plugin["External Plugin (any language)"]
        subgraph Receivers[" "]
            EP["HTTP endpoint<br/>(webhook)"]
            Poll["Poller<br/>(polling)"]
            HBClient["Heartbeat Client"]
        end

        Logic["Plugin Business Logic"]
    end

    %% Ingest pipeline
    Bus -->|"MQTT events / tick"| Hub

    %% Platform → Plugin (webhook)
    Hub -->|"webhook: POST"| HTTP
    HTTP -->|"plugin.on_event"| EP

    %% Polling
    Hub -->|"polling: queue"| Queue
    Poll -->|"poll_events"| HTTP

    %% Plugin internal
    EP --> Logic
    Poll --> Logic

    %% Plugin → Platform (RPC)
    Logic -->|"invoke_rpc"| HTTP

    %% Registry
    Reg -.->|"endpoint URL"| HTTP

    %% Heartbeat
    HBClient -->|"heartbeat"| HTTP
    HTTP --> HB
    HB -.->|"online/offline"| Reg

    style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
    style Ingest fill:#0f3460,stroke:#16213e,color:#eee
    style Support fill:#16213e,stroke:#1a1a2e,color:#eee
    style Plugin fill:#0f3460,stroke:#16213e,color:#eee
    style Receivers fill:#1a1a2e,stroke:#0f3460,color:#eee
```

### Event Flow (planned)

> **Diagram**: [External Plugin Event Flow](14-diagrams.md#external-plugin-event-flow)

```mermaid
sequenceDiagram
    participant D as Device
    participant P as Platform
    participant Pl as Plugin

    D->>P: MQTT telemetry
    P->>P: PluginHub.Dispatch(kind, payload)
    P->>Pl: POST plugin.on_event (JSON-RPC notification)
    Pl->>Pl: process event
    Pl->>P: POST platform.invoke_rpc (JSON-RPC request)
    P->>D: MQTT downlink (RPC command)
```

### Tick Flow (planned)

Plugins with `auto: true` and `interval_ms > 0` receive periodic `tick`
events — just like internal plugins. The platform owns the timer;
the plugin only reacts.

> **Diagram**: [External Plugin Tick Flow](14-diagrams.md#external-plugin-tick-flow)

```mermaid
sequenceDiagram
    participant Hub as PluginHub
    participant Pl as Plugin

    loop every interval_ms
        Hub->>Pl: POST plugin.on_event {kind: "tick"}
        Pl->>Pl: process
        Pl-->>Hub: HTTP 200
    end
```

---

## 3. Internal Plugin Model (reference)

Internal plugins implement the `plugins.Instance` interface:

```go
type Instance interface {
    ApplyConfig(raw json.RawMessage) error   // validate + load config
    OnEvent(rt Runtime, ev Event) error      // handle a single event
    IntervalMS() int                         // tick interval (0 = no tick)
    Auto() bool                              // subscribe to tick cycle?
}

type Runtime interface {
    InvokeRPC(method string, params map[string]any) error
}

type Event struct {
    Kind     string // tick | apply | status | telemetry | event | rpc/result
    Online   bool
    Payload  any
    DevicePK int64
}
```

Event types (constants in `plugins` package):

| Constant           | Value        | When delivered                                |
|--------------------|-------------|-----------------------------------------------|
| `EventTick`        | `tick`      | Every `interval_ms` while device is online    |
| `EventApply`       | `apply`     | Config saved or binding created               |
| `EventStatus`      | `status`    | Device online/offline transition              |
| `EventTelemetry`   | `telemetry` | Device published telemetry                    |
| `EventEvent`       | `event`     | Device published a custom event               |
| `EventRPCResult`   | `rpc/result`| Response to a previously sent RPC             |

The planned webhook/polling/gRPC transports would deliver the **same** event types over the wire. MQTT plugins (v1.0.x) instead receive rules bus kinds: `status`, `telemetry`, `event`, `attributes/reported`, `attributes/desired`, `rpc/result`, `ota`, `log` — no `tick` or `apply`.

---

## 4. External Plugin Contract

### 4.1 Registration

Admin creates the plugin via dashboard (**Settings → External Plugins**) or
via API. The platform saves the descriptor to the `external_plugins` table
under the admin's **active account** (from the JWT) and returns MQTT credentials.
No API token is issued.

```
POST /api/v1/plugins/external
Content-Type: application/json
Authorization: Bearer <admin JWT>

{
  "id":           "temp-alert",
  "name":         "Temperature Alert",
  "description":  "Send RPC when temperature exceeds threshold",
  "auto":         true,
  "interval_ms":  5000,
  "default_config": {
    "threshold_c": 35.0,
    "rpc_method":  "set_fan",
    "rpc_params":  {"on": true}
  },
  "form": [
    {"key": "threshold_c", "type": "number", "label": "Threshold °C",
     "min": -40, "max": 120},
    {"key": "rpc_method",  "type": "string", "label": "RPC method"},
    {"key": "rpc_params",  "type": "json",   "label": "RPC params JSON"}
  ]
}
```

Response (`201 Created`):

```json
{
  "ok": true,
  "id": "temp-alert",
  "mqtt_user": "extplg_acme_temp-alert",
  "mqtt_pass": "ext_plg_3f9c…",
  "mqtt_host": "192.168.1.22",
  "mqtt_port": 1883,
  "mqtt_ws": "ws://192.168.1.22:8083/",
  "rules_bus": "platform/v1"
}
```

Response fields: `ok`, `id`, `mqtt_user` (`extplg_{account_slug}_{plugin_id}`), `mqtt_pass`,
`mqtt_port`, `mqtt_host` (`BROKER_HOST_PUBLIC`), `mqtt_ws`, `rules_bus` (rules bus prefix).

**Save `mqtt_pass` immediately** — it is shown only once (only its SHA-256 hash is stored).
A new password can be issued with `POST /api/v1/plugins/external/{id}/regenerate-mqtt`
(response: `ok`, `id`, `mqtt_user`, `mqtt_pass`).

`id` is the table's primary key and is unique across all accounts: registering an existing
`id` again overwrites that plugin and issues a new password.

Fields (request):

| Field            | Type       | Req | Description                                  |
|------------------|-----------|------|----------------------------------------------|
| `id`             | string     | yes  | Unique plugin identifier (slug)             |
| `name`           | string     | yes  | Human-readable name                         |
| `description`    | string     | no   | Shown in UI                                 |
| `auto`           | bool       | no   | Stored; tick delivery is planned (default `false`) |
| `interval_ms`    | int        | no   | Stored; tick delivery is planned (default 0) |
| `default_config` | object     | no   | Stored default config (JSON)                |
| `effects`        | string[]   | no   | Stored effects catalog (for LED-like plugins) |
| `form`           | FormField[]| no   | Stored config form schema for UI            |

`account_id` / `account_slug` in the body are ignored — they are taken from the JWT.
The `endpoint` field (webhook URL / gRPC address) is **planned** and currently ignored.

The `form` schema is identical to the internal `FormField` type
(see `internal/plugins/form.go`).

### 4.2 Connectivity

Four transport modes are designed; **only Mode 3 (MQTT) is implemented** in v1.0.x.

#### Mode 1: Webhook (HTTP push) (planned)

Plugin specifies `endpoint` (starting with `http://` or `https://`) at registration.
Platform POSTs JSON-RPC notifications to that URL.

**Requirements:**
- URL must be reachable from the platform server
- Plugin must respond HTTP 200 within 5 seconds
- HTTPS recommended for production

**Limitations:**
- Plugin behind NAT/firewall — platform cannot reach it
- Dynamic IP/port — need to re-register

#### Mode 2: Polling (HTTP pull) (planned)

Plugin does not specify `endpoint`. Instead, it periodically calls
`platform.poll_events` to get accumulated events.

#### Mode 3: MQTT (rules bus subscription)

Every registered plugin receives **MQTT credentials** at registration.
The plugin connects to the platform's embedded MQTT broker and subscribes
to the rules bus — the same topics Node-RED uses.

**Connection:**
```
Host:     mqtt_host (from registration response)
Port:     mqtt_port (default 1883) or mqtt_ws (WebSocket)
Username: mqtt_user (format: extplg_{account_slug}_{plugin_id})
Password: mqtt_pass (one-time, shown only at registration / regenerate-mqtt)
```

**ACL (what the plugin can do over MQTT):**

| Action | Topic pattern | Description |
|--------|---------------|-------------|
| Subscribe (read) | `{rules_bus}/events/{account_slug}/#` (or narrower) | Events of the plugin's own account: telemetry, status, event, log, ota, attributes, rpc/result |
| Publish (write) | `{rules_bus}/commands/{account_slug}/{product}/{device}/{capability}` | Send RPC, set desired attributes, trigger OTA on the account's devices |

`{rules_bus}/events/#`, `#` and `+` are denied, as are other accounts' event and command subtrees.
The account slug is the part of `mqtt_user` between `extplg_` and the next `_`.

**Event topic structure** (subscribe):
```
platform/v1/events/{account_slug}/{product_id}/{device_id}/telemetry
platform/v1/events/{account_slug}/{product_id}/{device_id}/status
platform/v1/events/{account_slug}/{product_id}/{device_id}/event
platform/v1/events/{account_slug}/{product_id}/{device_id}/log
platform/v1/events/{account_slug}/{product_id}/{device_id}/ota
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/reported
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/events/{account_slug}/{product_id}/{device_id}/rpc/result
```

**Command topic structure** (publish):
```
platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc
platform/v1/commands/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{account_slug}/{product_id}/{device_id}/ota
```

The slug must match the plugin's own account (the ACL rejects others), and `{product_id}/{device_id}`
is resolved inside that account. The legacy slug-less form is no longer accepted.

**Example: subscribe to all events of the account (Python):**
```python
import paho.mqtt.client as mqtt

MQTT_USER = "extplg_acme_temp-alert"
ACCOUNT_SLUG = MQTT_USER.removeprefix("extplg_").split("_", 1)[0]  # "acme"

def on_message(client, userdata, msg):
    print(f"[{msg.topic}] {msg.payload.decode()}")

client = mqtt.Client()
client.username_pw_set(MQTT_USER, "one-time-password")
client.connect("192.168.1.22", 1883)
client.subscribe(f"platform/v1/events/{ACCOUNT_SLUG}/#")
client.on_message = on_message
client.loop_forever()
```

**Example: send RPC command (publish):**
```python
import json
client.publish(
    f"platform/v1/commands/{ACCOUNT_SLUG}/esp32-cam/cam-001/rpc",
    json.dumps({"method": "reboot", "params": {"delayMs": 1000}})
)
```

**MQTT mode advantages:**
- No need for HTTP endpoint or polling — just subscribe
- Works behind NAT (outbound MQTT connection)
- Sees all device events **of its account**, not just bound ones
- Can send commands to devices of its account via commands topics
- No heartbeat needed: MQTT connect / disconnect sets the plugin status `online` / `offline`

**MQTT mode limitations:**
- No per-binding event filtering (receives all events)
- No tick events (use internal timer instead)
- No `plugin.apply_config` / `plugin.detach` notifications

#### Mode 4: gRPC (planned)

Plugin specifies `endpoint` **without** `http://` prefix (e.g. `localhost:50051`).
Platform connects as gRPC client. See [Section 5](#5-grpc-transport-advanced-planned).

#### Polling details (planned)

Plugin does not specify `endpoint`. Instead, it periodically calls
`platform.poll_events` to get accumulated events.

```json
{
  "jsonrpc": "2.0",
  "id": "poll-1",
  "method": "platform.poll_events",
  "params": {
    "plugin_id": "temp-alert",
    "limit": 100
  }
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": "poll-1",
  "result": {
    "events": [
      {
        "id": 12345,
        "binding_id": 42,
        "method": "plugin.on_event",
        "params": { "kind": "telemetry", "payload": {...} }
      }
    ]
  }
}
```

After processing, plugin calls `platform.ack_events` to confirm:

```json
{
  "jsonrpc": "2.0",
  "id": "ack-1",
  "method": "platform.ack_events",
  "params": { "event_ids": [12345] }
}
```

#### Heartbeat (liveness check) (planned)

Plugin periodically pings the platform to report liveness.
Works in both modes (webhook and polling).

```json
{
  "jsonrpc": "2.0",
  "id": "hb-1",
  "method": "platform.heartbeat",
  "params": {
    "plugin_id": "temp-alert",
    "status": "ok",
    "stats": {
      "events_processed": 1234,
      "errors": 2,
      "uptime_sec": 3600
    }
  }
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": "hb-1",
  "result": {
    "ok": true,
    "pending_events": 5
  }
}
```

**Heartbeat parameters:**
- Interval: every 30 seconds (configurable)
- Timeout: no heartbeat for 90 seconds → plugin marked `offline`
- UI shows plugin status: `online` / `offline` / `unknown`

> **Diagram**: [External Plugin Heartbeat](14-diagrams.md#external-plugin-heartbeat)

```mermaid
sequenceDiagram
    participant Pl as Plugin
    participant P as Platform

    loop every 30 sec
        Pl->>P: platform.heartbeat {status: "ok"}
        P-->>Pl: {ok: true, pending_events: 5}
    end

    Note over Pl,P: No heartbeat for 90 sec → plugin offline
    P->>P: mark plugin offline
    P->>P: pause event delivery
```

#### Mode Comparison

Only the MQTT column is implemented; the other columns describe planned modes.

| Characteristic     | Webhook       | Polling       | MQTT              | gRPC          |
|--------------------|---------------|---------------|-------------------|---------------|
| Delivery latency   | Instant       | Poll interval | Instant           | Instant       |
| Plugin behind NAT  | No            | Yes           | Yes (outbound)    | No            |
| Event scope        | Bound device  | Bound device  | All devices of the account | Bound device  |
| Can send commands  | Via RPC API   | Via RPC API   | Via MQTT publish  | Via RPC API   |
| Heartbeat          | Required      | Required      | Not needed        | Ping (gRPC)   |
| Implementation     | HTTP server   | HTTP client   | MQTT client       | gRPC server   |
| Typing             | JSON          | JSON          | JSON              | Protobuf      |
| Recommendation     | LAN, cloud    | NAT, firewall | Cross-device, IoT | High perf     |

### 4.3 Notifications Platform → Plugin (webhook mode only) (planned)

Platform sends **JSON-RPC 2.0 notifications** (no `id` field, no response
expected) to `{endpoint}`. Plugin must respond HTTP `200 OK` within
5 seconds, otherwise the event is considered lost.

#### 4.3.1 `plugin.apply_config`

Sent on binding creation, config update, or "Apply now" press.

```json
{
  "jsonrpc": "2.0",
  "method": "plugin.apply_config",
  "params": {
    "binding_id": 42,
    "device_pk": 7,
    "config": { "threshold_c": 35.0, "rpc_method": "set_fan" }
  }
}
```

Plugin must validate config and return HTTP 200. If validation fails —
return HTTP 400 with JSON-RPC error — platform shows error in UI.

#### 4.3.2 `plugin.on_event`

Delivers each event the internal `OnEvent` would receive.

```json
{
  "jsonrpc": "2.0",
  "method": "plugin.on_event",
  "params": {
    "binding_id": 42,
    "device_pk": 7,
    "kind": "telemetry",
    "online": true,
    "payload": { "temperature": 38.2, "humidity": 61 }
  }
}
```

`kind` values: `tick` | `apply` | `status` | `telemetry` | `event` | `rpc/result`.

#### 4.3.3 `plugin.detach`

Sent on binding removal or plugin disconnect. Plugin must
release resources associated with the binding.

```json
{
  "jsonrpc": "2.0",
  "method": "plugin.detach",
  "params": { "binding_id": 42 }
}
```

### 4.4 Calls Plugin → Platform (planned)

No JSON-RPC endpoint and no plugin token exist in v1.0.x. Today a plugin sends
commands over MQTT (`{rules_bus}/commands/…`, see Mode 3) or via REST
`POST /api/v1/devices/{id}/rpc` with a user Bearer token.

Plugin calls platform at `PLATFORM_URL` (env var or from registration response).
All calls are standard JSON-RPC 2.0 with `id`, expect a response.

Auth: `Authorization: Bearer <plugin_token>` header. Token is issued
at registration, limited to plugin operations only.

#### 4.4.1 `platform.invoke_rpc` (planned)

Send RPC command to the device bound to this plugin.

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "method": "platform.invoke_rpc",
  "params": {
    "binding_id": 42,
    "method": "set_fan",
    "params": { "on": true },
    "timeout_ms": 4000
  }
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": "req-1",
  "result": { "sent": true }
}
```

#### 4.4.2 `platform.set_attributes` — NOT IMPLEMENTED

Set desired attributes / configuration to the device.

```json
{
  "jsonrpc": "2.0",
  "id": "req-2",
  "method": "platform.set_attributes",
  "params": {
    "binding_id": 42,
    "attributes": { "mode": "eco", "target_temp": 22 }
  }
}
```

#### 4.4.3 `platform.get_device` — NOT IMPLEMENTED

Get current device state (status, latest telemetry, attributes).

```json
{
  "jsonrpc": "2.0",
  "id": "req-3",
  "method": "platform.get_device",
  "params": { "binding_id": 42 }
}
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": "req-3",
  "result": {
    "device_pk": 7,
    "product_id": "smoke",
    "device_id":  "kitchen-01",
    "status":     "online",
    "enabled":    true,
    "telemetry":  { "temperature": 38.2, "humidity": 61 },
    "attributes": { "mode": "eco" }
  }
}
```

---

## 5. gRPC Transport (advanced) (planned)

For plugins requiring low latency or high throughput:

```protobuf
syntax = "proto3";
package iotplugin;

service ExternalPlugin {
  // Platform → Plugin notifications (unary)
  rpc ApplyConfig(ApplyConfigRequest) returns (ApplyConfigResponse);
  rpc OnEvent(OnEventRequest) returns (OnEventResponse);
  rpc Detach(DetachRequest) returns (DetachResponse);
}

service PlatformAPI {
  // Plugin → Platform callbacks
  rpc InvokeRPC(InvokeRPCRequest) returns (InvokeRPCResponse);
  rpc SetAttributes(SetAttributesRequest) returns (SetAttributesResponse);
  rpc GetDevice(GetDeviceRequest) returns (GetDeviceResponse);
}

message Event {
  int64  binding_id = 1;
  int64  device_pk  = 2;
  string kind       = 3; // tick | apply | status | telemetry | event | rpc/result
  bool   online     = 4;
  bytes  payload    = 5; // JSON
}

message ApplyConfigRequest {
  int64  binding_id = 1;
  int64  device_pk  = 2;
  bytes  config     = 3; // JSON
}
message ApplyConfigResponse { bool ok = 1; string error = 2; }

message OnEventRequest  { Event event = 1; }
message OnEventResponse { bool ok = 1; string error = 2; }

message DetachRequest     { int64 binding_id = 1; }
message DetachResponse    { bool ok = 1; }

message InvokeRPCRequest {
  int64  binding_id = 1;
  string method     = 2;
  bytes  params     = 3; // JSON
  int32  timeout_ms = 4;
}
message InvokeRPCResponse { bool sent = 1; }

message SetAttributesRequest {
  int64 binding_id = 1;
  bytes attributes = 2; // JSON
}
message SetAttributesResponse { bool ok = 1; }

message GetDeviceRequest  { int64 binding_id = 1; }
message GetDeviceResponse {
  int64  device_pk   = 1;
  string product_id  = 2;
  string device_id   = 3;
  string status      = 4;
  bool   enabled     = 5;
  bytes  telemetry   = 6; // JSON
  bytes  attributes  = 7; // JSON
}
```

---

## 6. Security

### HTTP API (planned)
- Plugin token issued at registration, stored hashed in DB.
- Token is scoped: can only call `platform.invoke_rpc` for bindings
  referencing this plugin ID.
- HTTPS recommended for production; HTTP allowed for local development.
- Rate limit: 100 calls/sec per plugin token.

None of the above exists in v1.0.x: no plugin token is issued, and there is no plugin HTTP API or rate limit.

### MQTT ACL
External plugins authenticate with the `extplg_{account_slug}_{plugin_id}` username + password
(only the SHA-256 hash of the password is stored). The broker enforces this ACL:

| Direction | Allowed topics | Denied |
|-----------|---------------|--------|
| Subscribe (read) | `{prefix}/events/{account_slug}/…` | `{prefix}/events/#`, `#`, `+`, other accounts' events, device topics (`iot/v1/…`, `v1/…`), `$…` |
| Publish (write) | `{prefix}/commands/{account_slug}/…` | Other accounts' commands, slug-less commands, device topics, `{prefix}/events/…`, `$…` |

Plugins **cannot** impersonate devices, read other accounts' events or command other accounts'
devices: the target device is resolved inside the plugin's own account.

---

## 7. Example Implementation — Python "Temperature Alert" Plugin (planned)

> This example targets the planned webhook + JSON-RPC transport (`endpoint`, `plugin.*`,
> `platform.*`, `/api/plugins/external/rpc`, `/api/plugins/external/register`, plugin token)
> and **does not work against v1.0.x**. For working MQTT examples see
> [examples/plugins/](../../examples/plugins/).

Full external plugin in Python (~80 lines) for the planned webhook transport.
Receives telemetry events, sends RPC when temperature exceeds threshold.

```python
#!/usr/bin/env python3
"""temp_alert_plugin.py — IoT Platform external plugin example.

Receives telemetry events from the platform.
When temperature exceeds threshold — calls set_fan RPC on the device.

Run:
    pip install flask requests
    python temp_alert_plugin.py
"""

import os
import threading
import requests
from flask import Flask, request, jsonify

# ── Configuration ──────────────────────────────────────────────
PLATFORM_URL = os.environ.get("PLATFORM_URL", "http://localhost:8080")
PLUGIN_TOKEN = os.environ.get("PLUGIN_TOKEN", "")
LISTEN_PORT  = int(os.environ.get("PLUGIN_PORT", "8090"))

app = Flask(__name__)

# Per-binding state: binding_id → {config, last_fired}
bindings: dict[int, dict] = {}
lock = threading.Lock()


# ── Plugin → Platform calls ────────────────────────────────────
def platform_call(method: str, params: dict) -> dict:
    """Call platform JSON-RPC API."""
    resp = requests.post(
        f"{PLATFORM_URL}/api/plugins/external/rpc",
        headers={"Authorization": f"Bearer {PLUGIN_TOKEN}"},
        json={"jsonrpc": "2.0", "id": 1, "method": method, "params": params},
        timeout=5,
    )
    resp.raise_for_status()
    return resp.json().get("result", {})


def invoke_rpc(binding_id: int, method: str, params: dict):
    """Send RPC command to device."""
    return platform_call("platform.invoke_rpc", {
        "binding_id": binding_id,
        "method": method,
        "params": params,
        "timeout_ms": 4000,
    })


# ── Platform → Plugin handlers ─────────────────────────────────
@app.route("/rpc", methods=["POST"])
def handle_rpc():
    """Single endpoint for all platform notifications."""
    data = request.get_json(force=True)
    method = data.get("method")
    params = data.get("params", {})

    if method == "plugin.apply_config":
        return handle_apply_config(params)
    elif method == "plugin.on_event":
        return handle_on_event(params)
    elif method == "plugin.detach":
        return handle_detach(params)
    else:
        return jsonify({"error": f"unknown method: {method}"}), 400


def handle_apply_config(params: dict):
    """Validate and save binding config."""
    binding_id = params["binding_id"]
    config = params.get("config", {})

    # Validate
    threshold = config.get("threshold_c", 35.0)
    if not isinstance(threshold, (int, float)):
        return jsonify({"error": "threshold_c must be a number"}), 400

    with lock:
        bindings[binding_id] = {
            "config": config,
            "last_fired": {},
        }
    return jsonify({"ok": True})


def handle_on_event(params: dict):
    """Handle event from platform."""
    binding_id = params["binding_id"]
    kind = params["kind"]
    payload = params.get("payload", {})

    with lock:
        state = bindings.get(binding_id)
    if not state:
        return jsonify({"ok": True})  # not yet configured

    config = state["config"]

    if kind == "telemetry":
        temp = payload.get("temperature")
        if temp is None:
            return jsonify({"ok": True})

        threshold = config.get("threshold_c", 35.0)
        if temp > threshold:
            method = config.get("rpc_method", "set_fan")
            rpc_params = config.get("rpc_params", {"on": True})
            try:
                invoke_rpc(binding_id, method, rpc_params)
                print(f"[temp-alert] {temp}°C > {threshold}°C → {method} fired")
            except Exception as e:
                print(f"[temp-alert] RPC failed: {e}")

    elif kind == "status":
        online = params.get("online", False)
        print(f"[temp-alert] device {'online' if online else 'offline'}")

    elif kind == "tick":
        # Periodic tick — for housekeeping, retries, etc.
        pass

    return jsonify({"ok": True})


def handle_detach(params: dict):
    """Binding removed — cleanup state."""
    binding_id = params["binding_id"]
    with lock:
        bindings.pop(binding_id, None)
    print(f"[temp-alert] binding {binding_id} detached")
    return jsonify({"ok": True})


# ── Heartbeat ──────────────────────────────────────────────────
start_time = None
events_processed = 0

def heartbeat_loop():
    """Periodic heartbeat to platform."""
    global events_processed
    while True:
        try:
            requests.post(
                f"{PLATFORM_URL}/api/plugins/external/rpc",
                headers={"Authorization": f"Bearer {PLUGIN_TOKEN}"},
                json={
                    "jsonrpc": "2.0", "id": 1,
                    "method": "platform.heartbeat",
                    "params": {
                        "plugin_id": "temp-alert",
                        "status": "ok",
                        "stats": {
                            "events_processed": events_processed,
                            "uptime_sec": int((time.time() - start_time) if start_time else 0),
                        }
                    }
                },
                timeout=5,
            )
        except Exception as e:
            print(f"[temp-alert] heartbeat failed: {e}")
        time.sleep(30)


# ── Registration ───────────────────────────────────────────────
def register():
    """Register plugin on platform at startup."""
    descriptor = {
        "id": "temp-alert",
        "name": "Temperature Alert",
        "description": "Fire RPC when temperature exceeds threshold",
        "endpoint": f"http://localhost:{LISTEN_PORT}/rpc",
        "auto": True,
        "interval_ms": 5000,
        "default_config": {
            "threshold_c": 35.0,
            "rpc_method": "set_fan",
            "rpc_params": {"on": True},
        },
        "form": [
            {
                "key": "threshold_c",
                "type": "number",
                "label": "Threshold °C",
                "min": -40,
                "max": 120,
            },
            {"key": "rpc_method", "type": "string", "label": "RPC method"},
            {"key": "rpc_params", "type": "json", "label": "RPC params JSON", "wide": True},
        ],
    }
    resp = requests.post(
        f"{PLATFORM_URL}/api/plugins/external/register",
        json=descriptor,
        timeout=5,
    )
    if resp.status_code == 200:
        token = resp.json().get("token", "")
        print(f"[temp-alert] registered, token={token[:8]}…")
        return token
    else:
        raise RuntimeError(f"registration failed: {resp.status_code} {resp.text}")


if __name__ == "__main__":
    import time
    start_time = time.time()
    token = register()
    PLUGIN_TOKEN = token
    # Start heartbeat in background
    threading.Thread(target=heartbeat_loop, daemon=True).start()
    app.run(host="0.0.0.0", port=LISTEN_PORT)
```

### End-to-End Flow (planned)

> **Diagram**: [External Plugin End-to-End Flow](14-diagrams.md#external-plugin-end-to-end-flow)

```mermaid
sequenceDiagram
    participant Pl as Plugin (Python)
    participant P as Platform
    participant U as User (UI)
    participant D as Device

    Note over U: Admin creates plugin in UI / API
    U->>P: POST /api/v1/plugins/external → MQTT creds (plugin token planned)
    U-->>Pl: pass credentials to plugin
    U->>P: Bind plugin to device
    P->>Pl: plugin.apply_config {threshold_c: 30}
    D->>P: MQTT telemetry {temperature: 32.5}
    P->>Pl: plugin.on_event {kind: "telemetry", payload: {...}}
    Pl->>Pl: 32.5 > 30 → need to call RPC
    Pl->>P: platform.invoke_rpc("set_fan", {on: true})
    P->>D: MQTT downlink → RPC command
```

---

## 8. Database Schema

Actual table (`internal/db/migrations/001_init.sql`):

```sql
CREATE TABLE IF NOT EXISTS external_plugins (
    id                TEXT PRIMARY KEY,
    account_id        INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    mqtt_user         TEXT NOT NULL DEFAULT '',   -- extplg_{account_slug}_{plugin_id}
    mqtt_pass_hash    TEXT NOT NULL DEFAULT '',   -- SHA-256 of mqtt_pass
    auto              INTEGER NOT NULL DEFAULT 0,
    interval_ms       INTEGER NOT NULL DEFAULT 0,
    default_config    TEXT NOT NULL DEFAULT '{}',
    effects           TEXT NOT NULL DEFAULT '[]',
    form              TEXT NOT NULL DEFAULT '[]',
    status            TEXT NOT NULL DEFAULT 'unknown',  -- online | offline | unknown
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_external_plugins_account ON external_plugins(account_id);
```

The planned transports would add `endpoint`, `auth_token_hash`, `mode` (webhook | polling)
and `last_heartbeat_at`; these columns do not exist yet.

External plugins are **not** bound to devices in v1.0.x: `device_plugin_bindings` is used only
by internal plugins. Delivering events to bound external plugins via `PluginHub` is planned.

---

## 9. Lifecycle

Implemented (MQTT): admin registers the plugin → status `unknown` → plugin connects to the broker
with `mqtt_user` / `mqtt_pass` → status `online` → plugin disconnects → status `offline`.
**Regenerate MQTT** replaces the password; **Delete** removes the plugin and its credentials.

The diagram below shows the planned full lifecycle (heartbeat, bindings, `plugin.*` notifications) **(planned)**.

> **Diagram**: [External Plugin Lifecycle](14-diagrams.md#external-plugin-lifecycle)

```mermaid
sequenceDiagram
    participant Pl as Plugin
    participant P as Platform
    participant U as User
    participant D as Device

    Note over U: Admin creates plugin in UI
    U->>P: POST /api/v1/plugins/external
    P-->>U: {ok: true, mqtt_user: "...", mqtt_pass: "...", ...}
    U-->>Pl: pass MQTT creds to plugin

    loop every 30 sec
        Pl->>P: platform.heartbeat
        P-->>Pl: {ok: true}
    end

    Note over U: Bind plugin to device
    U->>P: Create binding
    P->>Pl: plugin.apply_config
    Pl-->>P: HTTP 200

    Note over D: Device connects
    D->>P: MQTT status {online: true}
    P->>Pl: plugin.on_event {kind: "status", online: true}

    Note over D: Telemetry
    D->>P: MQTT telemetry
    P->>Pl: plugin.on_event {kind: "telemetry"}
    Pl->>P: platform.invoke_rpc (optional)

    Note over P: Tick (if auto=true)
    P->>Pl: plugin.on_event {kind: "tick"}

    Note over U: User presses "Apply now"
    U->>P: Update binding
    P->>Pl: plugin.apply_config
    P->>Pl: plugin.on_event {kind: "apply"}

    Note over Pl: Plugin stopped
    Pl->>P: (no heartbeat for 90 sec)
    P->>P: mark plugin offline

    Note over U: Remove binding
    U->>P: Delete binding
    P->>Pl: plugin.detach (if online)
    Pl->>Pl: Release resources
```

---

## 10. Requirements Checklist

- [x] Plugin can be written in any language (MQTT + JSON)
- [x] Registration, delete, regenerate MQTT password (`/api/v1/plugins/external`)
- [x] Plugin receives its account's rules bus events over MQTT (status, telemetry, event, attributes, rpc/result, ota, log)
- [x] Plugin can send RPC / desired attributes / OTA via `{rules_bus}/commands/{account_slug}/…` (own account only)
- [ ] Interaction via JSON-RPC 2.0 (`/api/plugins/external/rpc`) — **planned**
- [ ] Webhook / polling / gRPC transports, heartbeat — **planned**
- [ ] Plugin can call RPC on bound device (`platform.invoke_rpc`) — **planned** (`InvokeRPCForPlugin` exists in code but is not wired to any endpoint)
- [ ] Plugin can set device attributes (`platform.set_attributes`) — **planned**
- [ ] `tick` / `apply` events and device bindings for external plugins — **planned**
- [ ] Behavior identical to internal plugins — same event model, same lifecycle — **planned**

---

## 11. Future Improvements

- **Bidirectional streaming** (gRPC): replace polling with persistent stream.
- **Plugin marketplace**: signed descriptors, versioned configs.
- **Sandbox**: run untrusted plugins in WASM or container.
- **Metrics**: event counter, error rate, latency per plugin.
- **Hot reload**: re-register without unbinding devices.
- **`platform.set_attributes`**: allow plugins to set device desired attributes.
- **`platform.get_device`**: allow plugins to query device state.
- **Rate limiting**: 100 calls/sec per plugin token.

---

## Related documentation

| Doc | What |
|-----|------|
| [07-internal-plugins.md](07-internal-plugins.md) | In-process device plugins |
| [06-automation.md](06-automation.md) | Node-RED & rules bus |
| [04-mqtt.md](04-mqtt.md) | MQTT protocol & topics |
| [examples/plugins/](../../examples/plugins/) | MQTT code examples (Python, Go, Java, C++) |
