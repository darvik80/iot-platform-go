# Operations

## Run locally (dev)

```bash
# Terminal 1 — infra
cd deploy && docker compose up -d rabbitmq nodered

# Terminal 2 — API
export MQTT_BROKER=tcp://localhost:1883
export SQLITE_PATH=./data/platform.db
go run ./cmd/platform
```

Dashboard: http://localhost:8080

## SQLite

- Path: `SQLITE_PATH` (default `./data/platform.db`)
- WAL mode enabled
- Hourly retention deletes telemetry / events / logs older than `TELEMETRY_RETENTION_DAYS`

Backup: stop writes or use `sqlite3 .backup` against a checkpointed copy.

## Health

- `GET /healthz` — process up
- `GET /readyz` — ingest/publish connected

## Token rotation

1. Operator rotates token in UI/API.
2. New RabbitMQ user is created (`user=pass=new_token`).
3. Device firmware must be updated; old token hash is revoked in SQLite.

Orphaned broker users from previous tokens may remain if the plaintext was never stored — remove unused users in RabbitMQ Management if needed.

## Logging

Structured logs to stdout (`slog`). Key events: MQTT/AMQP connect, subscribe/bind failures, HTTP requests (chi logger).

## Production notes

- Change `JWT_SECRET`, `ADMIN_PASSWORD`, `RABBITMQ_*`, `NODERED_MQTT_PASSWORD`
- Prefer MQTT TLS and HTTPS in front of `:8080`
- Do not expose RabbitMQ management publicly without auth hardening
- Enable Node-RED `adminAuth` in `deploy/nodered/settings.js`
