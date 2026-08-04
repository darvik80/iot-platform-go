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
{ "email": "admin@crearts.local", "password": "admin" }
```

`email` accepts login **or** email. See [auth.md](auth.md) for Google OAuth and verification.

### `GET /me`

Current user.

## Overview & health

| Method | Path | Notes |
|--------|------|-------|
| GET | `/healthz` | liveness (no auth) |
| GET | `/readyz` | DB + MQTT/AMQP ready (no auth) |
| GET | `/overview` | online/offline counts, MQTT flag |
| GET | `/settings` | retention, broker host, Node-RED URL |

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
  "description": "",
  "topic_style_default": "short"
}
```

## Devices

| Method | Path | Notes |
|--------|------|-------|
| GET | `/devices?product_id=&status=&q=` | list |
| POST | `/devices` | returns `credentials` **once** |
| GET | `/devices/{id}` | detail + attrs + latest telemetry |
| PUT | `/devices/{id}` | metadata |
| DELETE | `/devices/{id}` | |
| POST | `/devices/{id}/token/rotate` | new credentials once |
| POST | `/devices/{id}/enable` | |
| POST | `/devices/{id}/disable` | |

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
    "topic_style": "short",
    "snippet": "CreartsCredentials::createAccessToken(...);"
  }
}
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
| POST | `/ota/firmwares` |
| POST | `/devices/{id}/ota` | `{ "firmware_id": 1 }` |
| POST | `/devices/{id}/ota/cancel` |
| GET | `/ota/jobs?device_id=` |

## Roles

| Role | Read | Write (devices, RPC, OTA, …) |
|------|------|------------------------------|
| `admin` | yes | yes |
| `operator` | yes | yes |
| `viewer` | yes | no |

Bootstrap user is `admin`.
