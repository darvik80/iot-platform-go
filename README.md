# Crearts IoT Platform

Self-hosted IoT backend + ops dashboard for the Crearts device MQTT protocol (**v1**).

| Layer | Stack |
|-------|--------|
| API / ingest | Go 1.22+, chi, SQLite |
| Broker | RabbitMQ MQTT (+ management API for device users) |
| Automation | Node-RED (rules bus on `platform/v1/...`) |
| UI | React + Bootstrap 5 (Vite), embedded in Go binary |

## Quick start

### 1. Broker + Node-RED

```bash
cd deploy
cp .env.example .env
docker compose up -d rabbitmq nodered
```

| Service | URL |
|---------|-----|
| RabbitMQ Management | http://localhost:15672 (`crearts` / `crearts`) |
| MQTT | `mqtt://localhost:1883` |
| Node-RED | http://localhost:1880 |

### 2. Platform API

```bash
# build dashboard then API (from repo root)
cd web && npm ci && npm run build && cd ..
go run ./cmd/platform
```

Dev UI with hot reload (API must be running on :8080):

```bash
cd web && npm run dev
```

Or: `make build` / `make web`.

Open http://localhost:8080 — default login `admin@crearts.local` / `admin`.

Or run the full stack (includes platform image):

```bash
cd deploy
docker compose up -d --build
```

### 3. Register a device

1. Create a **product** (e.g. `esp32-cam`)
2. **Add device** → copy the one-time access token
3. Flash firmware with:

```cpp
CreartsCredentials::createAccessToken("esp32-cam", "cam-001", "localhost", "<token>");
```

MQTT CONNECT:

```
client_id = {product_id}.{device_id}
username  = {product_id}.{device_id}
password  = <access_token>
```

## Configuration

| Env | Default | Description |
|-----|---------|-------------|
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `SQLITE_PATH` | `./data/platform.db` | SQLite file |
| `MQTT_BROKER` | `tcp://localhost:1883` | MQTT broker for publish |
| `MQTT_USERNAME` / `MQTT_PASSWORD` | `crearts` | Platform service account |
| `RABBITMQ_MGMT_URL` | `http://localhost:15672` | User provisioning |
| `JWT_SECRET` | (dev default) | Dashboard JWT signing key |
| `TELEMETRY_RETENTION_DAYS` | `30` | Cleanup job |
| `RPC_DEFAULT_TIMEOUT_MS` | `30000` | RPC wait |
| `LAB_ALLOW_UNKNOWN_DEVICES` | `false` | Reject unknown devices |
| `RULES_BUS_ENABLED` | `true` | Node-RED event/command bus |
| `RULES_BUS_PREFIX` | `platform/v1` | Bus topic prefix |
| `NODERED_URL` | `http://localhost:1880` | Dashboard link |
| `BROKER_HOST_PUBLIC` | `localhost` | Host shown in device credentials |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@crearts.local` / `admin` | Bootstrap admin |

## Architecture

```
ESP devices ──MQTT──► RabbitMQ ◄──AMQP/MQTT── platform-api (Go)
                         │                         │
                         │ platform/v1/events…     │ SQLite + REST/WS
                         ▼                         │
                      Node-RED ◄──── REST ─────────┘
                         │
Dashboard (SPA) ──HTTPS──┘
```

- **Ingest**: AMQP queue on `amq.topic` (`iot.v1.*.*.up.#`, `v1.#`) so short-topic uplinks resolve via RabbitMQ `user_id` (= `{product}.{device}`).
- **Downlink**: MQTT publish using the device’s `topic_style` (`short` or `full`).
- **Rules bus**: normalized JSON on `platform/v1/events/...`; commands on `platform/v1/commands/...`.

## Documentation

- [REST API](docs/api.md)
- [Auth & users](docs/auth.md)
- [MQTT protocol](docs/mqtt.md) (summary; full spec lives in embed-framework)
- [Automation / Node-RED](docs/automation.md)
- [Operations](docs/operations.md)
- [Synology / Gitea deploy](docs/deploy-synology.md)
- [Product schema](docs/product-schema.md)

## Acceptance checklist

- [ ] Device connects with token and appears **online**
- [ ] Telemetry shows in device detail / WS refresh
- [ ] Desired attribute patch reaches device (`v1/a/upd` or full topic)
- [ ] RPC round-trip with `id` in body
- [ ] NTP request gets `serverRecvTime` / `serverSendTime`
- [ ] Token shown once at create/rotate; rotate provisions new broker user
- [ ] Node-RED receives `platform/v1/events/.../telemetry`

## License

Proprietary — Crearts.
