# Operations

## Run locally (dev)

```bash
# Terminal 1 — API + embedded MQTT
export SQLITE_PATH=./data/platform.db
go run ./cmd/platform

# Terminal 2 (optional) — Node-RED
cd deploy && docker compose up -d nodered
```

Dashboard: http://localhost:8080  
MQTT: `mqtt://localhost:1883`

If ESP32 fails with `transport_read(): EOF` / `mqtt_message_receive() returned -2`, another broker almost always already owns `:1883` (RabbitMQ MQTT, Mosquitto, Windows WinNAT). On Windows Go used to share the port (`SO_REUSEADDR`); the platform now binds **exclusively** on IPv4 `0.0.0.0:1883` and will refuse to start if the port is taken. Stop the other listener or set `MQTT_ADDR=:1884` (and the same port in firmware).

## SQLite

- Path: `SQLITE_PATH` (default `./data/platform.db`)
- WAL mode enabled
- Hourly retention deletes telemetry / events / logs older than `TELEMETRY_RETENTION_DAYS`

Backup: stop writes or use `sqlite3 .backup` against a checkpointed copy.

## Health

- `GET /healthz` — process up
- `GET /readyz` — embedded MQTT listener up

## OTA

1. On **OTA**, upload a `.bin` (product + version). Platform stores it under `{SQLITE_PATH dir}/firmware` (or `OTA_DIR`) and generates a public URL `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`. ESP downloads that URL **without JWT**. Same product+module+version replaces the image (new token/URL). Alternatively register an external HTTPS URL.
2. **Notify product** — MQTT `v1/o/upd` / `…/down/ota/update` to every device of that product (`url` + `sha256` + `size`).
3. **Notify device** — same, one device (also on the device page).
4. Enable **Auto-OTA** on the product (device can opt out). New firmware + reconnect / version report push automatically.
5. Device progress arrives on `v1/o/p`; jobs show status/step on the OTA page.

`PUBLIC_BASE_URL` must be reachable from the device (NAS/LAN IP, not `localhost`).

## Token rotation

1. Operator rotates token in UI/API.
2. New hash is stored; the live MQTT session for `{product}.{device}` is disconnected.
3. Device firmware must be updated with the new token.

## Logging

Structured logs to stdout (`slog`). Key events: MQTT listen, MQTT connect/auth/ACL/ingest, HTTP requests (chi logger).

Dashboard → **MQTT** shows live sessions and a recent broker event trail (`GET /api/v1/mqtt`).

## Production notes

- Change `JWT_SECRET`, `ADMIN_PASSWORD`, `NODERED_MQTT_PASSWORD`
- Prefer MQTT TLS (front proxy / listener) and HTTPS in front of `:8080`
- Do not expose MQTT `:1883` to the public internet without auth + firewall
- If Synology (or another host) still runs RabbitMQ MQTT on `:1883`, stop it or bind the platform to another port (`MQTT_ADDR`)
- Enable Node-RED `adminAuth` in `deploy/nodered/settings.js`
