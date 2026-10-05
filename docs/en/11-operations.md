# Operations

Guide for running, configuring, and maintaining the platform.

---

## Configuration

Priority (first non-empty wins): **OS env → `./config.yml` → `/opt/iot-platform/etc/config.yml`**.

YAML keys are flattened to env names (`server.public_base_url` → `SERVER_PUBLIC_BASE_URL`, `mqtt.addr` → `MQTT_ADDR`). Environment variables override YAML.

The binary does **not** read `.env` files (there is no dotenv loader at startup). A `.env` file only takes effect when something else injects it into the process environment — Docker Compose `env_file:` or a systemd `EnvironmentFile=`.

Full examples: `config.yml.example`, `.env.example`.

### Key variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `HTTP_ADDR` | `:8080` | HTTP listen address (dashboard + API) |
| `MQTT_ADDR` | `:1883` | MQTT TCP listener |
| `MQTT_TLS_ADDR` | `:8883` | MQTT over TLS listener (active only when `MQTT_TLS_CERT` + `MQTT_TLS_KEY` are set) |
| `MQTT_PUBLIC_PORT` / `MQTT_WS_PUBLIC_PORT` / `MQTT_TLS_PUBLIC_PORT` | listen port | Port announced to clients (device credentials, Settings, MQTT status) when it differs from the listen port, e.g. Docker `-p 11883:1883`. Does not move the listener |
| `DATABASE_DRIVER` | `sqlite` | `sqlite` \| `postgres` |
| `DATABASE_DSN` | `/opt/iot-platform/lib/platform.db` | SQLite file path or PostgreSQL URL (`postgres://user:pass@host:5432/db?sslmode=disable`) |
| `SQLITE_PATH` | `/opt/iot-platform/lib/platform.db` | Legacy: SQLite file used when `DATABASE_DSN` is empty |
| `DATA_DIR` | directory of the SQLite file (`./data` for Postgres) | Base directory for runtime data (firmware, etc.) |
| `OTA_DIR` | `{DATA_DIR}/firmware` | Firmware storage directory |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Public URL (OTA, email, OAuth) |
| `JWT_SECRET` | — | JWT signing key. If unset or left at the dev default, the platform uses `settings.jwt_secret` from the DB, or generates a random one and stores it there (logged as a warning) |
| `TIMEZONE` | `UTC` | Timezone for schedules and quiet hours |
| `TELEMETRY_RETENTION_DAYS` | `30` | Auto-cleanup of old data |
| `LOG_LEVEL` | `info` | Log level: `debug` \| `info` \| `warn` \| `error` |

`ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_LOGIN` and `BOOTSTRAP_ACCOUNT_*` **do not** create an admin or account — use the setup wizard below.

`DATABASE_MAX_OPEN_CONNS` (alias `DB_MAX_OPEN_CONNS`) is parsed but currently **not applied**: the pool size is hardcoded in `internal/db/db.go` (SQLite — 1 connection, PostgreSQL — 8 open / 2 idle).

---

## First start: setup wizard

If the configured database is empty (no users and no accounts) — or no database is configured — the platform does not start normally. Instead it serves only the **setup wizard** on `HTTP_ADDR`:

- Web UI: `http://<host>:8080/setup` (the SPA redirects there automatically)
- API: `GET /api/v1/setup/state`, `POST /api/v1/setup/step`, `POST /api/v1/setup/apply`, `POST /api/v1/setup/generate-tls`

Steps: **database** (driver + DSN) → **admin** (login, email, password) → **account** (slug, name) → **SMTP** → **JWT secret** (random value pre-generated) → **MQTT TLS** (paths, or generate a self-signed CA + server cert) → **public URL** (`PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`) → **apply**.

On apply the wizard creates the admin user, the default account and the admin membership, stores settings in the `settings` table and writes `config.yml` to `/opt/iot-platform/etc/config.yml` if that directory exists and is writable, otherwise to `./config.yml`. The process then reloads the config (values from the written file override env) and continues starting the full platform — no restart needed.

At startup the configuration comes from env → `./config.yml` → `/opt/iot-platform/etc/config.yml`; the only value read back from the `settings` table is `jwt_secret` (used when no real secret is configured). `config.yml` is written with mode `0600`. If the log shows `failed to write config.yml`, copy at least `database.*` into a config file or env manually. Also make sure the DSN chosen in step 1 is writable by the service user (the wizard pre-fills `/opt/iot-platform/lib/platform.db`).

There is **no default login** (`admin/admin` or `admin@cogitor.local` does not exist on a fresh install).

> **Security**: the setup endpoints are **unauthenticated** while the wizard runs — whoever reaches the port first chooses the admin password. Run the wizard only on a trusted network or via `localhost` / an SSH tunnel, and do not publish port 8080 until setup is finished. `GET /api/v1/setup/state` never returns secrets (only `admin_password_set` / `smtp_pass_set` / `jwt_secret_set` flags and a DSN with the password masked). After apply the setup routes are removed and `GET /api/v1/setup/state` returns `{"completed":true}`. `generate-tls` writes only into the default cert directory or a relative path inside the working directory.
>
> The user created by the wizard is the **platform admin** (`settings.platform_admin_user_id`): only this user can list or delete all users. Self-registered users are admins of their personal account only, regardless of `REGISTER_DEFAULT_ROLE`.

---

## Local run

```bash
# Terminal 1 — platform (API + MQTT); empty DB → setup wizard on :8080
export DATABASE_DRIVER=sqlite
export DATABASE_DSN=./data/platform.db
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

Selected by `DATABASE_DRIVER`:

- **SQLite** (default) — pure-Go driver, WAL mode, `busy_timeout=5000`, single connection. File: `DATABASE_DSN` (legacy `SQLITE_PATH`), default `/opt/iot-platform/lib/platform.db`.
- **PostgreSQL** — `DATABASE_DRIVER=postgres`, `DATABASE_DSN=postgres://…` (if the DSN is empty the default is `postgres://localhost:5432/iot_platform?sslmode=disable`).

Common to both:

- Hourly cleanup of telemetry, events, and logs older than `TELEMETRY_RETENTION_DAYS`
- Migrations in `internal/db/migrations/` — auto-applied at startup

**Backup**:

- SQLite: stop writes or use `sqlite3 platform.db ".backup /backup/platform-$(date +%F).db"` (safe with WAL).
- PostgreSQL: `pg_dump -Fc -d "$DATABASE_DSN" -f /backup/iot-platform-$(date +%F).dump`; restore with `pg_restore -d <db> <file>`.
- Also back up `config.yml` (contains `JWT_SECRET`), the firmware directory and MQTT TLS certificates.

---

## Health

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Process is alive (liveness probe) |
| `GET /readyz` | MQTT listener + DB ready (readiness probe); returns `"status": "setup"` while the setup wizard is running |

Both are unauthenticated — for Kubernetes / Docker healthcheck.

---

## OTA firmware updates

1. Upload `.bin` on the **OTA** page (or register an external URL)
2. Platform stores the file in `OTA_DIR` (default `{DATA_DIR}/firmware`; `DATA_DIR` defaults to the directory of the SQLite file) and generates a public URL: `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`
3. ESP downloads firmware **without JWT** via HTTP
4. Notify device or whole product → MQTT `v1/me/o/upd` (short style) or `iot/v1/{product}/{device}/down/ota/update` (full style)
5. Enable **Auto-OTA** on the product — auto-push on reconnect

`PUBLIC_BASE_URL` must be reachable from the device (NAS/LAN IP, not `localhost`).

---

## Timezone

`TIMEZONE` (IANA, e.g. `Asia/Bangkok`) — for schedules and LED garland quiet hours. Default `UTC`. A plugin can override via `config.tz`.

---

## Device token rotation

1. Operator rotates token in UI or API
2. New hash stored in the database; live MQTT session for `{product}.{device}` is disconnected
3. Device firmware must be updated with the new token

---

## Logging

Structured logs to **stdout** (`slog`):

- `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe` — info
- `ingest` (including telemetry), `command`, downlink `publish` — info

Docker: `docker logs -f iot-platform-go` (container name from `deploy-docker.yml`). Debian package / systemd: `/var/log/iot-platform/platform.log`.

```bash
# Enable debug logs (Docker)
docker run ... -e LOG_LEVEL=debug ...
docker logs -f iot-platform-go
```

Dashboard → **MQTT** shows live sessions and broker event trail.

---

## Production checklist

- [ ] Complete the setup wizard from a trusted network, then restart the service
- [ ] Use a strong admin password (set in the wizard)
- [ ] Keep the random `JWT_SECRET` generated by the wizard (never the dev default)
- [ ] Set `PUBLIC_BASE_URL` to the real host (not `localhost`)
- [ ] Enable MQTT TLS or reverse proxy with HTTPS
- [ ] Don't expose MQTT `:1883` to public internet without firewall
- [ ] Enable `adminAuth` in Node-RED (`deploy/nodered/settings.js`)
- [ ] Set up database backup (SQLite `.backup` or PostgreSQL `pg_dump`)
