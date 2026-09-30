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
