# Cogitor IoT Platform

**Self-hosted IoT backend and operations dashboard for ESP32-class devices.**

[Русская версия](README.ru.md)

One Go binary. Embedded MQTT broker. SQLite or PostgreSQL. React dashboard. Multi-account. Zero cloud dependencies.

![Architecture](docs/images/architecture.svg)

---

## Table of Contents

- [What Is This?](#what-is-this)
- [Features](#features)
- [Quick Start](#quick-start)
- [Dashboard Overview](#dashboard-overview)
- [Device Onboarding](#device-onboarding)
- [Telemetry & Attributes](#telemetry--attributes)
- [Remote Procedure Calls (RPC)](#remote-procedure-calls-rpc)
- [OTA Firmware Updates](#ota-firmware-updates)
- [Automation & Plugins](#automation--plugins)
- [MCP — AI / LLM Integration](#mcp--ai--llm-integration)
- [Authentication & Users](#authentication--users)
- [Configuration Reference](#configuration-reference)
- [Deployment](#deployment)
- [MQTT Protocol](#mqtt-protocol)
- [Documentation](#documentation)
- [License](#license)

---

## What Is This?

Cogitor IoT Platform is a **single-process IoT backend** that runs on your NAS, Raspberry Pi, or any Linux/Windows machine. It replaces the typical multi-component stack (MQTT broker + backend + database + dashboard) with one binary you can build and run in minutes.

| Layer | Stack |
|-------|--------|
| API / ingest | Go 1.26+, chi router, SQLite / PostgreSQL |
| Broker | Embedded MQTT ([mochi-mqtt](https://github.com/mochi-mqtt/server)) inside the platform process |
| Automation | In-process plugins + external plugins / Node-RED over the MQTT rules bus (`platform/v1/...`) |
| UI | React 19 + Bootstrap 5 (Vite), embedded in Go binary |
| MCP | Embedded at `/mcp` (JWT) — LLM device tools |

**No cloud. No SaaS. No per-device fees.** Your data stays on your hardware.

---

## Features

- **Device lifecycle** — create products, register devices, one-time access tokens, token rotation
- **Telemetry ingestion** — store and query device metrics with automatic retention cleanup
- **Desired attributes** — push configuration to devices via MQTT downlink
- **RPC console** — call device methods (reboot, custom) with auto-populated forms from live device catalog
- **OTA updates** — upload `.bin` firmware, host download URLs, auto-push on reconnect (Auto-OTA)
- **Embedded MQTT broker** — no external broker needed; TCP, WebSocket and TLS (optional mTLS) listeners
- **Real-time dashboard** — WebSocket live updates for device status, telemetry, MQTT sessions
- **Automation** — rules bus for external plugins / Node-RED; in-process plugins (LED garland, schedule, Prometheus exporter, metrics dashboard)
- **MCP endpoint** — AI clients (Cursor, Claude, etc.) can list devices, read telemetry, invoke RPC
- **Multi-account** — accounts (tenants), memberships, invitations, per-account plugin policies, audit log
- **Auth & roles** — JWT login, Google OAuth, self-registration, admin / operator / viewer roles per account
- **API tokens** — long-lived tokens for scripts and MCP integration
- **Setup wizard** — first start on an empty database opens a web wizard (DB, admin, account, SMTP, JWT, MQTT TLS, public URL)

---

## Quick Start

### Prerequisites

- Go 1.26+ (for bare-metal) or Docker (for containerized deploy)
- Node.js 22+ (only needed to build the frontend; pre-built in Docker)

### 1. Build & Run (bare metal)

```bash
# Clone the repository
git clone git@gitea.darvik.synology.me:darvik/iot-platform-go.git
cd iot-platform-go

# Build dashboard + API in one step
make build

# Run
./bin/platform.exe
```

Or step by step:

```bash
# Build frontend only
make web

# Build Go binary
go build -o bin/platform.exe ./cmd/platform

# Run
./bin/platform.exe
```

### 2. Run with Docker

```bash
# Compose: ports and host folders come from deploy/.env (see docs/en/13-deploy-docker.md)
cd deploy && cp .env.example .env && docker compose up -d --build
# + Node-RED: docker compose --profile nodered up -d

# Or plain docker run: -p HOST:CONTAINER, data → /opt/iot-platform/lib, config → /opt/iot-platform/etc
docker build -t cogitor-iot .
docker run -d --name cogitor-iot -p 8080:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v "$PWD/iot/data:/opt/iot-platform/lib" -v "$PWD/iot/etc:/opt/iot-platform/etc" cogitor-iot
```

### 3. First Start — Setup Wizard

On an empty database the platform starts in **setup mode**: open http://localhost:8080 and the wizard walks you through database (SQLite / PostgreSQL), first admin, first account, SMTP, JWT secret, MQTT TLS and public URL. The result is written to `config.yml` and the platform continues starting — there is no default `admin/admin` login.

> **Security**: setup endpoints are unauthenticated. Run the wizard on localhost or a trusted network and restart the process after it finishes.

| Service | URL |
|---------|-----|
| Dashboard / API | http://localhost:8080 |
| MQTT broker | `mqtt://localhost:1883` |
| MQTT WebSocket | `ws://localhost:8083/` (subprotocol `mqtt`) |
| MQTT TLS | `mqtts://localhost:8883` (when cert + key are configured) |
| Node-RED (optional) | http://localhost:1880 |

> **Tip**: For development with hot-reload UI, run `cd web && npm run dev` in a separate terminal (API must be running on `:8080`).

---

## Dashboard Overview

![Dashboard Overview](docs/images/dashboard-overview.png)

The dashboard is a React SPA embedded into the Go binary. It provides a dark-themed, responsive interface for managing your entire IoT deployment.

### Navigation

| Page | What it shows |
|------|---------------|
| **Overview** | Online/offline/total device counts, MQTT status, recent events, active sessions |
| **MQTT** | Live broker sessions, connected clients (device / Node-RED / service), real-time broker event trail |
| **Products** | Device archetypes — create products, define RPC methods, telemetry metrics, attributes |
| **Devices** | All registered devices with status, firmware version, last seen. Search and filter. |
| **Device Detail** | Connection info, telemetry metrics, desired attributes, OTA panel, plugins, RPC console, events |
| **Events** | Device events filtered by severity, device, time range |
| **Logs** | Device log messages from MQTT `v1/me/l` uplink |
| **OTA** | Firmware management — upload `.bin`, register URLs, notify devices, Auto-OTA toggles, job tracking |
| **Users** | (Admin) Account members and invitations, roles |
| **Account** | (Admin) Account name/slug, catalog plugin policies |
| **Audit Log** | (Admin) Who changed what in the account |
| **Docs** | Built-in quick reference |
| **Settings** | Platform info, API tokens, external plugins (incl. Node-RED), integrations |

![Devices](docs/images/dashboard-devices.png)

![Device Detail](docs/images/dashboard-device-detail.png)

### Real-Time Updates

The dashboard keeps a WebSocket connection (`GET /api/v1/ws?token=<jwt>`). When a device connects, sends telemetry, or changes status, pages refresh automatically — no manual reload needed. Behind a reverse proxy, enable WebSocket upgrade for `/api/v1/ws`.

---

## Device Onboarding

![Device Onboarding Flow](docs/images/device-onboarding.svg)

### Step 1 — Create a Product

A **product** defines the device archetype: what RPC methods it supports, what telemetry metrics it sends, and what attributes it has.

1. Go to **Products** → enter a `product_id` (e.g. `esp32-cam`) and display name
2. Click **Create**
3. Open the product to define:
   - **RPC methods** — `reboot`, custom methods with typed parameters
   - **Metrics** — telemetry keys with labels, units, format (bytes, percent, rssi, etc.)
   - **Attributes** — reported / desired / both, with type and writability

New products get **ESP defaults** automatically (system metrics, `reboot`, `rpc-list`). Use **Reset to ESP presets** to restore them.

### Step 2 — Register a Device

1. Go to **Devices** → click **Add device**
2. Select the product, enter a `device_id` (e.g. `cam-001`) and optional display name
3. Click **Create**
4. **Copy the access token** from the credentials modal — it is shown **only once**

The credentials modal shows:
- `product_id`, `device_id`, `access_token`
- `username` / `client_id` (format: `{product_id}.{device_id}`)
- `broker_host` / `broker_port`
- Arduino snippet: `CogitorCredentials::createAccessToken(...)`
- **Copy JSON** / download `{product}-{device}.json` for firmware provisioning

### Step 3 — Flash Firmware

Embed the credentials in your ESP32 firmware:

```cpp
CogitorCredentials::createAccessToken("esp32-cam", "cam-001", "192.168.1.22", "<token>");
```

MQTT CONNECT parameters:

```
client_id = {product_id}.{device_id}
username  = {product_id}.{device_id}
password  = <access_token>
```

### Step 4 — Device Online

Once the device connects:
- It appears as **online** in the dashboard (green badge)
- Telemetry starts flowing to the device detail page
- Auto-OTA pushes the latest firmware if versions differ (when enabled)
- The platform auto-detects the topic style (full `iot/v1/...` vs short `v1/me/...`)

### Token Rotation

If a token is compromised:
1. Open the device detail page
2. Click **Rotate token**
3. The old MQTT session is disconnected immediately
4. A new token is shown once — update the device firmware

---

## Telemetry & Attributes

### Telemetry

Devices publish telemetry on `v1/me/t` (short) or `iot/v1/.../up/telemetry/data` (full). The platform:
- Parses the JSON payload
- Stores it in the database with timestamps
- Broadcasts it via WebSocket to the dashboard
- Publishes it to the rules bus for external plugins / Node-RED

**Query telemetry** via REST:

```
GET /api/v1/devices/{id}/telemetry?keys=temperature,humidity&from=...&to=...&limit=100
GET /api/v1/devices/{id}/telemetry/latest
```

**Retention**: telemetry older than `TELEMETRY_RETENTION_DAYS` (default 30) is auto-cleaned hourly.

### Attributes

Attributes represent device configuration state:

- **Reported** — device tells the platform its current state (e.g. `brightness: 80`)
- **Desired** — platform tells the device what state it should be in (e.g. `targetTemperature: 22`)

Set desired attributes via the device detail page form or REST:

```bash
PATCH /api/v1/devices/{id}/attributes/desired
{ "targetTemperature": 22, "mode": "auto" }
```

The platform persists the desired state and publishes it to the device via MQTT `v1/me/a/upd`.

---

## Remote Procedure Calls (RPC)

The RPC console on the device detail page lets you call any method the device supports.

### How It Works

1. Dashboard sends `POST /api/v1/devices/{id}/rpc` with method and params
2. Platform publishes MQTT downlink on `v1/me/r/req` (short) or `…/down/rpc/request` (full)
3. Device processes the request and responds on `v1/me/r/res`
4. Platform matches the response by correlation `id` and returns the result

### Built-in Methods

- **`rpc-list`** — returns the device's full method catalog with parameter types. The dashboard calls this automatically when the device is online to build dynamic forms.
- **`reboot`** — restart the device (configurable `delayMs`)

### Example

```json
POST /api/v1/devices/5/rpc
{
  "method": "reboot",
  "params": { "delayMs": 1000 },
  "timeout_ms": 30000
}
```

The RPC console auto-populates parameter forms from the product schema and live `rpc-list` response.

---

## OTA Firmware Updates

The OTA page manages firmware images and deployment.

### Upload Firmware

1. Go to **OTA** page
2. Select the product, enter version (e.g. `1.2.0`), optionally a module name (default: `main`)
3. Select a `.bin` file (max 32 MB)
4. Click **Upload**

The platform stores the image and generates a public download URL:

```
{PUBLIC_BASE_URL}/ota/fw/{token}.bin
```

> **Important**: `PUBLIC_BASE_URL` must be reachable from the device (use LAN IP, not `localhost`).

### Notify Devices

- **Notify product** — sends OTA update to all devices of that product
- **Notify single device** — select a device from the dropdown and click **Notify**

The device receives the download URL via MQTT `v1/me/o/upd` and begins downloading.

### Auto-OTA

Enable **Auto-OTA** per product (toggle on the OTA page). When enabled:
- Device comes online → platform checks firmware version → pushes update if outdated
- Device reports `ota/version` → same check
- Device was offline during notification → pending job is delivered on reconnect

Individual devices can opt out of Auto-OTA via the device detail page toggle.

### OTA Jobs

Track update progress on the OTA page:

| Status | Meaning |
|--------|---------|
| `pending` | Notification sent, device hasn't started yet |
| `running` | Device is downloading / flashing |
| `done` | Update completed |
| `failed` | Update failed (check device logs) |
| `cancelled` | Operator cancelled |

---

## Automation & Plugins

### Rules Bus (Node-RED)

The platform publishes normalized JSON events to the MQTT rules bus:

```
platform/v1/events/{account_slug}/{product_id}/{device_id}/telemetry
platform/v1/events/{account_slug}/{product_id}/{device_id}/status
platform/v1/events/{account_slug}/{product_id}/{device_id}/event
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/reported
platform/v1/events/{account_slug}/{product_id}/{device_id}/rpc/result
platform/v1/events/{account_slug}/{product_id}/{device_id}/ota
platform/v1/events/{account_slug}/{product_id}/{device_id}/log
```

External plugins (including Node-RED) subscribe to their own account's events (`platform/v1/events/{account_slug}/#`) and can publish commands back:

```
platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc
platform/v1/commands/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{account_slug}/{product_id}/{device_id}/ota
```

Node-RED has no dedicated credentials: register it in **Settings → External plugins** and use the issued `extplg_{account_slug}_{id}` MQTT user/password in the Node-RED MQTT broker node. See [06-automation](docs/en/06-automation.md).

> Commands **must** include the plugin's own `{account_slug}`: the broker ACL rejects other slugs, and the target device is looked up inside that account only. The old slug-less form `commands/{product_id}/{device_id}/…` is no longer accepted.

### Device Plugins

Plugins are **in-process** per-device automations (not Node-RED). They live in the platform process and use RPC to control devices.

#### LED Garland

Control WS2812 LED strips connected to ESP32:
- Effects: `off`, `white`, `warm`, `red`, `green`, `blue`, `blink`, `strobe`, `breathe`, `chase`, `comet`, `scanner`, `fill`, `theater`, `sparkle`, `fire`, `rainbow`, `police`, `christmas`
- **Quiet hours** — force `off` or dim effect during night (e.g. 23:00–07:00)
- **Active window** — strip is off outside the configured time range
- Multiple strips per device (GPIO + LED count)

#### Schedule

Fire device RPC at configurable local times:
- Set time and weekdays (e.g. reboot at 04:00 on weekdays)
- Timezone-aware (uses platform `TIMEZONE` or per-binding override)

#### Prometheus Exporter & Metrics Dashboard

- **prometheus-exporter** — starts a separate HTTP listener (default `:9100`, optional bearer token) with device / telemetry / RPC counters labelled by account
- **metrics-dashboard** — per-device dashboard with telemetry charts, gauges, RPC history and online timeline

Catalog plugins can be enabled or disabled per account (**Account** page, `/api/v1/plugins/policies`).

#### Adding Plugins

Plugins appear on the device detail page under **Plugins**. Click **Add plugin**, select a catalog type, configure, and save.

### External Plugins

External plugins run as **separate processes** (any language) and talk to the platform over **MQTT** (the rules bus): they subscribe to their account's events and publish commands. Webhook / polling / gRPC transports described in [08-external-plugins](docs/en/08-external-plugins.md) are planned, not implemented.

Create an external plugin in **Settings → External Plugins**. The platform issues MQTT credentials (`extplg_{account_slug}_{id}` + password, shown once). Write your plugin in Python, Go, Java, C++ — anything with an MQTT client. See [examples/plugins/](examples/plugins/).

---

## MCP — AI / LLM Integration

The platform exposes an **MCP (Model Context Protocol)** endpoint at `/mcp`. Any MCP-compatible AI client (Cursor, Claude, etc.) can interact with your devices.

### Setup

1. Go to **Settings** → create an API token (e.g. name: `mcp`)
2. Copy the `cip_…` token (shown once)
3. Configure your AI client:

```json
{
  "mcpServers": {
    "cogitor-iot": {
      "url": "http://192.168.1.22:8080/mcp",
      "headers": {
        "Authorization": "Bearer cip_…"
      }
    }
  }
}
```

### Available Tools

| Tool | Purpose |
|------|---------|
| `overview` | Online/offline counts, MQTT status |
| `list_devices` | Filter by product / status / query |
| `get_device` | Full detail (telemetry, attributes, schema) |
| `list_products` / `get_product` | Product schemas |
| `list_device_methods` | RPC method catalog |
| `invoke_rpc` | Call any device method |
| `reboot_device` | Shortcut for reboot RPC |
| `get_telemetry` | Latest telemetry samples |
| `get_attributes` | Reported + desired attributes |
| `set_desired_attributes` | Push desired attributes |

Write operations (`invoke_rpc`, `reboot_device`, `set_desired_attributes`) require **operator** or **admin** role.

> MCP tools are scoped to the token's active account (membership and account status are checked on every request); write tools require the `operator` or `admin` role in that account.

---

## Authentication & Users

### Login Methods

- **Password** — login by username or email + password
- **Google OAuth** — sign in / sign up with Google (if configured)
- **Self-registration** — creates a user **and a personal account** where the user is admin (enabled by default; set `REGISTER_ENABLED=false` for private installs)

### Roles

Roles are assigned **per account** (membership); one user can be admin in one account and viewer in another.

| Role | Read | Write (devices, RPC, OTA) | Admin (members, invitations, policies) |
|------|------|---------------------------|------------------------|
| `admin` | yes | yes | yes |
| `operator` | yes | yes | no |
| `viewer` | yes | no | no |

### API Tokens

Long-lived tokens for scripts and MCP:
- Create in **Settings** → name the token → copy `cip_…` (shown once)
- Use as `Authorization: Bearer cip_…`
- Revoke anytime from the same page

### Google OAuth Setup

1. Create OAuth client in Google Cloud Console (Web application)
2. Authorized redirect URI: `{PUBLIC_BASE_URL}/api/v1/auth/google/callback`
3. Set in the environment or `config.yml` (`google.client_id`, …):

```env
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
PUBLIC_BASE_URL=http://192.168.1.22:8080
```

---

## Configuration Reference

Configuration priority (first non-empty wins): **OS env → `./config.yml` → `/opt/iot-platform/etc/config.yml`**. The setup wizard writes `config.yml` (secrets included — keep it `chmod 600`). The binary does **not** read `.env` files; `.env` is only used by Docker Compose / systemd `EnvironmentFile`.

### Key Variables

| Env | Default | Description |
|-----|---------|-------------|
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `MQTT_ADDR` | `:1883` | Embedded MQTT TCP listen |
| `MQTT_WS_ADDR` | `:8083` | MQTT over WebSocket (`ws://host:port/`, subprotocol `mqtt`). `off` to disable |
| `MQTT_TLS_ADDR` | `:8883` | MQTT over TLS listen (requires cert + key) |
| `MQTT_TLS_CERT` | | Path to TLS certificate (enables TLS when set with key) |
| `MQTT_TLS_KEY` | | Path to TLS private key |
| `MQTT_TLS_CA` | | Path to CA cert (enables mutual TLS when set) |
| `MQTT_PUBLIC_PORT` / `MQTT_WS_PUBLIC_PORT` / `MQTT_TLS_PUBLIC_PORT` | listen port | Port announced to devices when the host maps a different one (Docker `-p 11883:1883`) |
| `SQLITE_PATH` | `/opt/iot-platform/lib/platform.db` | SQLite database file (legacy, use DATABASE_DSN) |
| `DATABASE_DRIVER` | `sqlite` | Database driver: `sqlite` or `postgres` |
| `DATABASE_DSN` | (auto) | Database DSN (e.g. `postgres://user:pass@host/db?sslmode=disable`) |
| `DB_MAX_OPEN_CONNS` | `8` | Parsed but currently not applied (SQLite uses 1, PostgreSQL 8) |
| `DATA_DIR` | (auto) | Data directory (defaults to DB file dir for SQLite, `./data` for Postgres) |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Public URL for OTA downloads, email links, OAuth. **Must be device-reachable** |
| `PLATFORM_NAME` | `Cogitor IoT` | Platform display name |
| `TIMEZONE` | `UTC` | IANA timezone for schedules and quiet hours |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `JWT_SECRET` | (dev default) | JWT signing key — **change in production** |
| `ADMIN_LOGIN` / `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin` / `admin@cogitor.local` / `admin` | Used only by the setup wizard; an existing DB is never re-seeded |
| `TELEMETRY_RETENTION_DAYS` | `30` | Telemetry cleanup period |
| `RPC_DEFAULT_TIMEOUT_MS` | `30000` | RPC response wait timeout |
| `BROKER_HOST_PUBLIC` | `localhost` | MQTT host shown in device credentials |
| `REGISTER_ENABLED` | `true` | Allow self-registration (disable on private installs) |
| `REGISTER_DEFAULT_ROLE` | `admin` | Role in the registrant's own personal account |
| `REQUIRE_EMAIL_VERIFY` | `false` | Require email verification before login |
| `RULES_BUS_ENABLED` | `true` | Enable the event/command bus for external plugins and Node-RED |
| `RULES_BUS_PREFIX` | `platform/v1` | Rules bus MQTT topic prefix |
| `LAB_ALLOW_UNKNOWN_DEVICES` | `false` | Reserved, currently not used |
| `BOOTSTRAP_ACCOUNT_SLUG` / `BOOTSTRAP_ACCOUNT_NAME` | `default` / `Default` | First account created by the setup wizard |

### Email Configuration

| Env | Default | Description |
|-----|---------|-------------|
| `EMAIL_MODE` | `log` | `log` prints verification links to stdout; `smtp` sends real email |
| `SMTP_HOST` | | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP port |
| `SMTP_USER` | | SMTP authentication user |
| `SMTP_PASS` | | SMTP authentication password |
| `SMTP_FROM` | `noreply@cogitor.local` | Sender address |

### Example environment (systemd `EnvironmentFile` / compose `.env`)

```env
HTTP_ADDR=:8080
PUBLIC_BASE_URL=http://192.168.1.22:8080
MQTT_ADDR=:1883
MQTT_WS_ADDR=:8083
BROKER_HOST_PUBLIC=192.168.1.22
TIMEZONE=Asia/Bangkok
DATABASE_DRIVER=sqlite
DATABASE_DSN=/opt/iot-platform/lib/platform.db
JWT_SECRET=<openssl rand -hex 32>
REGISTER_ENABLED=false
```

If `DATABASE_DSN` is empty and no `config.yml` exists, the platform starts the setup wizard instead.

---

## Deployment

### Docker

The platform image is built from the root `Dockerfile` (see [Run with Docker](#2-run-with-docker)). Image volumes: `/opt/iot-platform/lib` (SQLite `platform.db` + firmware) and `/opt/iot-platform/etc` (`config.yml` from the setup wizard + certs); container ports `8080 1883 8083 8883`.

`deploy/docker-compose.yml` runs the platform (host ports and folders from `deploy/.env`, optional settings from `deploy/platform.env`) and, with `--profile nodered`, Node-RED on `:1880`:

```bash
cd deploy
cp .env.example .env
docker compose up -d --build
```

Details, Gitea CI deploy and migration from the old `/data` layout — [docs/en/13-deploy-docker.md](docs/en/13-deploy-docker.md).

### Bare Metal (development)

```bash
make build          # builds web UI + Go binary
./bin/platform.exe  # run
```

### Cogitor VPS (`iot.cogitor.dev`)

Production target is a **systemd** service (no Docker on the VPS). Infra: ansible in `devops/cogitor` (`iot.yml`). CI: [docs/en/12-deploy-vds.md](docs/en/12-deploy-vds.md) / `.gitea/workflows/deploy-vds.yml`.

```bash
# on VPS (once)
ansible-playbook -i inventory/hosts.yml iot.yml

# from this repo (or Gitea on push)
make build-linux
# scp + systemctl restart iot-platform
```

Dashboard: `https://iot.cogitor.dev` · MQTT: `iot.cogitor.dev:1883` · MQTT WS: `wss://iot.cogitor.dev/mqtt`

### Docker (Synology / compose)

See [docs/en/13-deploy-docker.md](docs/en/13-deploy-docker.md) / `.gitea/workflows/deploy-docker.yml`.

### Health Checks

| Endpoint | Purpose |
|----------|---------|
| `GET /healthz` | Liveness probe (no auth) |
| `GET /readyz` | DB + MQTT ready (no auth); includes `mqtt_clients` count |
| `GET /metrics` | Platform runtime metrics (currently **unauthenticated** — restrict at the reverse proxy) |

---

## MQTT Protocol

The platform process **is** the MQTT broker. Devices and Node-RED connect to `MQTT_ADDR` (default `:1883`), MQTT-over-WebSocket on `MQTT_WS_ADDR` (default `:8083`) or MQTT over TLS on `MQTT_TLS_ADDR` (default `:8883`, see [05-mqtt-tls](docs/en/05-mqtt-tls.md)).

### Topic Styles

New devices default to `short`; the style is **auto-detected** from the device's first packet:

| Prefix | Style |
|--------|-------|
| `iot/v1/{product}/{device}/…` | Full |
| `v1/me/…` or `v1/me/t`, `v1/me/r/req`, … | Short |

### Key Topics (Short and Full Style)

Full-style topics are prefixed with `iot/v1/{product}/{device}/` (shown below as `…/`). Short uplink topics are accepted both as `v1/me/…` and `v1/…`; the platform always sends short downlinks to `v1/me/…`.

| Purpose | Dir | Short | Full |
|---------|-----|-------|------|
| Status (LWT — online/offline) | up | `v1/me/s` | `…/up/status` |
| Telemetry data | up | `v1/me/t` | `…/up/telemetry/data` |
| Device events | up | `v1/me/e` | `…/up/events/post` |
| Attributes report | up | `v1/me/a` | `…/up/attributes/report` |
| Request desired attributes | up | `v1/me/a/req` | `…/up/attributes/request` |
| Desired attributes response | down | `v1/me/a/res` | `…/down/attributes/response` |
| Desired attributes update | down | `v1/me/a/upd` | `…/down/attributes/update` |
| RPC request (platform → device) | down | `v1/me/r/req` | `…/down/rpc/request` |
| RPC response (device → platform) | up | `v1/me/r/res` | `…/up/rpc/response` |
| RPC request (device → platform) | up | `v1/me/r/creq` | `…/up/rpc/request` |
| RPC response (platform → device) | down | `v1/me/r/cres` | `…/down/rpc/response` |
| NTP time request | up | `v1/me/n/req` | `…/up/ntp/request` |
| NTP time response | down | `v1/me/n/res` | `…/down/ntp/response` |
| OTA version report | up | `v1/me/o/ver` | `…/up/ota/version` |
| OTA query (pull) | up | `v1/me/o/q` | `…/up/ota/query` |
| OTA update notification (URL) | down | `v1/me/o/upd` | `…/down/ota/update` |
| OTA cancel | down | `v1/me/o/can` | `…/down/ota/cancel` |
| OTA progress | up | `v1/me/o/p` | `…/up/ota/progress` |
| Device log messages | up | `v1/me/l` | `…/up/logs/report` |

### Device Authentication

1. Dashboard creates device → generates access token → SHA-256 stored in the database
2. MQTT CONNECT: `username={product}.{device}`, `password=token`
3. ACL allows the device its own `iot/v1/{product}/{device}/…` topics and the `v1/…` short-style tree
4. Token rotation drops the live session; device must reconnect with new token

> Short-style downlink topics (`v1/me/…`) look the same for every device, but the platform delivers each message directly to the target device's session only. Device-to-device publishes are not relayed: the broker hands device messages to the platform and nobody else.

### Monitoring

The **MQTT** page shows:
- Broker status (listening / down)
- Connected clients (kind, username, remote IP, subscriptions)
- Real-time broker event trail (auth, connect/disconnect, ACL deny, ingest, publish)

![MQTT page](docs/images/dashboard-mqtt.png)

---

## Documentation

See [docs/README_en.md](docs/README_en.md) for the full index.

| Document | EN | RU |
|----------|----|----|
| Product Requirements (PRD) | [01-prd.md](docs/en/01-prd.md) | [01-prd.md](docs/ru/01-prd.md) |
| REST API | [02-api.md](docs/en/02-api.md) | [02-api.md](docs/ru/02-api.md) |
| Auth & Users | [03-auth.md](docs/en/03-auth.md) | [03-auth.md](docs/ru/03-auth.md) |
| MQTT Protocol | [04-mqtt.md](docs/en/04-mqtt.md) | [04-mqtt.md](docs/ru/04-mqtt.md) |
| MQTT over TLS | [05-mqtt-tls.md](docs/en/05-mqtt-tls.md) | [05-mqtt-tls.md](docs/ru/05-mqtt-tls.md) |
| Automation / Node-RED | [06-automation.md](docs/en/06-automation.md) | [06-automation.md](docs/ru/06-automation.md) |
| Internal Plugins | [07-internal-plugins.md](docs/en/07-internal-plugins.md) | [07-internal-plugins.md](docs/ru/07-internal-plugins.md) |
| External Plugins | [08-external-plugins.md](docs/en/08-external-plugins.md) | [08-external-plugins.md](docs/ru/08-external-plugins.md) |
| Product Schema | [09-product-schema.md](docs/en/09-product-schema.md) | [09-product-schema.md](docs/ru/09-product-schema.md) |
| MCP (LLM Tools) | [10-mcp.md](docs/en/10-mcp.md) | [10-mcp.md](docs/ru/10-mcp.md) |
| Operations | [11-operations.md](docs/en/11-operations.md) | [11-operations.md](docs/ru/11-operations.md) |
| Deploy VDS | [12-deploy-vds.md](docs/en/12-deploy-vds.md) | [12-deploy-vds.md](docs/ru/12-deploy-vds.md) |
| Deploy Docker | [13-deploy-docker.md](docs/en/13-deploy-docker.md) | [13-deploy-docker.md](docs/ru/13-deploy-docker.md) |
| Diagrams | [14-diagrams.md](docs/en/14-diagrams.md) | [14-diagrams.md](docs/ru/14-diagrams.md) |
| Multi-Account & Monitoring (single v1.x) | [16-multi-account.md](docs/en/16-multi-account.md) | [16-multi-account.md](docs/ru/16-multi-account.md) |
| HA / Clustering (v2.x) | [17-ha-architecture.md](docs/en/17-ha-architecture.md) | [17-ha-architecture.md](docs/ru/17-ha-architecture.md) |

---

## Acceptance Checklist

Verify your deployment is working:

- [ ] Device connects with token and appears **online** in dashboard
- [ ] Telemetry shows in device detail and updates via WebSocket
- [ ] Desired attribute patch reaches device (`v1/me/a/upd`)
- [ ] RPC round-trip with matching `id` and `code: 0`
- [ ] NTP request gets `serverRecvTime` / `serverSendTime`
- [ ] Token rotation drops old session; new token works
- [ ] Node-RED receives `platform/v1/events/{account}/.../telemetry`
- [ ] Auto-OTA pushes firmware when enabled and device reconnects

---

## License

Proprietary — Cogitor.
