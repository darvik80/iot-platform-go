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
```

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

```bash
sudo tee /opt/iot-platform-go/etc/.env << 'EOF'
# --- Required ---
PUBLIC_BASE_URL=https://iot.cogitor.dev
JWT_SECRET=<generate-random-64-chars>
BROKER_HOST_PUBLIC=iot.cogitor.dev

# --- Admin ---
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=<strong-password>

# --- Database ---
SQLITE_PATH=/opt/iot-platform-go/data/platform.db

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

---

## Deploying the binary

### Automatic (Gitea CI)

Workflow **`.gitea/workflows/deploy-vds.yml`** — push to `vds` branch or manual trigger:

1. Builds web UI (`npm ci && npm run build`)
2. Builds Go binary (`CGO_ENABLED=0 GOOS=linux`)
3. `scp` binary + systemd unit to server
4. `systemctl restart iot-platform`
5. Health check: `curl http://127.0.0.1:8080/healthz`

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

# MQTT
mosquitto_sub -h iot.cogitor.dev -t '#' -u 'product.device' -P '<token>' -v
```

---

## Updating the platform

On push to `vds` — CI automatically:
1. Builds new binary
2. Uploads to server
3. Restarts service (~2 seconds downtime)

SQLite database and data in `/opt/iot-platform-go/data/` — not affected during updates.

---

## Endpoints summary

| Service | URL |
|---------|-----|
| Dashboard / API | `https://iot.cogitor.dev` |
| WebSocket | `wss://iot.cogitor.dev/api/v1/ws` |
| MQTT TCP | `iot.cogitor.dev:1883` |
| MQTT WebSocket | `wss://iot.cogitor.dev/mqtt` |
| Healthcheck | `https://iot.cogitor.dev/healthz` |

Docker / Synology: [13-deploy-docker.md](13-deploy-docker.md).
