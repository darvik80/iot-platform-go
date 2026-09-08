# REST API

All endpoints on `/api/v1`. Auth: `Authorization: Bearer <JWT>` or `Bearer <API token>`.

WebSocket for real-time updates: `GET /api/v1/ws?token=<jwt>` — JSON messages `{ "event", "data" }`.

---

## Authentication

### `GET /auth/config`

Public. Returns flags: `register_enabled`, `require_email_verify`, `google_enabled`.

### `POST /auth/register`

```json
{ "login": "jdoe", "email": "jdoe@example.com", "password": "secret123", "display_name": "Jane" }
```

Login: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`. Default role: `REGISTER_DEFAULT_ROLE` (usually `viewer`).

### `POST /auth/login`

```json
{ "email": "admin@cogitor.local", "password": "admin" }
```

The `email` field accepts **login or email**. Returns `{ "token": "<jwt>" }`.

Details: [03-auth.md](03-auth.md).

### `GET /me`

Current user (profile, role).

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
| POST | `/devices` | Create. Returns `credentials` **once** |
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

`POST /devices/{id}/rpc?wait=1`

```json
{ "method": "reboot", "params": { "delayMs": 1000 }, "timeout_ms": 30000 }
```

`wait=1` — wait for device response. Without `wait` — fire-and-forget.

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
| POST | `/ota/firmwares/upload` | Multipart: `file` + `product_id` + `version` + optional `module` / `sha256` |
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

## External plugins (admin)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/plugins/external` | List external plugins |
| POST | `/plugins/external` | Register; returns `token`, `mqtt_user`, `mqtt_pass` |
| DELETE | `/plugins/external/{id}` | Delete |
| POST | `/plugins/external/{id}/regenerate-mqtt` | Rotate MQTT credentials |

JSON-RPC endpoint for external plugins: `POST /api/plugins/external/rpc` (auth by plugin token).

Methods: `platform.heartbeat`, `platform.poll_events`, `platform.ack_events`, `platform.invoke_rpc`.

Details: [08-external-plugins.md](08-external-plugins.md).

---

## Roles

| Role | Read | Write (devices, RPC, OTA, …) |
|------|------|------------------------------|
| `admin` | yes | yes + user management |
| `operator` | yes | yes |
| `viewer` | yes | no |

Bootstrap user is `admin` (from env `ADMIN_LOGIN` / `ADMIN_PASSWORD`).
