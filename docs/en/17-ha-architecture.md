# HA / Clustering — Architecture (English)

> **Version**: 1.0 (condensed summary)
> **Canonical version**: [ru/17-ha-architecture.md](../ru/17-ha-architecture.md) (canonical, actively maintained)
> **Status**: Draft — design document; this English file is a condensed summary of the Russian one, not a full translation
> **Last updated**: September 2026
> **Scope**: multi-instance HA deployment for v2.0+ (RabbitMQ, PostgreSQL, SSE, External Plugin SDK)

> ⚠️ **Design document for v2.x — not implemented.** The current code has no RabbitMQ/AMQP, no SSE and no clustering: there is a single binary (`cmd/platform`) with the embedded mochi-mqtt broker, real-time updates go over WebSocket `/api/v1/ws`, external plugins use MQTT only, and there is no `pkg/plugin-sdk`, `platform-cluster` or `platform-worker`. For current behaviour see [01-prd](01-prd.md) and [11-operations](11-operations.md).

This document describes the **HA/clustering** architecture — a multi-instance deployment for horizontal scaling. It continues [16-multi-account.md](16-multi-account.md), which covers single-instance mode (v1.0..v1.6).

**Strategy:** finish v1.x (single mode) first, then v2.x (HA). No single → cluster migration is planned — they are **alternative deployment options** chosen at install time.

Diagrams (Mermaid) are referenced from [14-diagrams.md](14-diagrams.md); the canonical versions are in the Russian document.

---

## 0. Summary

- **Cluster is an alternative deployment option**, not a migration target; single mode stays fully supported.
- **Three binaries** in one repo: `platform-single` (v1.x, as today), `platform-cluster` (UI, REST API, SSE, Prometheus, RabbitMQ client), `platform-worker` (stateless execution of internal plugins).
- **RabbitMQ** is mandatory for the cluster: MQTT broker for devices, AMQP between instances and for external plugins.
- **PostgreSQL** is mandatory in the cluster (SQLite is not supported there).
- **SSE** (`GET /api/v1/events/stream`) is planned for UI updates in **both** modes — HTTP-based, no sticky sessions.
- **External plugins** use one SDK; transport is `mqtt` in single mode and `mqtt` or `amqp` in the cluster.
- The embedded MQTT broker (mochi-mqtt) remains in single mode only; the cluster uses RabbitMQ with the MQTT plugin.

---

## 1. Architecture overview

### 1.1 Two deployment options

| Aspect | Single mode (v1.x) | Cluster mode (v2.x) |
|--------|--------------------|---------------------|
| Binary | `platform-single` | `platform-cluster` + `platform-worker` |
| MQTT broker | Embedded (mochi-mqtt) | RabbitMQ (MQTT plugin) |
| Database | SQLite or PostgreSQL | PostgreSQL only |
| External plugins | MQTT only | MQTT or AMQP (via RabbitMQ) |
| Internal plugins | In-process | In workers (stateless) |
| Use case | Small/medium (≤1k devices) | Large (1k+ devices) |

### 1.2 Monorepo structure

Separate `cmd/platform-single`, `cmd/platform-cluster`, `cmd/platform-worker` over a shared `internal/` (~80% common code), a transport abstraction (`internal/transport/{mqtt,amqp}`) and a public `pkg/plugin-sdk`. No build tags — different binaries come from different `cmd/` directories.

### 1.3 HA architecture overview

N stateless `platform-cluster` instances behind an L7 load balancer, N workers, one RabbitMQ (optionally clustered) and one PostgreSQL. Devices and external plugins connect to RabbitMQ, not to a specific platform instance.

### 1.4 Single vs cluster

Side-by-side comparison (diagram in the RU document). The same plugin code (via the SDK) runs in both modes; in the cluster a plugin may connect to RabbitMQ's MQTT port (simple, same as single mode) or its AMQP port (richer routing).

---

## 2. Components

### 2.1 RabbitMQ (mandatory)

MQTT broker for devices, AMQP for inter-instance traffic and external plugins. Planned exchanges: `iot.devices` (topic, telemetry), `iot.commands` (topic, commands), `iot.cluster` (fanout, events to all instances), `iot.external` (topic, events for external plugins, routed by `account_slug.#`), plus a per-instance queue for SSE events.

### 2.2 PostgreSQL (mandatory)

Shared state for all instances. Device presence is derived from MQTT LWT and RabbitMQ connect/disconnect events (updating `devices.last_seen_at`). Two independent tables — `rpc_calls` (RPC history) and `audit_log` (connection, status, log and RPC events with `instance_id`).

### 2.3 Cluster instance (`platform-cluster`)

UI + REST API server, SSE server, simple Prometheus exporter and RabbitMQ client. No embedded MQTT broker, no internal plugins, no WebSocket server. Stateless — N instances behind an L7 LB.

### 2.4 Worker (`platform-worker`)

Runs internal plugins (schedule, led-garland, …) as in-process handlers subscribed via the same SDK; stateless, no leader election. Every worker receives all events, so plugins must deduplicate via unique DB constraints. Conceptually a worker is "an external plugin whose code is compiled in".

### 2.5 External plugins (MQTT or AMQP)

Plugins connect directly to RabbitMQ with `extplg_{slug}_{id}` credentials — via MQTT (same topics as single mode, only the broker address changes) or AMQP (exchange `iot.external`, routing key `events.{slug}.#`; commands to `iot.commands`).

- **2.5.1 ACL in the cluster** — enforced by RabbitMQ itself (MQTT topic authorization / vhost permissions), so a plugin of account `acme` cannot read or write another account's topics.
- **2.5.2 Node-RED** — keep the MQTT nodes and just point them to RabbitMQ `:1883`, or switch to `node-red-contrib-amqp` (`:5672`) for advanced routing.

---

## 3. Data flows

- **3.1 Telemetry** — device → RabbitMQ (MQTT plugin) → instances/workers/external plugins via exchanges → PostgreSQL.
- **3.2 Command routing and response correlation** — any instance publishes a command to `iot.commands`; RabbitMQ delivers it over the device's MQTT session. The response carries the same `request_id`; instances correlate it in memory after a fanout. Synchronous waiting (`?wait=1`) is **only** for the HTTP API/UI — plugins and Node-RED work asynchronously.
- **3.3 Event flow (SSE)** — the instance subscribes to `events.{account_slug}.#` for the user's accounts, filters by membership and streams `text/event-stream`; the browser reconnects with `Last-Event-ID`.

---

## 4. External Plugin SDK

Planned public Go module `pkg/plugin-sdk` with a single API (subscribe to events, publish commands). The transport is set explicitly in config (`mqtt` or `amqp`); the SDK maps topics to routing keys, handles reconnects/heartbeats, serialization, errors and metrics.

---

## 5. Functional requirements (HA-specific)

Detailed F2-xxx tables live in the Russian sub-document `ru/17-ha-architecture-requirements.md`: F2-01…F2-07 — HA Foundation (v2.0); F2-21…F2-26 — account-level features (v2.2: RBAC, rate limiting, retention, import/export, signing, remote_write); F2-31…F2-36 — future (v2.3+).

**Explicitly rejected:** DB-per-account, per-account sub-domains, white-label CSS, embedded NATS, WebSocket in cluster mode, Redis middleware, `rabbitmq-bridge` plugin, gRPC and webhook/polling transports for external plugins.

---

## 6. HA release plan

| Version | Theme | Exit criterion (short) |
|---------|-------|------------------------|
| v2.0 | HA Foundation | 2 cluster instances + 1 worker behind an L7 LB; a command from instance B reaches a device connected via instance A; SSE and Prometheus work |
| v2.1 | rabbitmq-bridge | (cancelled — see §5) |
| v2.2 | Account-level features | Custom RBAC, per-account rate limiting, account import/export round-trip |
| v2.3+ | Future | Multi-region DR, SSO per account, SIEM integration |

Rules: v1.x is completed first; each HA stage ends with a review checkpoint and CHANGELOG entry; docs are updated in the same PR as code; single mode remains supported alongside v2.x.

---

## 7. Backlog / Future

- **7.1** Simple single → cluster migration utility (only if there is demand, after v2.0).
- **7.2** Git labels/tags per version.
- **7.3** Multi-region / DR via PostgreSQL streaming replication (v2.3+).
- **7.4** Plugin marketplace with signed catalog plugins (v2.3+).
- **7.5** Removal of the gRPC transport from examples and code (planned for v1.1).
- **7.6** HA-readiness contract with v1 — v1 stays single-instance (NF-19) but avoids decisions that would block v2.

---

## 8. Related docs (English)

- [Multi-Account & Monitoring (en/16)](16-multi-account.md) — single mode v1.0..v1.6 specification (see its implementation-status banner)
- [Diagrams (en/14)](14-diagrams.md) — Mermaid architecture and flow diagrams
- [External Plugins (en/08)](08-external-plugins.md) — external plugin architecture
- [Internal Plugins (en/07)](07-internal-plugins.md) — internal plugin architecture
