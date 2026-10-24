# REST API

All endpoints on `/api/v1` (except the few root-level ones listed in [Service endpoints](#service-endpoints)). Auth: `Authorization: Bearer <JWT>` or `Bearer <API token>`.

WebSocket for real-time updates: `GET /api/v1/ws?token=<jwt>` — JSON messages `{ "event", "data" }` for the token's active account only. Browser `Origin` must be the platform host, the host of `PUBLIC_BASE_URL` or `localhost` (non-browser clients without `Origin` are accepted).

All `{id}` paths are resolved inside the active account: IDs of other accounts' devices, products, firmware, plugin bindings and external plugins return `404`.

Required role is noted per endpoint: **public** (no auth), **member** (any member of the active account), **write** (`admin` or `operator`), **admin**. Unless stated otherwise, reads are available to any member and mutations require write.

---

## Authentication

### `GET /auth/config`

Public. Returns flags: `register_enabled`, `require_email_verify`, `google_enabled`.

### `POST /auth/register`

```json
{ "login": "jdoe", "email": "jdoe@example.com", "password": "secret123", "display_name": "Jane" }
```

Login: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`. Optional `account_slug` (otherwise generated from the login). Registration creates a personal account in which the new user is `admin`. `REGISTER_DEFAULT_ROLE` (default `admin`) is only stored in the user's global `role` field.

### `POST /auth/login`

```json
{ "email": "admin@example.com", "password": "<password>" }
```

The `email` field accepts **login or email**. Returns `{ "token": "<jwt>" }`.

Rate limits (in-memory, per process): `login`, `register`, `resend-verification` and `check-slug` allow a burst of 20 requests per client IP, refilling one every 3 s; after 5 failed logins for the same login/email further attempts get `429` (with `Retry-After`) until a token refills, one per minute.

Details: [03-auth.md](03-auth.md).

### Other auth endpoints

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/auth/check-slug?slug=` | public | Account slug availability: `{ "available", "slug" }` |
| GET | `/auth/verify-email?token=` | public | Email verification (HTML page) |
| POST | `/auth/resend-verification` | public | `{ "email" }` or `{ "login" }` — resend verification email |
| GET | `/auth/google`, `/auth/google/callback` | public | Google OAuth |
| POST | `/auth/switch-account` | authenticated | `{ "account_id" }` → `{ "token" }` — new JWT with another active account |

### Current user

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/me` | authenticated | Current user (profile, role) |
| GET | `/me/accounts` | authenticated | Accounts the user is a member of |
| GET | `/me/tokens` | authenticated | List API tokens |
| POST | `/me/tokens` | authenticated | Create API token `{ "name" }`; token shown once |
| DELETE | `/me/tokens/{id}` | authenticated | Revoke API token |
| GET | `/me/avatar` | authenticated | Get avatar |
| POST | `/me/avatar` | authenticated | Upload avatar (multipart field `file`, up to ~3 MB) |
| DELETE | `/me/avatar` | authenticated | Remove avatar |

"Authenticated" endpoints do not require an active account.

---

## Account members, invitations, users

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/invitations/{token}` | public | Invitation info |
| POST | `/invitations/{token}/accept` | authenticated | Accept invitation |
| GET | `/invitations` | admin | List invitations of the active account |
| POST | `/invitations` | admin | Create: `{ "email", "role", "delivery_method" }` |
| POST | `/invitations/{id}/resend` | admin | Resend invitation |
| DELETE | `/invitations/{id}` | admin | Revoke invitation |
| GET | `/members` | admin | List account members |
| POST | `/members` | admin | Add member |
| PATCH | `/members/{userId}` | admin | Change role: `{ "role" }` |
| DELETE | `/members/{userId}` | admin | Remove member |
| GET | `/users` | platform admin | List all users |
| PATCH | `/users/{id}` | admin | Update profile (`display_name`, `email`, `password` ≥ 8 chars). Account admins may edit themselves and users who belong only to their account; `role` / `disabled` — platform admin only |
| DELETE | `/users/{id}` | platform admin | Delete user |
| GET | `/audit-log?limit=` | admin | Audit log |

---

## Overview & health

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Liveness — process is alive (no auth) |
| GET | `/readyz` | Readiness — DB + MQTT ready (no auth); includes `mqtt_clients` |
| GET | `/overview` | Online/offline counts, MQTT status, client count |
| GET | `/mqtt` | Live MQTT sessions + recent broker events |
| GET | `/settings` | Retention, broker host, Node-RED URL, timezone |

---

## Products

A product is a device archetype. Defines RPC methods, telemetry metrics, and attributes for all devices of this type.

| Method | Path |
|--------|------|
| GET | `/products` |
| POST | `/products` |
| GET | `/products/{id}` |
| PUT | `/products/{id}` |
| DELETE | `/products/{id}` |
| POST | `/products/{id}/esp-defaults` |

`POST /products/{id}/esp-defaults` (write) re-applies the ESP preset methods and metrics to the product, keeping product-specific ones.

Request body:

```json
{
  "product_id": "esp32-cam",
  "name": "ESP32-CAM",
  "description": ""
}
```

Product schema details (methods, metrics, attributes): [09-product-schema.md](09-product-schema.md).

---

## Devices

### CRUD

| Method | Path | Description |
|--------|------|-------------|
| GET | `/devices?product_id=&status=&q=` | List with filtering |
| POST | `/devices` | Create under a product of the active account. The pair `product_id.device_id` is the MQTT username, so it must be unique across the whole platform. Returns `credentials` **once** |
| GET | `/devices/{id}` | Detail + attributes + latest telemetry |
| PUT | `/devices/{id}` | Update metadata |
| DELETE | `/devices/{id}` | Delete |

### State management

| Method | Path |
|--------|------|
| POST | `/devices/{id}/token/rotate` | Rotate token; returns new credentials **once**. Old MQTT session is disconnected |
| POST | `/devices/{id}/enable` | Enable device |
| POST | `/devices/{id}/disable` | Disable; MQTT session is dropped |

### Create response

```json
{
  "device": { "id": 1, "product_id": "esp32-cam", "device_id": "cam-001", "status": "offline" },
  "credentials": {
    "product_id": "esp32-cam",
    "device_id": "cam-001",
    "access_token": "...",
    "username": "esp32-cam.cam-001",
    "client_id": "esp32-cam.cam-001",
    "broker_host": "localhost",
    "broker_port": 1883,
    "topic_style": "auto",
    "provision": {
      "wifi": { "ssid": "", "password": "" },
      "cogitor": {
        "product": "esp32-cam",
        "device": "cam-001",
        "host": "192.168.1.100",
        "port": 1883,
        "token": "...",
        "tls": false,
        "topic_short": true
      }
    }
  }
}
```

Dashboard shows a modal with these credentials on create/rotate. Wi-Fi is not stored on the platform — only passed to firmware.

### Device plugins

| Method | Path | Description |
|--------|------|-------------|
| GET | `/plugins` | Builtin plugin catalog |
| GET | `/devices/{id}/plugins` | Bindings for the device |
| POST | `/devices/{id}/plugins` | Create binding: `{ "plugin_id", "config"?, "enabled"? }` |
| PUT | `/devices/{id}/plugins/{bindingId}` | Update config / enabled |
| DELETE | `/devices/{id}/plugins/{bindingId}` | Delete binding |
| POST | `/devices/{id}/plugins/{bindingId}/apply` | Run one plugin frame now (device must be online) |

Details: [07-internal-plugins.md](07-internal-plugins.md).

---

## Telemetry & attributes

| Method | Path |
|--------|------|
| GET | `/devices/{id}/telemetry?keys=a,b&from=&to=&limit=` | Historical telemetry |
| GET | `/devices/{id}/telemetry/latest` | Latest samples |
| GET | `/devices/{id}/attributes` | Reported + desired attributes |
| PATCH | `/devices/{id}/attributes/desired` | Set desired attributes |

PATCH body is a flat JSON object. Platform persists and publishes `attributes/update` to the device via MQTT.

---

## RPC

### Invoke method

`POST /devices/{id}/rpc`

```json
{ "method": "reboot", "params": { "delayMs": 1000 }, "timeout_ms": 30000 }
```

By default the request waits for the device response (up to `timeout_ms`, otherwise the platform default). `?wait=0` — fire-and-forget. The RPC `id` sent to the device is a sequential integer.

### History

`GET /devices/{id}/rpc` — previous RPC calls.

---

## Events & logs

| Method | Path |
|--------|------|
| GET | `/events?device_id=&severity=&from=&to=&limit=` | Device events |
| GET | `/logs?device_id=&level=&from=&to=&limit=` | Device log messages |

---

## OTA firmware updates

### Firmware management

| Method | Path | Description |
|--------|------|-------------|
| GET | `/ota/firmwares?product_id=` | List firmwares |
| POST | `/ota/firmwares/upload` | Multipart: `file` + `product_id` + `version` + optional `module` / `sha256` / `sign_method` / `sign` / `size` |
| POST | `/ota/firmwares` | Register external URL: `{ product_id, version, url, sha256?, size?, module? }` |
| DELETE | `/ota/firmwares/{id}` | Remove image and related OTA jobs |

### Triggering updates

| Method | Path | Description |
|--------|------|-------------|
| POST | `/ota/firmwares/{id}/notify` | Notify device or whole product: `{ "device_id"?, "force"? }` |
| POST | `/devices/{id}/ota` | Start OTA: `{ "firmware_id", "force"? }` |
| POST | `/devices/{id}/ota/cancel` | Cancel active update |
| GET | `/ota/jobs?device_id=` | List OTA jobs |

### Public download (no JWT)

`GET /ota/fw/{token}.bin` — ESP downloads firmware via HTTP without auth. URL: `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`.

`PUBLIC_BASE_URL` must be reachable from the device (LAN IP, not `localhost`).

### Auto-OTA

When `auto_ota` is enabled on the product (and not disabled on the device): on connect or version report, platform pushes the latest firmware if versions differ. Offline notifications create a **pending** job — pushed on reconnect.

---

## External plugins

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/plugins/external` | member | List external plugins |
| POST | `/plugins/external` | admin | Register in the active account; `id` is `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$` and cannot reuse another account's plugin id. Returns `{ ok, id, mqtt_user, mqtt_pass, mqtt_port, mqtt_host, mqtt_ws, rules_bus }` |
| DELETE | `/plugins/external/{id}` | admin | Delete |
| POST | `/plugins/external/{id}/regenerate-mqtt` | admin | Rotate MQTT credentials |

MQTT username format: `extplg_{account_slug}_{plugin_id}`. External plugins integrate over MQTT only (rules bus); there is no separate HTTP API for them.

Details: [08-external-plugins.md](08-external-plugins.md).

---

## Plugin policies (admin)

Per-account control of catalog plugins (built-in plugins like LED garland, schedule, prometheus-exporter, metrics-dashboard).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/plugins/policies` | List all catalog plugins with effective enabled/settings for the active account |
| PATCH | `/plugins/policies/{pluginId}` | Update policy: `{ "enabled"?, "settings"? }` |

When a plugin is disabled, it stops running for all devices in the account. Settings are plugin-specific JSON configurations.

---

<a id="monitoring"></a>

## Monitoring

Implemented in **v1.9.0**: account dashboards, metric queries, alert rules and notification delivery. This is separate from the [device dashboard](#device-dashboard) / `metrics-dashboard` plugin. Setup and delivery semantics: [Operations → Monitoring](11-operations.md#monitoring).

All paths below are relative to **`/api/v1/monitoring`** and scoped to the token's active account. Suspended accounts are denied (`403`). `member` includes viewers; `write` means admin/operator. **All channel access, even reads, requires admin.** Resource/device IDs are internal numeric IDs, not MQTT device names.

| Method | Path | Role | Result |
|--------|------|------|--------|
| GET | `/dashboards` | member | Dashboard summaries, each with `widgets: []` |
| POST | `/dashboards` | write | Create dashboard |
| GET | `/dashboards/{id}` | member | Full dashboard with widgets |
| PUT / DELETE | `/dashboards/{id}` | write | Replace / delete dashboard |
| GET | `/metrics?device_id=1` | member | Received metric catalog: `[{"key":"temperature","kind":"number","last_seen":1700000000000}]` |
| POST | `/query` | member | Query one widget, including an unsaved preview |
| GET | `/rules` | member | Full rule list |
| POST | `/rules` | admin | Create rule |
| PUT / DELETE | `/rules/{id}` | admin | Replace / delete rule |
| GET | `/channels` | admin | Channel list without secrets |
| POST | `/channels` | admin | Create channel |
| PUT / DELETE | `/channels/{id}` | admin | Replace / delete channel |
| POST | `/channels/{id}/test` | admin | Send a real test message; no body required |
| GET | `/events?limit=100` | member | Alert events, newest first |
| GET | `/deliveries?limit=100` | member | Outbox delivery records, newest first |

Lists are JSON arrays. Creates return `201` and the saved object; updates return `200` and the saved object; successful deletes/tests return `204`. There is **no** `GET /rules/{id}` or `GET /channels/{id}`; use their lists.

### Dashboards and widgets

`POST /api/v1/monitoring/dashboards` example (device `1` must belong to the active account):

```json
{
  "name": "Plant room",
  "widgets": [{
    "id": "temperature-line",
    "title": "Temperature",
    "type": "line",
    "sources": [{"device_id": 1, "key": "temperature", "label": "Room", "color": "#3d9cf0"}],
    "window_seconds": 3600,
    "aggregation": "avg",
    "width": 6,
    "min": 0,
    "max": 100
  }]
}
```

The saved object has `id`, `account_id`, `name`, `version` (initially `1`) and `widgets`. Layout is persisted in SQLite/PostgreSQL; widget data is queried, not stored as a snapshot.

For `PUT /dashboards/{id}`, send `name`, the **current `version`**, and the **complete `widgets` array**. The update replaces all widgets and increments the version. On `409`, refetch the full dashboard, reapply edits and submit its new version. Never save a list summary as a full dashboard: its empty `widgets` would remove the layout.

- Maximum 50 dashboards/account, 24 widgets/dashboard and 1–8 distinct `(device_id, key)` sources/widget. Widget IDs must be unique within a dashboard.
- `type`: `line`, `bar`, `stat`, `gauge`, `table`; `aggregation`: `avg`, `min`, `max`, `sum`, `last`; `width`: `6` or `12`.
- `window_seconds`: `60..2678400` (31 days); gauge requires `min < max`; source colors are six-digit hex (`#RRGGBB`, default `#3d9cf0`).
- Use numeric sources for line/bar/gauge and numeric aggregates; `last` also supports boolean/text values in stat/table. The wizard enforces type compatibility; numeric API aggregates ignore non-numeric rows.
- `/metrics` lists actual received telemetry keys (up to 1000/device), not product-schema declarations. `kind` is `number`, `boolean` or `text`; `last_seen` is receive time in Unix milliseconds.

### Widget query

`POST /api/v1/monitoring/query` accepts a full widget plus optional `from`/`to` Unix-millisecond timestamps:

```json
{
  "widget": {
    "id": "temperature-stat", "title": "Average temperature", "type": "stat",
    "sources": [{"device_id": 1, "key": "temperature", "label": "Room", "color": "#3d9cf0"}],
    "window_seconds": 3600, "aggregation": "avg", "width": 6, "min": 0, "max": 100
  },
  "from": 1700000000000,
  "to": 1700003600000
}
```

Illustrative `200` response when samples exist:

```json
{
  "from": 1700000000000,
  "to": 1700003600000,
  "series": [{
    "device_id": 1, "key": "temperature", "label": "Room", "color": "#3d9cf0",
    "points": [{"ts": 1700003599000, "value": 23.5}]
  }]
}
```

Omitted/zero `to` defaults to now; omitted/zero `from` defaults to `to - window_seconds * 1000`. Require `0 <= from < to`, at most 31 days, and `to` no more than 60 seconds ahead of server time. Queries use measurement timestamps.

Each series contains up to **240** bucketed points spanning the whole interval, not merely the last 240 samples. Stat/gauge return one whole-window aggregate per source. A point's `ts` is the latest contributing measurement time; `value` can be a number, boolean or string with `last`. No matching data yields `points: []`, **not zero**. Numeric aggregates use numeric samples; `last` selects the latest measurement per bucket.

### Alert rules

`POST /api/v1/monitoring/rules` example (events only until channels are selected):

```json
{
  "name": "High temperature", "device_id": 1, "metric": "temperature",
  "kind": "threshold", "operator": "gt", "threshold": 80, "recovery": 75,
  "for_seconds": 30, "missing_seconds": 300, "cooldown_seconds": 300,
  "enabled": true, "channel_ids": []
}
```

The response adds `id`, `account_id`, `state` and `since` (Unix ms). `PUT /rules/{id}` takes the complete rule configuration, not a patch; state/timing are server-managed. There is no client version field for rules.

| Field / kind | Contract |
|--------------|----------|
| `threshold` | Requires `metric`, numeric data and `operator` `gt` / `lt`. `gt`: fire above `threshold`, recover at/below `recovery`; `lt`: fire below, recover at/above. Recovery must be toward normal |
| `no_data` | No receive-time data for `missing_seconds`; empty `metric` covers any telemetry from the device |
| `offline` | Empty `metric`; offline status **or** stale `last_seen` satisfies the condition |
| `for_seconds` | `0..604800`; default `0` means immediate firing on the next qualifying evaluation |
| `missing_seconds` | `1..604800`; omitted/zero defaults to `300`; also controls threshold freshness, even though the threshold UI hides it |
| `cooldown_seconds` | `60..604800`; omitted/zero defaults to `300`; repeat interval while firing |
| `channel_ids` | Distinct IDs of this account's channels; empty means events only. Maximum 100 rules/account, 20 channels/account |

`missing_seconds` is **not** a disconnect grace period: offline status can qualify immediately. Use `for_seconds` for sustained offline detection. Thresholds require fresh measurement and receive timestamps; stale/non-numeric samples cannot resolve firing or cause reminders. Latest-measurement ordering prevents delayed older recovery samples from replacing a newer alarm.

States are `normal`, `pending`, `firing`, `resolved` and persist across restarts. Missing/invalid threshold data, cleared conditions or sufficiently long evaluation gaps break pending sustain. Disabled rules/devices and suspended accounts are not evaluated or sent notifications; pending state resets. Updating a rule resets state to `normal`, advances the internal revision and cancels queued deliveries from the prior configuration. Evaluation is server-side every 5 seconds, independent of browser refresh.

### Notification channels

`POST /api/v1/monitoring/channels`: replace the placeholder with your own bot token; it is **not** a usable credential.

```json
{
  "name": "Duty chat", "kind": "telegram", "destination": "@plant_alerts",
  "enabled": true, "secret": "<BOT_TOKEN>"
}
```

Secret-masked response shape (`201`; no `secret` field is returned):

```json
{
  "id": 7, "account_id": 1, "name": "Duty chat", "kind": "telegram",
  "destination": "@plant_alerts", "enabled": true, "has_secret": true
}
```

Telegram accepts a numeric chat ID or `@channel`; the bot must have permission to write there. Tokens are encrypted with AES-GCM using a `JWT_SECRET`-derived key and account/channel binding. `PUT /channels/{id}` supplies channel configuration; blank/omitted `secret` preserves an existing Telegram token. After `JWT_SECRET` rotation, re-enter tokens; there is no automatic migration.

For email use `kind: "email"`, one email address as `destination` and no secret. Sending uses existing `email.*` YAML / SMTP environment configuration; `EMAIL_MODE=log` fails notification delivery rather than marking it sent. See [SMTP/TLS setup](11-operations.md#monitoring).

Saving alone does not send a test, but an enabled rule may send. Explicit `POST /channels/{id}/test` sends a **real message** through an enabled channel; it is synchronous, not an outbox delivery. Remove channel references from rules before deleting a channel (`409` otherwise). Channel updates cancel its queued deliveries.

### History and errors

Both history endpoints accept `limit=1..500`, default `100` (the UI shows 100). Event fields: `id`, `rule_id`, `name`, `state`, `message`, `created_at`. Delivery fields: `id`, `event_id`, `channel_id`, `status`, `attempts`, `last_error`, `next_attempt`; timestamps are Unix ms. Errors are sanitized and never expose bot tokens.

Outbox statuses: `pending`, `sending`, `sent`, `failed`; at most 5 attempts with backoff and leased claims. Older pending/sending work for the same rule/channel blocks newer notifications until terminal completion. Delivery is **at-least-once**: transport ambiguity can cause duplicates, not exactly-once delivery. Monitoring history is not automatically telemetry-retention-pruned.

JSON bodies are limited to **128 KiB**; unknown fields and trailing JSON are rejected.

| Status | Meaning |
|--------|---------|
| `400` | Invalid JSON/ID/configuration/range/limit; dashboard quota (50) exceeded; foreign device/channel references in a rule body |
| `403` | Insufficient role, inactive membership or suspended active account |
| `404` | Missing/foreign resource addressed by ID; missing/foreign widget device; disabled channel on test |
| `409` | Dashboard version conflict, rule/channel quota exceeded, or deletion of a referenced channel |
| `413` | Request body too large |
| `500` | Unexpected service error or test-message delivery failure |

---

## Device dashboard

| Method | Path | Description |
|--------|------|-------------|
| GET | `/devices/{id}/dashboard` | Aggregated dashboard panels for metrics-dashboard plugin |

Response:

```json
{
  "panels": [
    {
      "id": "telemetry_chart",
      "type": "telemetry_chart",
      "title": "WiFi RSSI",
      "data": { "keys": ["wifi_rssi"], "points": [[1234567890, -65], ...] }
    }
  ]
}
```

Panel types: `telemetry_chart`, `telemetry_gauge`, `rpc_table`, `online_timeline`, `events_count`.

---

## Service endpoints

Root-level (not under `/api/v1`) unless the path says otherwise.

| Method | Path | Role | Description |
|--------|------|------|-------------|
| GET | `/metrics` | public | Prometheus metrics |
| GET | `/api/v1/mqtt/ca.pem` | public | MQTT TLS CA certificate (404 if `MQTT_TLS_CA` is not set) |
| any | `/mcp` | authenticated | Embedded MCP server (Streamable HTTP), see [10-mcp.md](10-mcp.md) |

### Setup wizard

Available only while the setup wizard is running (empty database on first start); public.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/setup/state` | Current wizard state |
| POST | `/api/v1/setup/step` | Save a wizard step |
| POST | `/api/v1/setup/apply` | Apply configuration, create first account and admin |
| POST | `/api/v1/setup/generate-tls` | Generate MQTT TLS `ca.pem`, `cert.pem`, `key.pem`: `{ "host"?, "dir"? }` |

---

## Roles

| Role | Read | Write (devices, RPC, OTA, …) |
|------|------|------------------------------|
| `admin` | yes | yes + user management |
| `operator` | yes | yes |
| `viewer` | yes | no |

The first user and first account are created by the setup wizard; that user is `admin`.
