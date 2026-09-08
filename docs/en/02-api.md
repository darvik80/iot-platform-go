# REST API (`/api/v1`)

Auth: `Authorization: Bearer <jwt>` after `POST /auth/login`, or a personal API token.

WebSocket: `GET /api/v1/ws?token=<jwt>` — JSON messages `{ "event", "data" }`.

## Auth

### `GET /auth/config`

Public flags: `register_enabled`, `require_email_verify`, `google_enabled`.

### `POST /auth/register`

```json
{ "login": "jdoe", "email": "jdoe@example.com", "password": "secret123", "display_name": "Jane" }
```

### `POST /auth/login`

```json
{ "email": "admin@cogitor.local", "password": "admin" }
```

`email` accepts login **or** email. See [03-auth.md](03-auth.md) for Google OAuth and verification.

### `GET /me`

Current user.

## Overview & health

| Method | Path | Notes |
|--------|------|-------|
| GET | `/healthz` | liveness (no auth) |
| GET | `/readyz` | DB + embedded MQTT ready (no auth); includes `mqtt_clients` |
| GET | `/overview` | online/offline counts, MQTT flag + client count |
| GET | `/mqtt` | live MQTT sessions + recent broker events |
| GET | `/settings` | retention, broker host, Node-RED URL, timezone |

## Products

| Method | Path |
|--------|------|
| GET | `/products` |
| POST | `/products` |
| GET | `/products/{id}` |
| PUT | `/products/{id}` |
| DELETE | `/products/{id}` |

```json
{
  "product_id": "esp32-cam",
  "name": "ESP32-CAM",
  "description": ""
}
```

## Devices

| Method | Path | Notes |
|--------|------|-------|
| GET | `/devices?product_id=&status=&q=` | list |
| POST | `/devices` | returns `credentials` **once**. Topic style is detected from MQTT (`v1/…` / `v1/me/…` → short, `iot/v1/…` → full). |
| GET | `/devices/{id}` | detail + attrs + latest telemetry |
| PUT | `/devices/{id}` | metadata |
| DELETE | `/devices/{id}` | |
| POST | `/devices/{id}/token/rotate` | new credentials once |
| POST | `/devices/{id}/enable` | |
| POST | `/devices/{id}/disable` | |
| GET | `/plugins` | builtin plugin catalog |
| GET | `/devices/{id}/plugins` | bindings for this device |
| POST | `/devices/{id}/plugins` | `{ "plugin_id", "config"?, "enabled"? }` |
| PUT | `/devices/{id}/plugins/{bindingId}` | update config / enabled |
| DELETE | `/devices/{id}/plugins/{bindingId}` | |
| POST | `/devices/{id}/plugins/{bindingId}/apply` | run one plugin frame now |

### Create response

```json
{
  "device": { "id": 1, "product_id": "esp32-cam", "device_id": "cam-001", "status": "offline", "...": "..." },
  "credentials": {
    "product_id": "esp32-cam",
    "device_id": "cam-001",
    "access_token": "...",
    "username": "esp32-cam.cam-001",
    "client_id": "esp32-cam.cam-001",
    "broker_host": "localhost",
    "broker_port": 1883,
    "topic_style": "auto",
    "snippet": "CogitorCredentials::createAccessToken(...);",
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

Dashboard create/rotate modal can fill `wifi` and **Copy JSON** / download `{product}-{device}.json` for firmware provisioning. Wi‑Fi is not stored on the platform.
```

## Telemetry & attributes

| Method | Path |
|--------|------|
| GET | `/devices/{id}/telemetry?keys=a,b&from=&to=&limit=` |
| GET | `/devices/{id}/telemetry/latest` |
| GET | `/devices/{id}/attributes` |
| PATCH | `/devices/{id}/attributes/desired` |

Desired body is a flat JSON object; platform persists and publishes `attributes/update`.

## RPC

`POST /devices/{id}/rpc?wait=1`

```json
{ "method": "reboot", "params": { "delayMs": 1000 }, "timeout_ms": 30000 }
```

`GET /devices/{id}/rpc` — history.

## Events / logs / OTA

| Method | Path |
|--------|------|
| GET | `/events?device_id=&severity=&from=&to=&limit=` |
| GET | `/logs?device_id=&level=&from=&to=&limit=` |
| GET | `/ota/firmwares?product_id=` |
| POST | `/ota/firmwares` | register an **external** URL `{ product_id, version, url, sha256?, size?, module? }` |
| POST | `/ota/firmwares/upload` | multipart: `file` + `product_id` + `version` + optional `module` / `sha256` / `sign` — stores the image and returns a public download URL. Same product+module+version **replaces** the previous image |
| DELETE | `/ota/firmwares/{id}` | remove image, hosted file, and OTA jobs for that firmware |
| POST | `/ota/firmwares/{id}/notify` | `{ "device_id"?: pk, "force"?: bool }` — omit `device_id` to notify the whole product |
| POST | `/devices/{id}/ota` | `{ "firmware_id": 1, "force"?: false }` |
| POST | `/devices/{id}/ota/cancel` |
| GET | `/ota/jobs?device_id=` |

Public (no JWT — ESP HTTP GET): `GET /ota/fw/{token}.bin` — hosted firmware download. URL is `{PUBLIC_BASE_URL}/ota/fw/{token}.bin` and is what MQTT `v1/o/upd` sends as `url`. Set `PUBLIC_BASE_URL` to a host the device can reach (LAN IP, not `localhost`). Images live under `OTA_DIR` or `{sqlite dir}/firmware`. Max upload 32MB.

Product `auto_ota` + device `auto_ota` (default on): when a device comes online or reports `ota/version`, platform pushes the latest firmware if versions differ. Device `ota/query` always gets an update if a newer image exists (pull). Offline notify creates a **pending** job; it is pushed on reconnect.

## Roles

| Role | Read | Write (devices, RPC, OTA, …) |
|------|------|------------------------------|
| `admin` | yes | yes |
| `operator` | yes | yes |
| `viewer` | yes | no |

Bootstrap user is `admin`.
