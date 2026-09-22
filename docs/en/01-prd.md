# Cogitor IoT Platform — Product Requirements Document

> **Version**: 1.0  
> **Status**: Active  
> **Last updated**: August 2026

This document defines what the platform does, for whom, and why. It is the source of truth for feature scope and acceptance criteria.

Related docs: [API](02-api.md), [Auth](03-auth.md), [MQTT](04-mqtt.md), [Automation](06-automation.md), [Operations](11-operations.md).

---

## 1. Vision

Cogitor IoT Platform is a **self-hosted, single-binary IoT backend and operations dashboard** for managing ESP32-class devices over MQTT. It provides everything a small-to-medium deployment needs — device provisioning, telemetry ingestion, remote procedure calls, OTA firmware updates, automation, and an LLM integration surface — without depending on cloud services or third-party platforms.

The guiding principle: **one process, one binary, zero external SaaS**.

---

## 2. Problem Statement

| Pain point | Current alternatives | Cogitor approach |
|------------|---------------------|------------------|
| Cloud IoT platforms require internet, incur per-device costs, and lock in data | AWS IoT, ThingsBoard Cloud, Blynk | Self-hosted; all data stays on the operator's NAS / server |
| Multi-component stacks (broker + backend + DB + UI) are complex to deploy and maintain | ThingsBoard CE + Mosquitto + PostgreSQL + Grafana | Single Go binary with embedded MQTT broker, SQLite, and React dashboard |
| Firmware OTA is manual (SCP / TFTP / custom scripts) | Custom shell scripts, manual flashing | Built-in OTA: upload `.bin`, hosted download URL, auto-push on reconnect |
| Automation requires a separate server with its own state | Standalone Node-RED, Home Assistant | In-process per-device plugins (Go) + optional Node-RED for cross-device rules via MQTT rules bus |
| No LLM / AI integration for device ops | None in open-source IoT platforms | Embedded MCP endpoint — any MCP-compatible AI client can list devices, invoke RPC, read telemetry |

---

## 3. Target Users

### 3.1 Primary — Device Maker / Lab Operator
- Builds ESP32-based devices (LED controllers, sensors, actuators)
- Needs a dashboard to see device status, push firmware, and send commands
- Runs the platform on a home NAS or a Raspberry Pi
- Wants minimal ops overhead

### 3.2 Secondary — Small-Business IoT Deployer
- Manages 10–200 devices across a single site
- Needs role-based access (admin / operator / viewer)
- Requires automation rules (scheduled reboots, LED quiet hours, threshold alerts)
- Needs audit trail (events, logs, OTA history)

### 3.3 Tertiary — AI/LLM Enthusiast
- Wants to query device state and invoke commands via natural language
- Uses Cursor, ChatGPT, or another MCP-compatible client
- Expects a standards-based integration point (MCP over HTTP)

---

## 4. Goals & Success Metrics

| Goal | Metric | Target |
|------|--------|--------|
| Minimal deployment footprint | Docker containers / processes | 1 (platform) + 1 optional (Node-RED) |
| Fast device onboarding | Time from "Create device" to first telemetry | < 5 minutes (including firmware flash) |
| Reliable device presence | MQTT session tracking accuracy | 100% of connected sessions reflected in dashboard within 2 s |
| Zero-touch firmware updates | Devices on latest firmware after Auto-OTA enable | All online devices updated within one reconnect cycle |
| Operational visibility | Dashboard page load time | < 1 s for up to 200 devices |
| AI operability | MCP tools available | All core device operations accessible via MCP |

---

## 5. Functional Requirements

### 5.1 Device Lifecycle Management

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-101 | Create **products** as device archetypes with shared schema (RPC methods, telemetry metrics, attributes) | P0 | ✅ |
| F-102 | Register **devices** under a product; generate one-time access token shown once at creation | P0 | ✅ |
| F-103 | Rotate device token; old MQTT session is disconnected immediately | P0 | ✅ |
| F-104 | Enable / disable / delete devices; disabled devices cannot authenticate | P0 | ✅ |
| F-105 | Auto-detect MQTT topic style (full `iot/v1/…` vs short `v1/me/…`) from the first device packet | P0 | ✅ |
| F-106 | Device provisioning payload: Wi-Fi + Cogitor MQTT credentials as JSON for firmware | P1 | ✅ |

### 5.2 Telemetry & Attributes

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-201 | Ingest telemetry from MQTT uplink topics; store in SQLite with timestamps | P0 | ✅ |
| F-202 | Expose latest telemetry and historical range queries via REST | P0 | ✅ |
| F-203 | Support **reported** and **desired** attributes; desired attributes are published to the device on save | P0 | ✅ |
| F-204 | Telemetry retention: auto-cleanup of records older than configurable days (default 30) | P1 | ✅ |
| F-205 | Product-defined metric schema: key, label, group, unit, format (bytes / percent / rssi / etc.) | P1 | ✅ |

### 5.3 Remote Procedure Calls (RPC)

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-301 | Send RPC requests to devices via MQTT downlink; wait for response with configurable timeout | P0 | ✅ |
| F-302 | RPC console in dashboard: auto-populate methods from product schema + live `rpc-list` | P0 | ✅ |
| F-303 | RPC history stored per device | P1 | ✅ |
| F-304 | Correlation via JSON `id` field; success = `code: 0` | P0 | ✅ |

### 5.4 OTA Firmware Updates

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-401 | Upload `.bin` firmware images (multipart, max 32 MB); hosted at a public URL (no auth for ESP HTTP GET) | P0 | ✅ |
| F-402 | Register external firmware URLs (HTTPS) as an alternative to hosting | P1 | ✅ |
| F-403 | Notify a single device or an entire product about new firmware via MQTT | P0 | ✅ |
| F-404 | **Auto-OTA**: when enabled on a product, push latest firmware to devices on connect or version report | P0 | ✅ |
| F-405 | Track OTA job status: pending → running → success / failed / cancelled | P0 | ✅ |
| F-406 | Module support: same product can have firmware for different modules (main, factory, etc.) | P1 | ✅ |
| F-407 | Same product + module + version replaces the previous image (new download URL) | P1 | ✅ |

### 5.5 Authentication & Authorization

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-501 | Login by username or email + password | P0 | ✅ |
| F-502 | Self-registration with optional email verification (SMTP or log mode) | P1 | ✅ |
| F-503 | Google OAuth sign-in / sign-up with account linking | P1 | ✅ |
| F-504 | Role-based access: **admin** (full), **operator** (read + write devices/RPC/OTA), **viewer** (read-only) | P0 | ✅ |
| F-505 | Bootstrap admin account from environment variables | P0 | ✅ |
| F-506 | Long-lived API tokens for MCP and scripts (created in Settings, shown once) | P0 | ✅ |
| F-507 | Admin Users page: list, change role, disable, delete | P1 | ✅ |

### 5.6 Automation

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-601 | **Rules bus**: normalized JSON events on `platform/v1/events/{product}/{device}/…` via MQTT | P0 | ✅ |
| F-602 | **Commands**: Node-RED publishes to `platform/v1/commands/{product}/{device}/…` for RPC, desired attributes, OTA | P0 | ✅ |
| F-603 | **Device plugins**: in-process per-device automations (LED garland, schedule) with RPC callbacks | P0 | ✅ |
| F-604 | LED garland plugin: WS2812 strip control with effects, quiet hours, night-light windows | P1 | ✅ |
| F-605 | Schedule plugin: fire device RPC at configurable local times and weekdays | P1 | ✅ |
| F-606 | Node-RED automation page embedded as iframe in the dashboard | P1 | ✅ |
| F-607 | Plugin extensibility: Go builtin plugins via `Register()` + `init()` pattern | P2 | ✅ |

### 5.7 MQTT Broker

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-701 | Embedded MQTT broker (mochi-mqtt) in the platform process — no external broker required | P0 | ✅ |
| F-702 | Device ACL scoped to `{product}.{device}` identity (up/down topics only) | P0 | ✅ |
| F-703 | Optional MQTT-over-WebSocket listener | P2 | ✅ |
| F-704 | LWT-based presence detection (online/offline) | P0 | ✅ |
| F-705 | MQTT monitoring page: live sessions, broker event trail (auth, connect, ACL, ingest) | P0 | ✅ |
| F-706 | NTP sync: device sends time request, platform responds with `serverRecvTime` / `serverSendTime` | P1 | ✅ |

### 5.8 Dashboard (Web UI)

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-801 | React SPA (Bootstrap 5, Vite) embedded into Go binary at build time | P0 | ✅ |
| F-802 | Pages: Overview, MQTT, Products, Devices, Device Detail, Events, Logs, OTA, Automation, Settings, Users | P0 | ✅ |
| F-803 | Real-time updates via WebSocket (device status, telemetry, MQTT events, OTA progress) | P0 | ✅ |
| F-804 | Overview: online/offline/total counts, MQTT status, recent events, active sessions | P0 | ✅ |
| F-805 | Device detail: connection info, metrics, desired attributes form, OTA panel, plugins, RPC console, events | P0 | ✅ |
| F-806 | Mobile-responsive layout with off-canvas navigation | P1 | ✅ |

### 5.9 MCP (LLM Integration)

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-901 | Streamable HTTP MCP endpoint at `/mcp` (same process, JWT/API token auth) | P0 | ✅ |
| F-902 | Tools: overview, list/get devices, list/get products, invoke RPC, reboot, telemetry, attributes | P0 | ✅ |
| F-903 | Write tools (RPC, reboot, set attributes) require operator/admin role | P0 | ✅ |
| F-904 | Compatible with Cursor, Claude, and other MCP clients | P1 | ✅ |

### 5.10 Observability

| ID | Requirement | Priority | Status |
|----|-------------|----------|--------|
| F-1001 | Structured logs to stdout (`slog`): connect, disconnect, auth_fail, ACL deny, RPC, OTA | P0 | ✅ |
| F-1002 | Configurable log level: debug / info / warn / error | P0 | ✅ |
| F-1003 | Events page: filter by device, severity, time range | P0 | ✅ |
| F-1004 | Logs page: device log messages with level and time filtering | P1 | ✅ |
| F-1005 | Health endpoints: `/healthz` (liveness), `/readyz` (DB + MQTT ready) | P0 | ✅ |

---

## 6. Non-Functional Requirements

| ID | Category | Requirement |
|----|----------|-------------|
| NF-01 | **Deployment** | Single Docker image (multi-stage: Node.js → Go → Alpine). Runs as non-root user. |
| NF-02 | **Deployment** | Alternative: bare-metal via `make build` → single binary + SQLite file |
| NF-03 | **Portability** | Windows (dev), Linux (production), ARM64 compatible (SQLite pure-Go driver) |
| NF-04 | **Security** | JWT auth for dashboard/API; SHA-256 hashed device tokens; ACL-scoped MQTT |
| NF-05 | **Security** | No device tokens in Node-RED flows; rules bus uses separate credentials |
| NF-06 | **Security** | OTA firmware download is unauthenticated (ESP HTTP GET) — URL is token-based |
| NF-07 | **Performance** | SQLite WAL mode for concurrent reads during ingest |
| NF-08 | **Performance** | WebSocket fan-out for real-time dashboard updates (no polling) |
| NF-09 | **Reliability** | MQTT session state; LWT for disconnect detection; pending OTA jobs delivered on reconnect |
| NF-10 | **Configuration** | Priority cascade: OS env → `./config.yml` → `/etc/iot-platform/config.yml` |
| NF-11 | **Maintainability** | Database migrations in `internal/db/migrations/`; auto-applied at startup |
| NF-12 | **Extensibility** | Plugin catalog with Go `init()` registration; form-driven UI generation |

---

## 7. Architecture

> **Diagram**: [Platform Architecture](14-diagrams.md#platform-architecture)

```mermaid
graph TB
    subgraph Devices["ESP32 Devices"]
        ESP1["Sensor / Actuator"]
        ESP2["LED Controller"]
        ESP3["Camera / Other"]
    end

    subgraph Platform["Cogitor IoT Platform (single Go binary)"]
        subgraph Transport["MQTT Transport"]
            Broker["MQTT Broker<br/>(mochi-mqtt, embedded)"]
        end

        subgraph Core["Core Processing"]
            ACL["Device ACL<br/>(product.device identity)"]
            Ingest["Telemetry Ingest"]
            PluginHub["Plugin Hub<br/>(in-process)"]
            OTA["OTA Manager<br/>(Auto-OTA)"]
        end

        subgraph Services["Services & Storage"]
            Rules["Rules Bus<br/>publisher"]
            WS["WebSocket Hub<br/>(real-time)"]
            DB["SQLite<br/>(WAL mode)"]
            API["REST API<br/>(chi router)"]
            Auth["JWT / OAuth<br/>Auth"]
            MCP["MCP Endpoint<br/>/mcp"]
        end
    end

    subgraph External["External Clients"]
        UI["React Dashboard<br/>(embedded SPA)"]
        NR["Node-RED<br/>(optional, automation)"]
        LLM["LLM / AI Client<br/>(Cursor, Claude)"]
    end

    %% Device → Transport
    ESP1 & ESP2 & ESP3 -->|"MQTT TCP :1883<br/>MQTT WS :8083"| Broker

    %% Transport → Core
    Broker --> ACL
    ACL --> Ingest

    %% Core internal
    Ingest --> PluginHub
    Ingest --> OTA

    %% Core → Services
    Ingest --> DB
    Ingest --> WS
    Ingest --> Rules
    PluginHub -->|"InvokeRPC"| Broker
    OTA -->|"MQTT downlink<br/>v1/o/upd"| Broker

    %% Services internal
    API --> Auth
    API --> DB
    API --> WS
    API --> PluginHub
    MCP --> API

    %% Services ↔ External
    Rules -->|"platform/v1/events/..."| NR
    NR -->|"platform/v1/commands/..."| Broker
    UI -->|"REST + WS<br/>JWT / API token"| API
    LLM -->|"Streamable HTTP<br/>JWT / API token"| MCP

    style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
    style Transport fill:#0f3460,stroke:#16213e,color:#eee
    style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Services fill:#16213e,stroke:#1a1a2e,color:#eee
    style Devices fill:#0f3460,stroke:#16213e,color:#eee
    style External fill:#16213e,stroke:#1a1a2e,color:#eee
```

### Data Flow

1. **Device → Platform**: MQTT CONNECT with `{product}.{device}` / access token. PUBLISH telemetry on `v1/t` or `iot/v1/.../up/telemetry/data`. Platform ingests in-process, stores in SQLite, broadcasts via WebSocket, publishes to rules bus.
2. **Platform → Device**: RPC requests, desired attribute updates, OTA notifications published on downlink topics. Topic style auto-detected from device's uplink prefix.
3. **Platform ↔ Node-RED**: Events on `platform/v1/events/...`; commands on `platform/v1/commands/...`. Node-RED uses dedicated MQTT user with ACL-limited topics.
4. **Dashboard → Platform**: REST API + WebSocket for real-time updates. JWT or API token auth.
5. **LLM → Platform**: MCP endpoint at `/mcp` — tools for device listing, telemetry, RPC, OTA.

---

## 8. MQTT Protocol (Cogitor v1)

### Topic Styles

| Style | Prefix | Identity source |
|-------|--------|-----------------|
| Full | `iot/v1/{product}/{device}/{up|down}/{capability}/{operation}` | Topic path |
| Short | `v1/t`, `v1/r/req`, `v1/a/upd`, … | MQTT username (`{product}.{device}`) |

### Key Topics

| Direction | Short | Purpose |
|-----------|-------|---------|
| Up | `v1/s` | Status (LWT) |
| Up | `v1/t` | Telemetry data |
| Up | `v1/e` | Events |
| Up | `v1/a` | Attributes report |
| Down | `v1/a/upd` | Attributes update (desired) |
| Down | `v1/r/req` | RPC request |
| Up | `v1/r/res` | RPC response |
| Down | `v1/o/upd` | OTA update (URL) |
| Up | `v1/o/p` | OTA progress |
| Up | `v1/n/req` | NTP request |
| Down | `v1/n/res` | NTP response |

---

## 9. Configuration

### Environment Variables (key subset)

| Variable | Default | Purpose |
|----------|---------|---------|
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `MQTT_ADDR` | `:1883` | Embedded MQTT TCP listen |
| `SQLITE_PATH` | `/var/lib/iot-platform/platform.db` | SQLite database file |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Public URL for OTA downloads, email links, OAuth callbacks |
| `JWT_SECRET` | (dev default) | JWT signing key — **must change in production** |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@cogitor.local` / `admin` | Bootstrap admin credentials |
| `TIMEZONE` | `UTC` | IANA timezone for schedules and quiet hours |
| `TELEMETRY_RETENTION_DAYS` | `30` | Auto-cleanup period |
| `RULES_BUS_ENABLED` | `true` | Enable Node-RED event/command bus |
| `BROKER_HOST_PUBLIC` | `localhost` | MQTT host shown in device credentials |

### Configuration File Priority

1. OS environment variables
2. `.env` file
3. `./config.yml`
4. `/etc/iot-platform/config.yml`

---

## 10. Deployment

### Docker Compose (recommended)

```yaml
services:
  platform:
    build: .
    ports:
      - "8080:8080"   # HTTP + Dashboard
      - "1883:1883"   # MQTT
    volumes:
      - platform_data:/data
    environment:
      - PUBLIC_BASE_URL=http://<host-ip>:8080
      - JWT_SECRET=<change-me>

  nodered:              # optional
    image: nodered/node-red:4.0
    ports:
      - "1880:1880"
```

### Bare Metal (development)

```bash
make build        # builds web UI + Go binary → bin/platform.exe
./bin/platform.exe
```

### Synology NAS

- Gitea CI builds image on push to `master`
- Mounts `/volume1/docker/iot-platform-go/data` for SQLite persistence
- `.env` from `/volume1/docker/iot-platform-go/etc/.env`

---

## 11. REST API Summary

| Area | Endpoints |
|------|-----------|
| Auth | `POST /auth/login`, `POST /auth/register`, `GET /auth/config`, Google OAuth |
| Overview | `GET /overview`, `GET /healthz`, `GET /readyz`, `GET /settings` |
| Products | CRUD `/products`, `POST /products/{id}/esp-defaults` |
| Devices | CRUD `/devices`, `POST /devices/{id}/token/rotate`, `POST /devices/{id}/enable`, `POST /devices/{id}/disable` |
| Telemetry | `GET /devices/{id}/telemetry`, `GET /devices/{id}/telemetry/latest` |
| Attributes | `GET /devices/{id}/attributes`, `PATCH /devices/{id}/attributes/desired` |
| RPC | `POST /devices/{id}/rpc`, `GET /devices/{id}/rpc` (history) |
| OTA | `POST /ota/firmwares/upload`, `POST /ota/firmwares/{id}/notify`, `POST /devices/{id}/ota` |
| Events/Logs | `GET /events`, `GET /logs` |
| Plugins | `GET /plugins`, CRUD `/devices/{id}/plugins`, `POST …/apply` |
| MQTT | `GET /mqtt` (sessions + events) |
| MCP | `/mcp` (streamable HTTP, JWT/token auth) |
| WebSocket | `GET /api/v1/ws?token=<jwt>` |

---

## 12. Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.25+, chi router |
| Database | SQLite (modernc.org/sqlite — pure Go, no CGO) |
| MQTT Broker | mochi-mqtt (embedded) |
| Frontend | React 18 + Bootstrap 5 + Vite |
| Auth | JWT (golang-jwt), Google OAuth |
| Real-time | gorilla/websocket |
| Automation | Node-RED (optional, via MQTT rules bus) |
| AI Integration | MCP (modelcontextprotocol/go-sdk) |
| Container | Multi-stage Docker (Node.js 22 → Go 1.25 → Alpine 3.20) |

---

## 13. Out of Scope (v1)

- ~~Multi-tenancy / team workspaces~~ — implemented, see [16-multi-account.md](16-multi-account.md)
- Cloud relay / hybrid cloud-edge
- Mobile native apps (responsive web is sufficient)
- Video streaming from ESP32-CAM
- Mesh networking / device-to-device communication
- Third-party plugin marketplace (plugins are Go builtins)
- Internationalization (UI is English-only)

---

## 14. Future Considerations (v2+)

- **Multi-account / IAM**: see [16-multi-account.md](16-multi-account.md) — implemented release: tenants, per-account roles, account-scoped rules, built-in monitoring
- **MQTT 5 features**: shared subscriptions, message expiry
- **TimescaleDB / InfluxDB**: swap SQLite for high-volume telemetry
- **Plugin marketplace**: user-contributed plugins with web-based install
- **Fleet management**: groups, tags, bulk operations
- **Alerting**: email/webhook on threshold crossing (without Node-RED)
- **mDNS / local discovery**: auto-detect platform from firmware
- **TLS for MQTT**: native TLS listener or reverse-proxy guidance
- **Grafana integration**: telemetry export / direct SQLite datasource

---

## 15. Acceptance Criteria

A device deployment is considered successful when:

- [ ] Device connects with access token and appears **online** in dashboard
- [ ] Telemetry is visible in device detail and updates in real-time via WebSocket
- [ ] Desired attribute patch reaches the device via MQTT downlink
- [ ] RPC round-trip completes with matching `id` and `code: 0`
- [ ] NTP request receives `serverRecvTime` / `serverSendTime`
- [ ] Token rotation invalidates old session; new token works immediately
- [ ] Node-RED receives events on `platform/v1/events/...`
- [ ] Auto-OTA pushes firmware to device on reconnect when enabled
- [ ] MCP client can list devices and invoke RPC via `/mcp`

---

## Related documentation

| Doc | What |
|-----|------|
| [02-api.md](02-api.md) | REST API reference |
| [03-auth.md](03-auth.md) | Authentication & roles |
| [04-mqtt.md](04-mqtt.md) | MQTT protocol & topics |
| [05-mqtt-tls.md](05-mqtt-tls.md) | MQTT over TLS |
| [06-automation.md](06-automation.md) | Node-RED & rules bus |
| [07-internal-plugins.md](07-internal-plugins.md) | In-process device plugins |
| [08-external-plugins.md](08-external-plugins.md) | Out-of-process plugins |
| [09-product-schema.md](09-product-schema.md) | Product schema (RPC, telemetry, attributes) |
| [10-mcp.md](10-mcp.md) | MCP (LLM integration) |
| [11-operations.md](11-operations.md) | Deployment & configuration |
| [16-multi-account.md](16-multi-account.md) | Multi-account, IAM, monitoring, plugin policies |
