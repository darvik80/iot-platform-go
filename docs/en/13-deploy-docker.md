# Deploy Docker (Synology / compose)

Containerized platform deployment — convenient for NAS (Synology, QNAP) or any Docker host. Multi-stage build: Node.js → Go → Alpine (final image ~30 MB).

Three ways to run the same image:

- **Docker Compose** — `deploy/docker-compose.yml` (service `platform`, optional `nodered`), ports and host folders in `deploy/.env`;
- **`docker run`** — any host;
- **Gitea CI** — `.gitea/workflows/deploy-docker.yml` rebuilds and recreates the container on push to `master` (Synology runner).

Production on VPS uses systemd instead — see [12-deploy-vds.md](12-deploy-vds.md).

---

## Image layout

```
docker build -t localhost/iot-platform-go:latest .
         │
         ├── node:22-bookworm     → npm ci && npm run build  (React SPA)
         ├── golang:1.26-bookworm → go build                 (Go binary + embedded SPA)
         └── alpine:3.20          → /opt/iot-platform/bin/platform
                                    │
                              ┌─────┴──────┐
                              │  Container │   user "platform" (uid 1000)
                              │  :8080 HTTP│   via docker-entrypoint.sh (su-exec)
                              │  :1883 MQTT│
                              │  :8083 WS  │
                              │  :8883 TLS │
                              └─────┬──────┘
                                    │
                 <host dir>/
                 ├── data/ → /opt/iot-platform/lib   SQLite platform.db + firmware/
                 └── etc/  → /opt/iot-platform/etc   config.yml (setup wizard) + certs/
```

Image facts (`Dockerfile`, `docker-entrypoint.sh`):

| | Value |
|---|---|
| Container ports | `8080` HTTP, `1883` MQTT, `8083` MQTT over WebSocket, `8883` MQTT over TLS (`EXPOSE`) |
| Listen env | `HTTP_ADDR=:8080`, `MQTT_ADDR=:1883`, `MQTT_WS_ADDR=:8083`, `MQTT_TLS_ADDR=:8883` |
| Database | `SQLITE_PATH=/opt/iot-platform/lib/platform.db` (default DSN; the setup wizard pre-fills the same path) |
| Volumes | `/opt/iot-platform/lib`, `/opt/iot-platform/etc` (`VOLUME`) |
| Health | `HEALTHCHECK` → `GET /healthz` (also answers in setup-wizard mode) |
| Entrypoint | as root: `chown` of `lib/` and `etc/` to uid 1000, then `su-exec platform`; with `--user` it runs the command as is and you own volume permissions |

**Ports:** change the **host** side only (`-p HOST:CONTAINER`). Container ports are fixed by the image; if a host port differs from the container port, tell the platform which port clients must use — `PUBLIC_BASE_URL` for HTTP and `MQTT_PUBLIC_PORT` / `MQTT_WS_PUBLIC_PORT` / `MQTT_TLS_PUBLIC_PORT` for MQTT (shown in device credentials, Settings and MQTT status). Compose and the CI workflow set the MQTT ones automatically.

**Folders:** both bind mounts can be root-owned on the host — the entrypoint hands them to uid 1000 on every start. `config.yml` holds secrets and is written with mode `0600`.

---

## Docker Compose

```bash
cd deploy
cp .env.example .env              # ports, host folders, image name
cp platform.env.example platform.env   # optional: pinned platform settings
docker compose up -d --build      # platform only
docker compose --profile nodered up -d   # + Node-RED
```

`deploy/.env` (compose substitution only):

| Variable | Default | Meaning |
|---|---|---|
| `IOT_IMAGE` | `localhost/iot-platform-go:latest` | Image to run; `--build` tags the local build with it. Prebuilt: `darvik80/iot-platform:latest` (amd64 + arm64) |
| `IOT_CONTAINER_NAME` | `iot-platform-go` | Container name |
| `IOT_HTTP_PORT` | `8080` | Host port → 8080 |
| `IOT_MQTT_PORT` | `1883` | Host port → 1883, also `MQTT_PUBLIC_PORT` |
| `IOT_MQTT_WS_PORT` | `8083` | Host port → 8083, also `MQTT_WS_PUBLIC_PORT` |
| `IOT_MQTT_TLS_PORT` | `8883` | Host port → 8883, also `MQTT_TLS_PUBLIC_PORT` |
| `IOT_DATA_DIR` | `./iot/data` | Host folder → `/opt/iot-platform/lib` |
| `IOT_ETC_DIR` | `./iot/etc` | Host folder → `/opt/iot-platform/etc` |
| `NODERED_PORT` | `1880` | Node-RED host port (profile `nodered`) |

`deploy/platform.env` is passed with `env_file` and is optional. Every variable in it **overrides** `config.yml` from the setup wizard, so keep only what you want to pin (`PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`, `JWT_SECRET`, `DATABASE_*`, `MQTT_TLS_*`, …) and never leave a variable with an empty value — an empty variable still counts as set and hides the `config.yml` value.

---

## docker run

```bash
docker build -t iot-platform-go .
docker run -d --name iot-platform-go --restart unless-stopped \
  -p 8080:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v "$PWD/iot/data:/opt/iot-platform/lib" \
  -v "$PWD/iot/etc:/opt/iot-platform/etc" \
  iot-platform-go
```

Different host ports, e.g. HTTP on 9083 and MQTT on 11883:

```bash
docker run -d --name iot-platform-go --restart unless-stopped \
  -p 9083:8080 -p 11883:1883 -p 8083:8083 -p 8883:8883 \
  -e MQTT_PUBLIC_PORT=11883 \
  -e PUBLIC_BASE_URL=http://192.168.1.22:9083 \
  -e BROKER_HOST_PUBLIC=192.168.1.22 \
  -v /srv/iot/data:/opt/iot-platform/lib \
  -v /srv/iot/etc:/opt/iot-platform/etc \
  iot-platform-go
```

The binary does not read `.env` files; use `-e VAR=…` or `--env-file <file>`.

- Setup wizard (first start): `http://<host>:<HTTP port>/setup`
- MQTT: `mqtt://<host>:<MQTT port>`, WS: `ws://<host>:<WS port>/`, TLS: `mqtts://<host>:<TLS port>` (only when cert/key are configured)

---

## Configuration

Priority: **OS env → `./config.yml` (= `/opt/iot-platform/config.yml`, not on a volume) → `/opt/iot-platform/etc/config.yml`**. Full variable list — [11-operations.md](11-operations.md#configuration).

### First start — setup wizard

With an empty database the container serves the **setup wizard** on the HTTP port (`/setup`, API `/api/v1/setup/*`) — there is no default admin login and `ADMIN_*` variables are ignored. Details: [Operations → setup wizard](11-operations.md#first-start-setup-wizard).

- Keep the SQLite DSN pre-filled in step 1 (`/opt/iot-platform/lib/platform.db`) — it is on the data volume.
- The wizard writes `config.yml` to `/opt/iot-platform/etc/config.yml` (the etc volume) and continues starting the platform; the next container start reads it from there.
- In step 7 enter the **host-side** public URL and broker host (e.g. `http://192.168.1.22:9083`, `192.168.1.22`).
- Setup endpoints are unauthenticated: run the wizard from a trusted network.

### MQTT TLS

Put the files into `<host etc dir>/certs/mqtt/` (or let the wizard generate them into `/opt/iot-platform/etc/certs/mqtt`) and set `MQTT_TLS_CERT` / `MQTT_TLS_KEY` (paths inside the container: `/opt/iot-platform/etc/certs/mqtt/…`). See [05-mqtt-tls.md](05-mqtt-tls.md).

---

## Synology NAS + Gitea CI

`.gitea/workflows/deploy-docker.yml` runs on **push to `master`** and on **workflow_dispatch**, in one job on the NAS runner (the image is built where the container runs):

1. Builds `localhost/<container>:latest` with `VERSION` (from `version.txt`) and `COMMIT`.
2. Recreates the container with `--restart unless-stopped`, publishing all four ports and mounting `<IOT_HOST_DIR>/data` → `/opt/iot-platform/lib`, `<IOT_HOST_DIR>/etc` → `/opt/iot-platform/etc`. Docker creates missing host folders.
3. Waits until the container healthcheck reports `healthy` (≈90 s); otherwise prints the last 100 log lines and fails.
4. Prunes dangling images.

Override anything with **repository variables** (Settings → Actions → Variables), no file edits needed:

| Variable | Default | Meaning |
|---|---|---|
| `IOT_CONTAINER_NAME` | `iot-platform-go` | Container and image name |
| `IOT_HTTP_PORT` | `9083` | Host port → 8080 |
| `IOT_MQTT_PORT` | `1883` | Host port → 1883 (+ `MQTT_PUBLIC_PORT`) |
| `IOT_MQTT_WS_PORT` | `8083` | Host port → 8083 (+ `MQTT_WS_PUBLIC_PORT`) |
| `IOT_MQTT_TLS_PORT` | `8883` | Host port → 8883 (+ `MQTT_TLS_PUBLIC_PORT`) |
| `IOT_HOST_DIR` | `/volume1/docker/iot-platform-go` | Contains `data/` and `etc/` |
| `PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`, `TIMEZONE` | — | Passed as `-e` only when set; otherwise `config.yml` applies |

Dashboard: `http://<nas-ip>:9083` · MQTT: `<nas-ip>:1883`.

`.gitea/workflows/deploy-docker-hub.yml` builds the same `Dockerfile` for `linux/amd64` + `linux/arm64` and pushes `darvik80/iot-platform` (`latest` from `master`, version tags from `v*`).

### Migrating from the old `/data` layout

Older images stored the database in `/data` and the config in `/etc/iot-platform`. The host folders stay the same (`data/`, `etc/`); only the container side moved. The entrypoint links `/data` → `/opt/iot-platform/lib` and `/etc/iot-platform` → `/opt/iot-platform/etc`, so an existing `config.yml` with `dsn: /data/platform.db` or cert paths under `/etc/iot-platform/certs` keeps working. You can later edit those paths to the new ones.

### Manual rebuild

```bash
export DOCKER_API_VERSION=1.43
docker build -t localhost/iot-platform-go:latest .
docker rm -f iot-platform-go || true
docker run -d --name iot-platform-go --restart unless-stopped \
  -p 9083:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v /volume1/docker/iot-platform-go/data:/opt/iot-platform/lib \
  -v /volume1/docker/iot-platform-go/etc:/opt/iot-platform/etc \
  localhost/iot-platform-go:latest
```

---

## Updating

Data lives in the host folders, so recreating the container is safe:

```bash
cd deploy && docker compose up -d --build        # compose
# or: docker build … && docker rm -f iot-platform-go && docker run … (same arguments)
```

Back up `data/platform.db` (stop the container first, or run `sqlite3 platform.db ".backup out.db"` on the host — the image has no `sqlite3`) and `etc/config.yml` before major upgrades.

---

## Verification

```bash
docker ps --filter name=iot-platform-go          # STATUS shows (healthy)
docker logs -f iot-platform-go
curl -fsS http://localhost:<HTTP port>/healthz
curl -fsS http://localhost:<HTTP port>/readyz     # 503 until MQTT is up; "setup" in wizard mode
docker exec iot-platform-go ls -l /opt/iot-platform/lib /opt/iot-platform/etc
```

---

## Port 1883 conflict

If another broker uses 1883 on the host, publish MQTT on another host port: `IOT_MQTT_PORT=1884` (compose / Gitea variable) or `-p 1884:1883 -e MQTT_PUBLIC_PORT=1884` (`docker run`). Devices then connect to `mqtt://<host>:1884`, and the platform shows that port in device credentials.

---

## Node-RED (optional)

Service `nodered` in `deploy/docker-compose.yml`, started with `--profile nodered`:

- image `nodered/node-red:4.0`, container `cogitor-nodered`, port `${NODERED_PORT:-1880}:1880`
- `extra_hosts: host.docker.internal:host-gateway`
- env `NODERED_CREDENTIAL_SECRET` (default `cogitor-nodered-dev`), `MQTT_BROKER_HOST` (default `host.docker.internal`; Podman `host.containers.internal`; `platform` when both run from this compose file), `ACCOUNT_SLUG`
- bind mounts `./nodered/data:/data`, `./nodered/settings.js`, `./nodered/flows.json`, `./nodered/entrypoint.sh` (relative to `deploy/`)

The flow template connects to port 1883 of `MQTT_BROKER_HOST`; with `MQTT_BROKER_HOST=platform` that is the container port, regardless of `IOT_MQTT_PORT`. Node-RED has no dedicated MQTT credentials — register it as an **external plugin** in the dashboard and use the returned `extplg_…` user/password. Automation setup — [06-automation.md](06-automation.md).

---

## Port summary

| Host port (default) | Container port | Protocol | Purpose |
|---|---|---|---|
| `IOT_HTTP_PORT` (8080 compose / 9083 CI) | 8080 | HTTP | Dashboard + API (+ setup wizard on first start) |
| `IOT_MQTT_PORT` (1883) | 1883 | MQTT TCP | Devices |
| `IOT_MQTT_WS_PORT` (8083) | 8083 | MQTT WS | Devices (WebSocket) |
| `IOT_MQTT_TLS_PORT` (8883) | 8883 | MQTT TLS | Devices (TLS; listener only when cert/key configured) |
| `NODERED_PORT` (1880) | 1880 | HTTP | Node-RED (optional) |

VPS (systemd): [12-deploy-vds.md](12-deploy-vds.md).
