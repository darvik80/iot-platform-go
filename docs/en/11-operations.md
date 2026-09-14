# Operations

Guide for running, configuring, and maintaining the platform.

---

## Configuration

Priority (first non-empty wins): **OS env → `.env` → `./config.yml` → `/etc/iot-platform-go/config.yml`**.

YAML keys: `server.public_base_url`, `server.http_addr`, `mqtt.addr`. Environment variables (`.env` / compose) override YAML.

Full examples: `config.yml.example`, `.env.example`.

### Key variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `HTTP_ADDR` | `:8080` | HTTP listen address (dashboard + API) |
| `MQTT_ADDR` | `:1883` | MQTT TCP listener |
| `SQLITE_PATH` | `./data/platform.db` | SQLite database file |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Public URL (OTA, email, OAuth) |
| `JWT_SECRET` | dev default | JWT signing key — **change in production** |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@cogitor.local` / `admin` | Bootstrap admin |
| `TIMEZONE` | `UTC` | Timezone for schedules and quiet hours |
| `TELEMETRY_RETENTION_DAYS` | `30` | Auto-cleanup of old data |
| `LOG_LEVEL` | `info` | Log level: `debug` \| `info` \| `warn` \| `error` |

---

## Local run

```bash
# Terminal 1 — platform (API + MQTT)
export SQLITE_PATH=./data/platform.db
go run ./cmd/platform

# Terminal 2 (optional) — Node-RED
cd deploy && docker compose up -d nodered
```

- Dashboard: http://localhost:8080
- MQTT: `mqtt://localhost:1883`

### Port 1883 conflict

If ESP32 gets `transport_read(): EOF` / `mqtt_message_receive() returned -2` — another process occupies `:1883` (RabbitMQ, Mosquitto, Windows WinNAT). Platform binds to `0.0.0.0:1883` and refuses to start if port is busy. Fix: stop the other listener or set `MQTT_ADDR=:1884`.

---

## Database

SQLite with WAL mode (concurrent reads during ingest).

- Path: `SQLITE_PATH` (default `./data/platform.db`)
- Hourly cleanup of telemetry, events, and logs older than `TELEMETRY_RETENTION_DAYS`
- Migrations in `internal/db/migrations/` — auto-applied at startup

**Backup**: stop writes or use `sqlite3 .backup` on a checkpointed copy.

---

## Health

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Process is alive (liveness probe) |
| `GET /readyz` | MQTT listener + DB ready (readiness probe) |

Both are unauthenticated — for Kubernetes / Docker healthcheck.

---

## OTA firmware updates

1. Upload `.bin` on the **OTA** page (or register an external URL)
2. Platform stores in `{SQLITE_PATH dir}/firmware` and generates a public URL: `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`
3. ESP downloads firmware **without JWT** via HTTP
4. Notify device or whole product → MQTT `v1/o/upd`
5. Enable **Auto-OTA** on the product — auto-push on reconnect

`PUBLIC_BASE_URL` must be reachable from the device (NAS/LAN IP, not `localhost`).

---

## Timezone

`TIMEZONE` (IANA, e.g. `Asia/Bangkok`) — for schedules and LED garland quiet hours. Default `UTC`. A plugin can override via `config.tz`.

---

## Device token rotation

1. Operator rotates token in UI or API
2. New hash stored in SQLite; live MQTT session for `{product}.{device}` is disconnected
3. Device firmware must be updated with the new token

---

## Logging

Structured logs to **stdout** (`slog`):

- `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe` — info
- `ingest` (except telemetry), `command`, downlink `publish` — info
- Telemetry ingest — **debug**

Docker: `docker compose logs -f platform`

```bash
# Enable debug logs
LOG_LEVEL=debug docker compose up -d
docker compose logs -f platform
```

Dashboard → **MQTT** shows live sessions and broker event trail.

---

## Production checklist

- [ ] Change `JWT_SECRET` to a random value
- [ ] Change `ADMIN_PASSWORD`
- [ ] Set `PUBLIC_BASE_URL` to the real host (not `localhost`)
- [ ] Enable MQTT TLS or reverse proxy with HTTPS
- [ ] Don't expose MQTT `:1883` to public internet without firewall
- [ ] Enable `adminAuth` in Node-RED (`deploy/nodered/settings.js`)
- [ ] Set up SQLite backup
