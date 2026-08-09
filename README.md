# Crearts IoT Platform

Self-hosted IoT backend + ops dashboard for the Crearts device MQTT protocol (**v1**).

| Layer | Stack |
|-------|--------|
| API / ingest | Go 1.22+, chi, SQLite |
| Broker | Embedded MQTT ([mochi-mqtt](https://github.com/mochi-mqtt/server)) inside the platform process |
| Automation | Node-RED (rules bus on `platform/v1/...`) |
| UI | React + Bootstrap 5 (Vite), embedded in Go binary |
| MCP | Embedded at `/mcp` (JWT) — LLM device tools |

## Quick start

### 1. Platform (HTTP + MQTT)

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

MQTT listen: `mqtt://localhost:1883`.

### 2. Node-RED (optional)

```bash
cd deploy
cp .env.example .env
docker compose up -d nodered
```

| Service | URL |
|---------|-----|
| Dashboard / API | http://localhost:8080 |
| MQTT | `mqtt://localhost:1883` |
| Node-RED | http://localhost:1880 |

Full stack (platform image + Node-RED):

```bash
cd deploy
docker compose up -d --build
```

If another broker (e.g. RabbitMQ MQTT) already owns `:1883`, stop it or set `MQTT_ADDR=:1884` and `BROKER_HOST_PUBLIC` accordingly.

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

YAML: `server.public_base_url`. `.env`: `PUBLIC_BASE_URL` (wins if set). See `config.yml.example` and `.env.example`.

| Env | Default | Description |
|-----|---------|-------------|
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `SQLITE_PATH` | `./data/platform.db` | SQLite file |
| `OTA_DIR` | `{sqlite dir}/firmware` | Hosted OTA `.bin` storage |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Absolute URLs (email, Google, **OTA download**). Use a LAN IP devices can reach |
| `TIMEZONE` | `UTC` | IANA zone for schedule / LED quiet hours. Binding `tz` overrides |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error`. Telemetry ingest is debug |
| `MQTT_ADDR` | `:1883` | Embedded MQTT TCP listen |
| `MQTT_WS_ADDR` | (off) | Optional MQTT-over-WebSocket listen |
| `JWT_SECRET` | (dev default) | Dashboard JWT signing key |
| `TELEMETRY_RETENTION_DAYS` | `30` | Cleanup job |
| `RPC_DEFAULT_TIMEOUT_MS` | `30000` | RPC wait |
| `LAB_ALLOW_UNKNOWN_DEVICES` | `false` | Reject unknown devices |
| `RULES_BUS_ENABLED` | `true` | Node-RED event/command bus |
| `RULES_BUS_PREFIX` | `platform/v1` | Bus topic prefix |
| `NODERED_URL` | `http://localhost:1880` | Dashboard link |
| `NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD` | `nodered` | Rules-bus MQTT account |
| `BROKER_HOST_PUBLIC` | `localhost` | Host shown in device credentials |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@crearts.local` / `admin` | Bootstrap admin |

## Architecture

```
ESP devices ──MQTT──► platform-api (Go, embedded broker)
                         │
                         ├── SQLite + REST/WS
                         └── platform/v1/events…  ──► Node-RED
                                      ▲
                         platform/v1/commands… ◄──┘

Dashboard (SPA) ──HTTPS──► platform-api
```

- **Ingest**: hook on device PUBLISH (`iot/v1/.../up/...`, `v1/#`); identity from MQTT username `{product}.{device}`.
- **Downlink**: inline publish using the **detected** topic style (`v1/…` or `v1/me/…` → short; `iot/v1/…` → full).
- **Rules bus**: normalized JSON on `platform/v1/events/...`; commands on `platform/v1/commands/...`.

## Documentation

- [REST API](docs/api.md)
- [Auth & users](docs/auth.md)
- [MQTT protocol](docs/mqtt.md) (summary; full spec lives in embed-framework)
- [Automation / Node-RED](docs/automation.md)
- [Device plugins](docs/plugins.md)
- [Operations](docs/operations.md)
- [Synology / Gitea deploy](docs/deploy-synology.md)
- [MCP (LLM device tools)](docs/mcp.md)
- [Product schema](docs/product-schema.md)

## Acceptance checklist

- [ ] Device connects with token and appears **online**
- [ ] Telemetry shows in device detail / WS refresh
- [ ] Desired attribute patch reaches device (`v1/a/upd` or full topic)
- [ ] RPC round-trip with `id` in body
- [ ] NTP request gets `serverRecvTime` / `serverSendTime`
- [ ] Token shown once at create/rotate; rotate drops the old MQTT session
- [ ] Node-RED receives `platform/v1/events/.../telemetry`

## License

Proprietary — Crearts.
