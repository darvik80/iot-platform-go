# Deploy Docker (Synology / compose)

Containerized platform deployment — convenient for NAS (Synology, QNAP) or any Docker host. Multi-stage build: Node.js → Go → Alpine (final image ~30 MB).

Production on VPS uses systemd instead — see [12-deploy-vds.md](12-deploy-vds.md).

---

## How it works

```
docker compose up -d --build
         │
         ├── node:22      → npm ci && npm run build  (React SPA)
         ├── golang:1.25  → go build                  (Go binary + embedded SPA)
         └── alpine:3.20  → /platform                 (runtime: ~30 MB)
                                    │
                              ┌─────┴──────┐
                              │  Container  │
                              │  :8080 HTTP │
                              │  :1883 MQTT │
                              │  :8083 WS   │
                              └─────┬──────┘
                                    │
                    /volume1/docker/iot-platform-go/
                    ├── data/        → SQLite + firmware
                    └── etc/.env     → configuration
```

---

## Quick start (local)

```bash
cd deploy
cp .env.example .env
# Edit .env — minimum: JWT_SECRET, PUBLIC_BASE_URL, BROKER_HOST_PUBLIC
docker compose up -d --build
```

- Dashboard: http://localhost:8080
- MQTT: `mqtt://localhost:1883`
- MQTT WS: `ws://localhost:8083/`

---

## Configuration

Minimum `.env`:

```bash
# --- Required ---
PUBLIC_BASE_URL=http://<host-ip>:9083
JWT_SECRET=<random-64-chars>
BROKER_HOST_PUBLIC=<host-ip>

# --- Admin ---
ADMIN_EMAIL=admin@local
ADMIN_PASSWORD=admin

# --- Database (inside container) ---
SQLITE_PATH=/data/platform.db
```

Full variable list — [11-operations.md](11-operations.md#configuration).

---

## Synology NAS

### 1. One-time setup

```bash
# SSH into Synology
mkdir -p /volume1/docker/iot-platform-go/{data,etc}
cp .env.example /volume1/docker/iot-platform-go/etc/.env
# Edit /volume1/docker/iot-platform-go/etc/.env
```

### 2. Gitea workflow (automatic deploy)

`.gitea/workflows/deploy-docker.yml` — **workflow_dispatch** (manual trigger on NAS runner):

1. Creates data directories
2. Builds image `localhost/iot-platform-go:latest`
3. Recreates container on ports:
   - HTTP: **9083** → 8080
   - MQTT TCP: **1883** → 1883
   - MQTT WS: **8083** → 8083
4. Mounts:
   - `/volume1/docker/iot-platform-go/data` → `/data` (SQLite + firmware)
   - `/volume1/docker/iot-platform-go/etc` → `/etc/iot-platform` (read-only, config)

### 3. Manual rebuild

```bash
export DOCKER_API_VERSION=1.41
docker build -t localhost/iot-platform-go:latest .
docker rm -f iot-platform-go || true
docker run -d \
  -p 9083:8080 \
  -p 1883:1883 \
  -p 8083:8083 \
  --restart unless-stopped \
  --name iot-platform-go \
  -v /volume1/docker/iot-platform-go/data:/data \
  -v /volume1/docker/iot-platform-go/etc:/etc/iot-platform:ro \
  localhost/iot-platform-go:latest
```

Dashboard: `http://<nas-ip>:9083` · MQTT: `<nas-ip>:1883`

---

## Updating

```bash
# Rebuild image
docker compose build --no-cache

# Recreate container (data persists in volume)
docker compose up -d --force-recreate
```

SQLite data is stored in the mounted `data/` directory — not lost on container recreation.

---

## Verification

```bash
# Container status
docker compose ps

# Logs
docker compose logs -f platform

# Health
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
```

---

## Port 1883 conflict

If another MQTT broker is running on the host (Mosquitto, RabbitMQ):

```bash
# Change MQTT port in .env
TARGET_MQTT_PORT=1884

# Or in docker run
-p 1884:1883
```

Connect devices to `mqtt://<host>:1884`.

---

## Node-RED (optional)

```yaml
# deploy/docker-compose.yml — add to services:
  nodered:
    image: nodered/node-red:4.0
    ports:
      - "1880:1880"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    environment:
      MQTT_BROKER_HOST: host.docker.internal
```

Node-RED connects to the platform's MQTT broker via `host.docker.internal:1883`.
Automation setup — [06-automation.md](06-automation.md).

---

## Port summary

| Host port | Container port | Protocol | Purpose |
|-----------|---------------|----------|---------|
| 9083 | 8080 | HTTP | Dashboard + API |
| 1883 | 1883 | MQTT TCP | Devices |
| 8083 | 8083 | MQTT WS | Devices (WebSocket) |
| 1880 | 1880 | HTTP | Node-RED (optional) |

VPS (systemd): [12-deploy-vds.md](12-deploy-vds.md).
