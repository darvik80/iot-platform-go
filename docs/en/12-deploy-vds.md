# Deploy to VDS (systemd, no Docker)

Production deployment on a virtual server. The platform is a **single Go binary** managed by systemd. No Docker, no extra processes.

**Result:** `iot.cogitor.dev` — dashboard over HTTPS, MQTT over TCP and WebSocket.

---

## Architecture

```
Internet
  │
  ├── https://iot.cogitor.dev        → nginx → localhost:8080 (HTTP)
  ├── wss://iot.cogitor.dev/mqtt     → nginx → localhost:8083 (WS)
  └── iot.cogitor.dev:1883           → MQTT TCP (direct)
                                            │
                                      ┌─────┴─────┐
                                      │  systemd   │
                                      │ iot-platform│
                                      │  (Go binary)│
                                      └─────┬─────┘
                                            │
                                      SQLite WAL
                                      /opt/iot-platform-go/data/
                                      (or PostgreSQL via DATABASE_DSN)
```

Two installation options are described below: a manual layout under `/opt/iot-platform-go` (used by the `deploy-vds.yml` CI workflow) and the [Debian package](#debian-package) (`make deb`) with its own layout.

---

## Prerequisites

| Component | Requirement |
|-----------|------------|
| VPS | Linux (Ubuntu 22+, Debian 12+), 512 MB RAM, 1 vCPU |
| DNS | `iot.cogitor.dev` → server IP |
| SSH | User with `sudo NOPASSWD` |
| nginx | For HTTPS and WSS TLS termination |
| Domain | For Let's Encrypt (or self-signed for internal use) |

---

## Server setup

### 1. User and directories

```bash
# On the server
sudo useradd -r -m -d /opt/iot-platform-go -s /usr/sbin/nologin iot
sudo mkdir -p /opt/iot-platform-go/{bin,data,etc}
sudo chown -R iot:iot /opt/iot-platform-go
```

### 2. Systemd unit

```ini
# /etc/systemd/system/iot-platform.service
[Unit]
Description=IoT Platform (Go)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=iot
Group=iot
WorkingDirectory=/opt/iot-platform-go
EnvironmentFile=/opt/iot-platform-go/etc/.env
ExecStart=/opt/iot-platform-go/bin/platform
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=iot-platform

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/iot-platform-go/data
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable iot-platform
```

### 3. Environment variables

The binary itself does not read `.env`; here the file is loaded by systemd via `EnvironmentFile=` in the unit above. Values set here are OS env and therefore override any `config.yml`.

```bash
sudo tee /opt/iot-platform-go/etc/.env << 'EOF'
# --- Required ---
PUBLIC_BASE_URL=https://iot.cogitor.dev
JWT_SECRET=<generate-random-64-chars>
BROKER_HOST_PUBLIC=iot.cogitor.dev

# --- Database (SQLite default; or DATABASE_DRIVER=postgres + postgres:// DSN) ---
DATABASE_DRIVER=sqlite
DATABASE_DSN=/opt/iot-platform-go/data/platform.db

# --- Network ---
HTTP_ADDR=:8080
MQTT_ADDR=:1883

# --- Optional ---
TIMEZONE=UTC
TELEMETRY_RETENTION_DAYS=30
LOG_LEVEL=info
EOF

sudo chown iot:iot /opt/iot-platform-go/etc/.env
sudo chmod 600 /opt/iot-platform-go/etc/.env
```

### 4. First start — setup wizard

There is no admin in env (`ADMIN_*` variables are ignored). On the first start the database is empty, so the platform serves the **setup wizard** on `:8080` (`/setup`, API `/api/v1/setup/*`) instead of the dashboard. It creates the admin user and the default account — see [Operations → setup wizard](11-operations.md#first-start-setup-wizard).

- The setup endpoints are unauthenticated — open the wizard **before** enabling the public nginx site, e.g. via `ssh -L 8080:127.0.0.1:8080 user@HOST` and `http://localhost:8080/setup`.
- In step 1 set the DSN to the same value as `DATABASE_DSN` in `.env` (`/opt/iot-platform-go/data/platform.db`); the wizard's default `/opt/iot-platform/lib/platform.db` is not writable under this unit's `ProtectSystem=strict`.
- With this unit only `/opt/iot-platform-go/data` is writable, so the wizard cannot persist `config.yml` (a `failed to write config.yml` warning is expected); the values in `.env` remain the source of truth.
- Restart the service after the wizard completes so the setup routes are unmounted.

---

## nginx (TLS termination)

```nginx
# /etc/nginx/sites-available/iot-platform
server {
    listen 443 ssl http2;
    server_name iot.cogitor.dev;

    ssl_certificate     /etc/letsencrypt/live/iot.cogitor.dev/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/iot.cogitor.dev/privkey.pem;

    # Dashboard + API
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket (dashboard real-time)
    location /api/v1/ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }

    # MQTT over WebSocket
    location /mqtt {
        proxy_pass http://127.0.0.1:8083;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }
}

server {
    listen 80;
    server_name iot.cogitor.dev;
    return 301 https://$host$request_uri;
}
```

```bash
sudo ln -s /etc/nginx/sites-available/iot-platform /etc/nginx/sites-enabled/
sudo certbot --nginx -d iot.cogitor.dev
sudo nginx -t && sudo systemctl reload nginx
```

**Firewall ports:**

| Port | Protocol | Purpose |
|------|----------|---------|
| 80 | TCP | HTTP → redirect to HTTPS |
| 443 | TCP | HTTPS (dashboard + API + WSS) |
| 1883 | TCP | MQTT TCP (for devices) |
| 8883 | TCP | MQTT over TLS (only if `MQTT_TLS_CERT` / `MQTT_TLS_KEY` are configured) |

---

## Deploying the binary

### Automatic (Gitea CI)

Workflow **`.gitea/workflows/deploy-vds.yml`** — push to `vds` branch or manual trigger:

1. Builds web UI (`npm ci && npm run build`, Node 22)
2. Builds Go binary (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`)
3. `scp` binary + `deploy/iot-platform.service` to server, runs `deploy/remote-deploy.sh`
4. Installs the unit if changed, `systemctl restart iot-platform`
5. Health check: `curl http://127.0.0.1:8080/healthz`

> **Known issue**: `deploy-vds.yml` (and `build-deb.yml`) pin `go-version: "1.25.x"`, while `go.mod` requires `go 1.26.0` and the `Dockerfile` uses `golang:1.26-bookworm`. Bump the workflows to `1.26.x` (or rely on Go toolchain auto-download) if the build fails on the Go version check.

**Gitea secrets:**

| Secret | Value |
|--------|-------|
| `IOT_DEPLOY_HOST` | Server IP or domain (`iot.cogitor.dev`) |
| `IOT_DEPLOY_USER` | SSH user with sudo NOPASSWD |
| `IOT_DEPLOY_SSH_KEY` | ed25519 private key (pubkey in `authorized_keys`) |

### Manual deploy

```bash
# Locally — build and upload
make build-linux
scp bin/platform darvik@HOST:/tmp/iot-platform.new

# On server — install and restart
ssh darvik@HOST '
  sudo install -o iot -g iot -m 755 /tmp/iot-platform.new /opt/iot-platform-go/bin/platform
  sudo systemctl restart iot-platform
  sleep 2
  systemctl is-active --quiet iot-platform && echo "OK" || echo "FAILED"
'
```

---

## Verification

```bash
# Service status
sudo systemctl status iot-platform

# Logs
sudo journalctl -u iot-platform -f

# Health endpoints
curl -fsS https://iot.cogitor.dev/healthz   # liveness
curl -fsS https://iot.cogitor.dev/readyz    # readiness (MQTT + DB)

# MQTT — subscribe with device credentials to the device's own downlink topics
# (wildcard '#' is rejected by the broker ACL)
mosquitto_sub -h iot.cogitor.dev -u 'product.device' -P '<token>' -v -t 'v1/me/r/req'
# publish a test telemetry message
mosquitto_pub -h iot.cogitor.dev -u 'product.device' -P '<token>' -t 'v1/t' -m '{"temp":21.5}'
```

---

## Updating the platform

On push to `vds` — CI automatically:
1. Builds new binary
2. Uploads to server
3. Restarts service (~2 seconds downtime)

SQLite database and data in `/opt/iot-platform-go/data/` — not affected during updates. With PostgreSQL the data lives in the database server; back it up with `pg_dump` (see [Operations → Database](11-operations.md#database)).

---

## Debian package

Alternative to the manual layout above: build a `.deb` with `make deb` (runs `deb/build.sh`, needs Linux or WSL with `dpkg-deb`, Node and Go). The CI workflow `.gitea/workflows/build-deb.yml` builds the same package on push to `master` / `v*` tags and attaches it to a Gitea Release on tags.

```bash
make deb                                   # → dist/iot-platform_<version>_amd64.deb (version from version.txt)
sudo dpkg -i dist/iot-platform_<version>_amd64.deb
```

| Item | Location |
|------|----------|
| Binary | `/opt/iot-platform/bin/platform` |
| System user / group | `iot-platform` (created by `preinst`, home `/opt/iot-platform`, no login shell) |
| Config | `/opt/iot-platform/etc/config.yml` (placeholder on fresh install; written by the setup wizard), optional `/opt/iot-platform/etc/.env`, certs in `/opt/iot-platform/etc/certs/` |
| Data | `/opt/iot-platform/lib` (default SQLite file `/opt/iot-platform/lib/platform.db`, firmware in `lib/firmware`) |
| Logs | `/var/log/iot-platform/platform.log` (rotated by `/etc/logrotate.d/iot-platform`) |
| systemd unit | `/lib/systemd/system/iot-platform.service` (source: `deb/iot-platform.service`) |

The unit runs as `iot-platform` with `WorkingDirectory=/opt/iot-platform`, `EnvironmentFile=/opt/iot-platform/etc/.env` (optional) and `ProtectSystem=strict` with `ReadWritePaths=/opt/iot-platform/etc /opt/iot-platform/lib /var/log/iot-platform`. `postinst` enables and (re)starts the service and prints the wizard URL `http://<host>:8080`. Because `/opt/iot-platform/etc` is writable, the wizard persists its result to `/opt/iot-platform/etc/config.yml`; upgrades keep that file. The same setup-wizard security caveats apply — finish it before exposing port 8080.

---

## Endpoints summary

| Service | URL |
|---------|-----|
| Dashboard / API | `https://iot.cogitor.dev` |
| WebSocket | `wss://iot.cogitor.dev/api/v1/ws` |
| MQTT TCP | `iot.cogitor.dev:1883` |
| MQTT TLS (optional) | `mqtts://iot.cogitor.dev:8883` |
| MQTT WebSocket | `wss://iot.cogitor.dev/mqtt` |
| Healthcheck | `https://iot.cogitor.dev/healthz` |

Docker / Synology: [13-deploy-docker.md](13-deploy-docker.md).
