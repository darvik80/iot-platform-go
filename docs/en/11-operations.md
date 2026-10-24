# Operations

Guide for running, configuring, and maintaining the platform.

---

## Configuration

Priority (first non-empty wins): **OS env → `./config.yml` → `/opt/iot-platform/etc/config.yml`**. The `.yaml` extension is accepted too (`config.yml` wins if both exist). The startup log line `config sources files=…` shows which files were applied; an unreadable or invalid file is logged as `config file ignored` with the reason.

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
| `BROKER_HOST_PUBLIC` | host of `PUBLIC_BASE_URL` (wizard) / `localhost` | MQTT host announced to devices for TCP and TLS. A URL or `host:port` is reduced to the host. Behind Cloudflare use a DNS-only name — see [12-deploy-vds](12-deploy-vds.md#cloudflare) |
| `MQTT_WS_PUBLIC_URL` | — | Full MQTT-over-WebSocket URL announced to clients, e.g. `wss://iot.example.com/mqtt` behind nginx. Empty = `ws://{BROKER_HOST_PUBLIC}:{WS port}/` |
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
MQTT_BROKER_HOST=host.docker.internal docker compose up -d nodered
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

<a id="monitoring"></a>

## Monitoring (v1.9.0)

Open `/monitoring` in the dashboard: **Dashboards**, **Alerts**, **Channels**, **History**. This is account-wide monitoring across devices, separate from the per-device `metrics-dashboard` plugin. See the [Monitoring API](02-api.md#monitoring) for request examples and limits.

### Access and persistence

- Everything belongs to the **active account**; switch accounts before configuring monitoring. Suspended accounts are denied access.
- `admin` and `operator` can create, edit, reorder and delete dashboards/widgets; `viewer` can read them.
- All members can read rules, events and delivery history and query widgets. Only `admin` can change rules or access channels at all (including listing and testing).
- Layouts, rules, rule state and the notification outbox are stored in the configured SQLite/PostgreSQL database and survive restarts. Widget values are queried from telemetry, not saved as cached snapshots.

### Build a dashboard

1. Create a named dashboard, then open the widget wizard.
2. Select a device and its **actually received metrics**, not just product-schema declarations. Add up to eight distinct device/metric sources, including sources from different devices in the account.
3. Choose `line`, `bar`, `stat`, `gauge` or `table`; set the title, labels, colors, time window and aggregation (`avg`, `min`, `max`, `sum`, `last`).
4. Use numeric metrics for line/bar/gauge and numeric aggregates. `last` also permits boolean/text values in stat/table. For a gauge, set a minimum smaller than its maximum.
5. Preview, save and arrange widgets; width `6` is half-width and `12` is full-width. Narrow screens stack widgets.

The wizard cannot offer a metric until telemetry has arrived. An empty interval means **no data**, not zero. Numeric aggregates ignore non-numeric samples; `last` selects the latest measurement in each bucket.

| Limit / behavior | Value |
|------------------|-------|
| Dashboards per account / widgets per dashboard | 50 / 24 |
| Sources per widget | 1–8 distinct device/metric pairs |
| Widget window | 60 seconds–31 days |
| Series query | Up to 240 points per source, bucketed across the entire requested window |
| Stat / gauge query | One whole-window aggregate per source when data exists |

Dashboard saves replace the **entire widget list** using the current `version`. If another editor saved first, `409` means reload/refetch the dashboard and reapply your changes; do not blindly retry an old layout. Relevant dashboard, alert and history views refresh about every 15 seconds.

### Alert rules

An admin selects the device, rule kind, timing and optional notification channels. Account limit: **100 rules** and **20 channels**.

| Kind | Condition |
|------|-----------|
| `threshold` | Fresh numeric metric crosses `gt` (greater than) or `lt` (less than) threshold; a separate recovery value provides hysteresis |
| `no_data` | No telemetry received for `missing_seconds`; optionally restrict to one metric, or leave it empty for any device telemetry |
| `offline` | Device status is offline **or** its `last_seen` is stale for `missing_seconds`; no metric is used |

For example, `gt`, threshold `80`, recovery `75` fires above 80 and resolves at or below 75; `lt` reverses the comparisons (recovery must be at or above the threshold).

| API field | Meaning / default |
|-----------|-------------------|
| `for_seconds` | Sustain duration before firing; `0` means the next evaluation can fire immediately |
| `missing_seconds` | Missing-data/stale-device interval and threshold freshness; default `300`, minimum `1` |
| `cooldown_seconds` | Repeat-notification interval while firing; default `300`, minimum `60` |

All three durations are capped at `604800` seconds (7 days). The current threshold form hides `missing_seconds`, but the API exposes it; its default still applies.

**Offline timing is not a disconnect grace period.** A disconnect can satisfy the condition immediately, regardless of `missing_seconds`. Set `for_seconds` to require sustained offline status before firing.

Threshold evaluation requires both a fresh measurement timestamp and fresh receive time. Stale/missing/non-numeric data does **not** resolve a firing threshold rule or justify a repeat notification. Measurements are ordered by their timestamps so a delayed older recovery sample cannot override a newer alarm sample. `no_data` instead uses receive time.

Rules move through `normal`, `pending`, `firing` and `resolved`. A broken condition, missing threshold data or a sufficiently long evaluation gap resets pending sustain; disabled rules/devices and suspended accounts also reset pending state. State survives restart, but downtime does not prove uninterrupted sustain.

Updating a rule resets its state to `normal`, advances its internal revision and cancels queued deliveries from its previous configuration. Disabled rules/devices and suspended accounts are not evaluated or sent notifications. With **no channels selected**, events are still recorded, but nothing is sent.

### Email channels (SMTP)

Use the platform's **existing email configuration**, via environment variables or the corresponding `email.*` YAML keys; monitoring introduces no separate SMTP mechanism.

| Environment | YAML key | Setting |
|-------------|----------|---------|
| `EMAIL_MODE` | `email.mode` | `smtp`, or leave unset with a host configured; explicit `log` disables real delivery |
| `SMTP_HOST` | `email.smtp_host` | SMTP server hostname; required for sending |
| `SMTP_PORT` | `email.smtp_port` | Default `587` |
| `SMTP_FROM` | `email.smtp_from` | Valid sender address accepted by your SMTP provider |
| `SMTP_USER` / `SMTP_PASS` | `email.smtp_user` / `email.smtp_pass` | Credentials when the server requires authentication |

Port **465** uses implicit TLS. Other ports use STARTTLS; plaintext is permitted only when the **actual connected peer is loopback**, not merely when a hostname looks local. A remote server without STARTTLS fails delivery.

Create an enabled email channel with one recipient address, then explicitly **Test** it. `EMAIL_MODE=log` is a failed notification attempt, **not** a delivered/log-only success; outbox retries eventually become `failed`.

### Telegram channels

1. Obtain a bot token and choose a numeric chat ID (including negative group IDs) or an `@channel` destination.
2. Make sure the bot can write there: start the bot in a private chat, or add it to the group/channel with the required posting permissions.
3. An admin creates an enabled Telegram channel with the destination and token, then explicitly tests it with a real message.
4. Select that channel on the intended alert rule.

The token is encrypted at rest with **AES-GCM**, using a key derived from `JWT_SECRET` and authenticated binding to the account/channel. The API never returns it, only `has_secret`. Leaving the token blank/omitted when updating an existing Telegram channel preserves it.

**Keep `JWT_SECRET` with your backups.** Rotating it makes stored Telegram tokens unreadable: re-enter every Telegram token after rotation; there is no automatic migration. Do not put tokens into screenshots, examples or logs.

Saving a channel alone does **not** send a test, but an enabled rule referencing it may send a notification. The explicit Test action sends a real message; a disabled channel cannot be tested. A channel referenced by a rule must be removed from that rule before deletion.

### Delivery, history and troubleshooting

Rule evaluation runs **server-side every 5 seconds**. A separate notification worker sends messages independently of browser sessions; closing `/monitoring` does not stop alerts, and slow SMTP/Telegram does not block evaluation.

- The persisted outbox uses `pending`, `sending`, `sent`, `failed`, with leased claims and at most **5 attempts**. Retryable failures use backoff; permanent failures can terminate earlier.
- Older unfinished notifications for the same rule/channel block newer ones until completion or terminal failure, preserving firing/recovery order.
- Delivery is **at-least-once**, not exactly-once: ambiguous transport outcomes or recovery after a crash can produce duplicates. `sent` indicates transport acceptance, not that a person read the message.
- Rule/channel updates cancel queued work; before sending, the worker rechecks account, device, rule and channel availability. An already accepted external message cannot be recalled.
- History displays the latest **100** events/deliveries. The API accepts `limit=1..500` (default `100`) and exposes attempts, the sanitized last error and next retry time.
- Monitoring history is **not** automatically pruned by `TELEMETRY_RETENTION_DAYS`; include it in database capacity planning and backups. Telemetry retention still affects widget queries.

If no message arrives, check the active account, device/rule/channel enabled flags, selected channels, sustain/cooldown, and History delivery status. For SMTP check mode/host/TLS/authentication; for Telegram check destination, bot permissions and whether `JWT_SECRET` changed. Test deliberately after fixing configuration, then observe new rule events.

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
