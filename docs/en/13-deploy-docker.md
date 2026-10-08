# Deploy Docker (Synology / compose)

Containerized platform deployment — convenient for NAS (Synology, QNAP) or any Docker host. Multi-stage build: Node.js → Go → Alpine (final image ~30 MB).

Ways to run the same image:

- **Docker Hub** — prebuilt multi-arch image `darvik80/iot-platform`, no repository or build tools needed on the host (see [From Docker Hub](#from-docker-hub));
- **Docker Compose** — root `docker-compose.yml` (service `platform`, optional `nodered`), ports and host folders in `.env`;
- **`docker run`** — any host, local build;
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

## From Docker Hub

The image is published by `.gitea/workflows/deploy-docker-hub.yml` (see [Docker Hub publishing](#docker-hub-publishing)):

| Tag | Content |
|---|---|
| `darvik80/iot-platform:latest` | last build of `master` |
| `darvik80/iot-platform:X.Y.Z` | release `vX.Y.Z` — pin this in production |

Platforms: `linux/amd64`, `linux/arm64` (Docker picks the right one; Synology on ARM, Raspberry Pi 64-bit work as is).

### docker run

```bash
mkdir -p /srv/iot-platform/data /srv/iot-platform/etc
docker pull darvik80/iot-platform:latest
docker run -d --name iot-platform-go --restart unless-stopped \
  -p 8080:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v /srv/iot-platform/data:/opt/iot-platform/lib \
  -v /srv/iot-platform/etc:/opt/iot-platform/etc \
  darvik80/iot-platform:latest
```

Open `http://<host>:8080/setup` and complete the [setup wizard](#first-start--setup-wizard). With other host ports add `-e MQTT_PUBLIC_PORT=…` (and `MQTT_WS_PUBLIC_PORT` / `MQTT_TLS_PUBLIC_PORT`) as in [docker run](#docker-run-1).

### Docker Compose without the repository

Copy the root [`docker-compose.yml`](../../docker-compose.yml) to the host (only the `platform` service is needed) or use this minimal file:

```yaml
services:
  platform:
    image: darvik80/iot-platform:${IOT_TAG:-latest}
    container_name: iot-platform-go
    restart: unless-stopped
    ports:
      - "${IOT_HTTP_PORT:-8080}:8080"
      - "${IOT_MQTT_PORT:-1883}:1883"
      - "${IOT_MQTT_WS_PORT:-8083}:8083"
      - "${IOT_MQTT_TLS_PORT:-8883}:8883"
    environment:
      MQTT_PUBLIC_PORT: ${IOT_MQTT_PORT:-1883}
      MQTT_WS_PUBLIC_PORT: ${IOT_MQTT_WS_PORT:-8083}
      MQTT_TLS_PUBLIC_PORT: ${IOT_MQTT_TLS_PORT:-8883}
    env_file:
      - path: ./platform.env
        required: false
    volumes:
      - ./data:/opt/iot-platform/lib
      - ./etc:/opt/iot-platform/etc
```

```bash
docker compose up -d
```

### Synology Container Manager

1. **Registry** → search `darvik80/iot-platform` → **Download**, tag `latest` (or a version).
2. In File Station create `docker/iot-platform-go/data` and `docker/iot-platform-go/etc`.
3. **Image** → select it → **Run**:
   - container name `iot-platform-go`, enable **auto-restart**;
   - **Port settings**: local `9083` → container `8080`, `1883` → `1883`, `8083` → `8083`, `8883` → `8883` (TCP);
   - **Volume settings**: `docker/iot-platform-go/data` → `/opt/iot-platform/lib`, `docker/iot-platform-go/etc` → `/opt/iot-platform/etc`;
   - **Environment**: `MQTT_PUBLIC_PORT`, `MQTT_WS_PUBLIC_PORT`, `MQTT_TLS_PUBLIC_PORT` = the local ports chosen above (only needed when they differ from the container ports).
4. Open `http://<nas-ip>:9083/setup`.

### Updating a Docker Hub deployment

Data and `config.yml` are on the host folders, so the container can be recreated at any time:

```bash
docker pull darvik80/iot-platform:latest
docker rm -f iot-platform-go
docker run …                         # the same command as above
# compose: docker compose pull && docker compose up -d
```

In Container Manager: **Registry** → download the tag again, then stop the container and recreate it from the new image with the same settings (**Action → Reset** on DSM 7.2+, otherwise delete and run again).

To roll back, run the previous `X.Y.Z` tag with the same folders. Database migrations run on start and are forward-only — back up `data/platform.db` and `etc/config.yml` before upgrading across versions.

---

## Docker Compose

From the repository root:

```bash
cp .env.example .env                   # ports, host folders, image name
cp platform.env.example platform.env   # optional: pinned platform settings
docker compose up -d                   # prebuilt image from Docker Hub
docker compose up -d --build           # or: build from this checkout
docker compose --profile nodered up -d # + Node-RED
```

Without a `.env` the defaults below apply, so a bare `docker compose up -d` also works. On a host without the repository, `docker-compose.yml` alone (plus an optional `.env` / `platform.env`) is enough: with no local image Compose pulls `IOT_IMAGE`.

`.env` is used for `${…}` substitution only — none of its variables reach the container. The root `.env.example` also lists bare-metal platform variables (`HTTP_ADDR`, `DATABASE_DSN=./data/…`, …); Compose ignores them.

| Variable | Default | Meaning |
|---|---|---|
| `IOT_IMAGE` | `darvik80/iot-platform:latest` | Image to run (amd64 + arm64); `--build` tags the local build with this name |
| `IOT_VERSION` | `dev` | `VERSION` build arg for `--build` |
| `IOT_CONTAINER_NAME` | `iot-platform-go` | Container name |
| `IOT_HTTP_PORT` | `8080` | Host port → 8080 |
| `IOT_MQTT_PORT` | `1883` | Host port → 1883, also `MQTT_PUBLIC_PORT` |
| `IOT_MQTT_WS_PORT` | `8083` | Host port → 8083, also `MQTT_WS_PUBLIC_PORT` |
| `IOT_MQTT_TLS_PORT` | `8883` | Host port → 8883, also `MQTT_TLS_PUBLIC_PORT` |
| `IOT_DATA_DIR` | `./iot/data` | Host folder → `/opt/iot-platform/lib` |
| `IOT_ETC_DIR` | `./iot/etc` | Host folder → `/opt/iot-platform/etc` |
| `NODERED_PORT` | `1880` | Node-RED host port (profile `nodered`) |

Changes to `.env` apply on the next `docker compose up -d` (Compose recreates the container when ports, volumes or environment differ).

`platform.env` is passed with `env_file` and is optional. Every variable in it **overrides** `config.yml` from the setup wizard, so keep only what you want to pin (`PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`, `JWT_SECRET`, `DATABASE_*`, `MQTT_TLS_*`, …) and never leave a variable with an empty value — an empty variable still counts as set and hides the `config.yml` value.

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

1. Prints the effective settings (handy to check that variables were picked up).
2. Builds `localhost/<container>:latest` with `--pull`, `VERSION` (from `version.txt`) and `COMMIT` (short SHA).
3. Recreates the container with `--restart unless-stopped`, publishing all four ports and mounting `<IOT_HOST_DIR>/data` → `/opt/iot-platform/lib`, `<IOT_HOST_DIR>/etc` → `/opt/iot-platform/etc`. Docker creates missing host folders.
4. Waits until the container healthcheck reports `healthy` (up to 2 min, stops early on `unhealthy`); otherwise prints the last 100 log lines and fails.
5. Prunes dangling images.

Runs are serialized (`concurrency`), so two quick pushes never race on `docker rm` / `docker run`. The job uses `runs-on: ubuntu-latest` — that label must point to the runner on the Docker host itself.

**Runner requirement.** Recent Gitea runner versions give container jobs a Docker *proxy* instead of the daemon socket and remove everything created through it when the job ends. The job then succeeds, but the log ends with `removing container iot-platform-go the job left behind` and the container is gone (the image stays). Mount the real socket in the runner's `config.yaml` and restart the runner:

```yaml
container:
  options: -v /var/run/docker.sock:/var/run/docker.sock
  valid_volumes:
    - /var/run/docker.sock
```

This applies to every job on that runner: containers they start are no longer removed automatically, as with older runner versions.

Override anything with **repository variables** (Settings → Actions → Variables), no file edits needed. Variables are read when a run starts: after changing one, re-run the workflow (**Run workflow** / workflow_dispatch) to apply it. When you change `IOT_CONTAINER_NAME`, remove the old container by hand (`docker rm -f <old name>`) — otherwise it keeps the host ports and the new one fails to start.

| Variable | Default | Meaning |
|---|---|---|
| `IOT_CONTAINER_NAME` | `iot-platform-go` | Container and image name |
| `IOT_HTTP_PORT` | `9083` | Host port → 8080 |
| `IOT_MQTT_PORT` | `1883` | Host port → 1883 (+ `MQTT_PUBLIC_PORT`) |
| `IOT_MQTT_WS_PORT` | `8083` | Host port → 8083 (+ `MQTT_WS_PUBLIC_PORT`) |
| `IOT_MQTT_TLS_PORT` | `8883` | Host port → 8883 (+ `MQTT_TLS_PUBLIC_PORT`) |
| `IOT_HOST_DIR` | `/volume1/docker/iot-platform-go` | Contains `data/` and `etc/` |
| `PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`, `MQTT_WS_PUBLIC_URL`, `TIMEZONE` | — | Passed as `-e` only when set; otherwise `config.yml` applies |
| `DOCKER_API_VERSION` | `1.43` | Docker client API version (Synology's daemon rejects newer ones) |

Dashboard: `http://<nas-ip>:9083` · MQTT: `<nas-ip>:1883`.

### Docker Hub publishing

Deploying the published image: [From Docker Hub](#from-docker-hub).

`.gitea/workflows/deploy-docker-hub.yml` builds the same `Dockerfile` with buildx + QEMU on the `devops.local` runner:

| Trigger | Tags pushed |
|---|---|
| push to `master` | `latest` |
| push of tag `vX.Y.Z` | `X.Y.Z` (and `version.txt` value if it differs) |
| workflow_dispatch on another branch | none — build only |

Secrets: `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN` (Hub access token). Variables: `DOCKERHUB_IMAGE` (default `darvik80/iot-platform`), `DOCKER_PLATFORMS` (default `linux/amd64,linux/arm64`; arm64 needs a runner allowed to run privileged QEMU/binfmt — set `linux/amd64` if it is not).

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
docker compose pull && docker compose up -d      # compose, Docker Hub image
docker compose up -d --build                     # compose, local build
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

Service `nodered` in `docker-compose.yml`, started with `--profile nodered`:

- image `nodered/node-red:4.0`, container `cogitor-nodered`, port `${NODERED_PORT:-1880}:1880`
- `extra_hosts: host.docker.internal:host-gateway`
- env `NODERED_CREDENTIAL_SECRET` (default `cogitor-nodered-dev`), `MQTT_BROKER_HOST` (default `platform` — the compose service; for a platform outside compose use `host.docker.internal`, Podman `host.containers.internal`, or its LAN IP), `ACCOUNT_SLUG`
- bind mounts `./deploy/nodered/data:/data`, `./deploy/nodered/settings.js`, `./deploy/nodered/flows.json`, `./deploy/nodered/entrypoint.sh`

`MQTT_BROKER_HOST` is baked into `deploy/nodered/data/flows.json` on the first start only; to change it later, edit the broker node in Node-RED or delete that file. The flow template connects to port 1883 of `MQTT_BROKER_HOST`; with `MQTT_BROKER_HOST=platform` that is the container port, regardless of `IOT_MQTT_PORT`. Node-RED has no dedicated MQTT credentials — register it as an **external plugin** in the dashboard and use the returned `extplg_…` user/password. Automation setup — [06-automation.md](06-automation.md).

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
