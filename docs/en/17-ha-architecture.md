# HA / Clustering — Architecture (English)

> **Version**: 1.0 (not yet started)
> **Canonical version**: [ru/17-ha-architecture.md](../ru/17-ha-architecture.md) (canonical, actively maintained)
> **Status**: Not started — will be rewritten from scratch after the Russian version stabilises
> **Last updated**: September 2026
> **Scope**: multi-instance HA deployment for v2.0+ (RabbitMQ, PostgreSQL, SSE, External Plugin SDK)

> ⚠️ **English version does not exist yet.**
> The Russian version (linked above) is the source of truth. This file is a placeholder so that cross-references and table indexes resolve.
>
> Why the delay: the Russian version is being actively iterated; translating to English now would create a second source of truth that drifts. We pick one canonical language for v2 and translate once the architecture is locked (after v1.6 is shipped).

---

## Quick reference (read the Russian version)

The English version of this document will cover:

- **TL;DR** — 3 binaries (single, cluster, worker), RabbitMQ for cluster, PostgreSQL for cluster, SSE instead of WebSocket
- **§1 Architecture overview** — two deployment options (single vs cluster), monorepo structure (no build tags)
- **§2 Components** — RabbitMQ (with topology), PostgreSQL (`device_instances` table), Cluster Instance, Worker, External Plugins
- **§3 Data flows** — telemetry, command routing, SSE event flow (all Mermaid)
- **§4 External Plugin SDK** — single API; transport selected by deployment mode (`mqtt` in single, `amqp` in cluster)
- **§5 Functional requirements (HA-specific)** — F2-01..F2-07 (v2.0), F2-21..F2-26 (v2.2), F2-31..F2-36 (v2.3+)
- **§6 Release plan** — staged rollout of HA features
- **§7 Backlog** — including v1.1: removal of gRPC transport from SDK
- **§8 Related documentation** — links to 16-multi-account and others

See the [Russian canonical version](../ru/17-ha-architecture.md) for the full content.

---

## Related docs (English)

- [Multi-Account & Monitoring (en/16)](16-multi-account.md) — single mode v1.0..v1.6 (deprecated, see Russian canonical)
- [Diagrams (en/14)](14-diagrams.md) — Mermaid architecture and flow diagrams
- [External Plugins (en/08)](08-external-plugins.md) — external plugin architecture
- [Internal Plugins (en/07)](07-internal-plugins.md) — internal plugin architecture
