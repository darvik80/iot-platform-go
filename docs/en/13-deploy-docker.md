# Deploy Docker (Synology / compose)

Containerized platform deployment — convenient for NAS (Synology, QNAP) or any Docker host. Multi-stage build: Node.js → Go → Alpine (final image ~30 MB).

The platform container is built from the root `Dockerfile` and started with `docker run` (that is what the CI workflow does). `deploy/docker-compose.yml` contains **only** the optional Node-RED service — there is no `platform` service in compose.

Production on VPS uses systemd instead — see [12-deploy-vds.md](12-deploy-vds.md).

---

## How it works

```
docker build -t localhost/iot-platform-go:latest .
         │
         ├── node:22-bookworm    → npm ci && npm run build  (React SPA)
         ├── golang:1.26-bookworm → go build                (Go binary + embedded SPA)
         └── alpine:3.20         → /platform                (runtime: ~30 MB)
                                    │
                              ┌─────┴──────┐
                              │  Container  │   user "platform" (uid 1000)
                              │  :8080 HTTP │   via docker-entrypoint.sh (su-exec)
                              │  :1883 MQTT │
                              │  :8083 WS   │
                              │  :8883 TLS  │
                              └─────┬──────┘
                                    │
                    /volume1/docker/iot-platform-go/
                    ├── data/ → /data               SQLite (/data/platform.db) + firmware
                    └── etc/  → /etc/iot-platform   config.yml (written by setup wizard), certs
```

Image facts (`Dockerfile`, `docker-entrypoint.sh`):

- `EXPOSE 8080 1883 8083 8883`
- `ENV SQLITE_PATH=/data/platform.db`, `HTTP_ADDR=:8080`, `MQTT_ADDR=:1883`
- The entrypoint `chown`s `/data` to `platform` and then drops privileges with `su-exec platform`. `/etc/iot-platform` is **not** chowned — if you want the setup wizard to persist `config.yml` there, make the host directory writable for uid 1000.

---

## Quick start (local)

```bash
# Platform
docker build -t iot-platform-go .
mkdir -p ./run/data ./run/etc && sudo chown -R 1000:1000 ./run/etc
docker run -d --name iot-platform-go \
  -p 8080:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v "$PWD/run/data:/data" \
  -v "$PWD/run/etc:/etc/iot-platform" \
  iot-platform-go

# Optional: Node-RED
cd deploy && docker compose up -d
```

- Setup wizard (first start): http://localhost:8080/setup
- Dashboard: http://localhost:8080
- MQTT: `mqtt://localhost:1883`
- MQTT WS: `ws://localhost:8083/`
- MQTT TLS: `mqtts://localhost:8883` (only when TLS cert/key are configured)

---

## Configuration

The binary reads **OS env → `./config.yml` → `/etc/iot-platform/config.yml`**. It does not read `.env` files; to pass variables use `docker run -e VAR=…` or `docker run --env-file <file>`.

### First start — setup wizard

With an empty `/data/platform.db` the container serves the **setup wizard** on port 8080 (`/setup`, API `/api/v1/setup/*`) — there is no default admin login and `ADMIN_*` variables are ignored. Details: [Operations → setup wizard](11-operations.md#first-start-setup-wizard).

- In step 1 set the SQLite DSN to **`/data/platform.db`** (the wizard pre-fills `/var/lib/iot-platform/platform.db`, which is not on a volume inside the container).
- The wizard writes `config.yml` to `/etc/iot-platform/config.yml` if that directory is writable by uid 1000; otherwise to `./config.yml` (= `/config.yml`, not writable either) and only logs a warning. Without a persisted `config.yml` the next start falls back to env/defaults (the database settings are lost; the JWT secret is recovered from the `settings` table if the DB is reachable) — so either make `etc/` writable or pass `-e DATABASE_DSN=…` / `-e JWT_SECRET=…` explicitly.
- Setup endpoints are unauthenticated: run the wizard from a trusted network / localhost, then restart the container.

### Optional env

```bash
docker run ... \
  -e PUBLIC_BASE_URL=http://<host-ip>:9083 \
  -e BROKER_HOST_PUBLIC=<host-ip> \
  -e JWT_SECRET=<random-64-chars> \
  -e TIMEZONE=Asia/Bangkok \
  ...
# PostgreSQL instead of SQLite:
#  -e DATABASE_DRIVER=postgres -e DATABASE_DSN=postgres://user:pass@db:5432/iot_platform?sslmode=disable
```

Env values override `config.yml`. Full variable list — [11-operations.md](11-operations.md#configuration).

---

## Synology NAS

### 1. One-time setup

```bash
# SSH into Synology
mkdir -p /volume1/docker/iot-platform-go/{data,etc}
chown 1000:1000 /volume1/docker/iot-platform-go/etc   # lets the setup wizard write config.yml
```

### 2. Gitea workflow (automatic deploy)

`.gitea/workflows/deploy-docker.yml` — runs on **push to `master`** and on **workflow_dispatch** (manual trigger), on the NAS runner:

1. Creates `/volume1/docker/iot-platform-go/{data,etc}`
2. Builds image `localhost/iot-platform-go:latest` (`DOCKER_API_VERSION=1.43`)
3. Recreates container `iot-platform-go` (`--restart unless-stopped`) on ports:
   - HTTP: **9083** → 8080 (`TARGET_PORT`)
   - MQTT TCP: **1883** → 1883 (`TARGET_MQTT_PORT`)
   - MQTT WS: **8083** → 8083 (`TARGET_MQTT_WS_PORT`)
   - 8883 (MQTT TLS) is **not** published by the workflow — add `-p 8883:8883` if you enable TLS
4. Mounts (both read-write, no `:ro`):
   - `/volume1/docker/iot-platform-go/data` → `/data` (SQLite + firmware)
   - `/volume1/docker/iot-platform-go/etc` → `/etc/iot-platform` (config.yml, certs)
5. No `-e` / `--env-file` is passed — configuration comes from the setup wizard's `config.yml`.

### 3. Manual rebuild

```bash
export DOCKER_API_VERSION=1.43
docker build -t localhost/iot-platform-go:latest .
docker rm -f iot-platform-go || true
docker run -d \
  -p 9083:8080 \
  -p 1883:1883 \
  -p 8083:8083 \
  --restart unless-stopped \
  --name iot-platform-go \
  -v /volume1/docker/iot-platform-go/data:/data \
  -v /volume1/docker/iot-platform-go/etc:/etc/iot-platform \
  localhost/iot-platform-go:latest
```

Dashboard: `http://<nas-ip>:9083` · MQTT: `<nas-ip>:1883`

---

## Updating

```bash
# Rebuild image and recreate container (data persists in the bind mounts)
docker build --no-cache -t localhost/iot-platform-go:latest .
docker rm -f iot-platform-go
docker run -d ...   # same arguments as above
```

SQLite data is stored in the mounted `data/` directory — not lost on container recreation.

---

## Verification

```bash
# Container status
docker ps --filter name=iot-platform-go

# Logs
docker logs -f iot-platform-go

# Health
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:8080/readyz
```

---

## Port 1883 conflict

If another MQTT broker is running on the host (Mosquitto, RabbitMQ), publish MQTT on another host port:

```bash
# docker run
-p 1884:1883

# or in the Gitea workflow: env TARGET_MQTT_PORT: 1884 in .gitea/workflows/deploy-docker.yml
```

`TARGET_MQTT_PORT` is a workflow variable, not a platform setting. Connect devices to `mqtt://<host>:1884`.

---

## Node-RED (optional)

`deploy/docker-compose.yml` already defines the only compose service, `nodered`:

- image `nodered/node-red:4.0`, container `cogitor-nodered`, port `1880:1880`
- `extra_hosts: host.docker.internal:host-gateway`
- env `NODERED_CREDENTIAL_SECRET` (default `cogitor-nodered-dev`), `MQTT_BROKER_HOST` (default `host.docker.internal`; for Podman use `host.containers.internal`)
- bind mounts `./nodered/data:/data`, `./nodered/settings.js`, `./nodered/flows.json`, `./nodered/entrypoint.sh` (paths relative to `deploy/`)

```bash
cd deploy && docker compose up -d
```

Node-RED connects to the platform's MQTT broker via `host.docker.internal:1883`. It has no dedicated MQTT credentials anymore — register it as an **external plugin** in the dashboard and use the returned `extplg_…` MQTT user/password. Automation setup — [06-automation.md](06-automation.md).

---

## Port summary

| Host port | Container port | Protocol | Purpose |
|-----------|---------------|----------|---------|
| 9083 | 8080 | HTTP | Dashboard + API (+ setup wizard on first start) |
| 1883 | 1883 | MQTT TCP | Devices |
| 8083 | 8083 | MQTT WS | Devices (WebSocket) |
| 8883 | 8883 | MQTT TLS | Devices (TLS; only if cert/key configured, not published by the CI workflow) |
| 1880 | 1880 | HTTP | Node-RED (optional, compose) |

VPS (systemd): [12-deploy-vds.md](12-deploy-vds.md).
