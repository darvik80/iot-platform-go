# Cogitor IoT Platform — Documentation

[Русская версия](README_ru.md)

## Documents

| Document | Description |
|----------|-------------|
| [Product Requirements (PRD)](en/01-prd.md) | Product vision, features, acceptance criteria |
| [REST API](en/02-api.md) | Full REST API reference (`/api/v1`) |
| [Auth & Users](en/03-auth.md) | Authentication, registration, roles, Google OAuth |
| [MQTT Protocol](en/04-mqtt.md) | Device MQTT contract, topic styles, downlink |
| [MQTT over TLS](en/05-mqtt-tls.md) | TLS setup, certificate generation |
| [Automation / Node-RED](en/06-automation.md) | Rules bus, Node-RED connected as an external plugin |
| [Internal Plugins](en/07-internal-plugins.md) | In-process plugins, builtin catalog, lifecycle, RPC |
| [External Plugins](en/08-external-plugins.md) | External plugins over MQTT (implemented); webhook / gRPC contract (planned) |
| [Product Schema](en/09-product-schema.md) | Product definition: metrics, attributes, RPC methods |
| [MCP (LLM Tools)](en/10-mcp.md) | Model Context Protocol endpoint for AI tools |
| [Operations](en/11-operations.md) | Backup, restore, monitoring, troubleshooting |
| [Deploy VDS](en/12-deploy-vds.md) | Bare-metal / VPS deployment with systemd |
| [Deploy Docker](en/13-deploy-docker.md) | Docker image deployment (compose file ships Node-RED only) |
| [Diagrams](en/14-diagrams.md) | Mermaid architecture and flow diagrams |
| [Diagram Style Guide](en/15-diagram-style.md) | Style guide for Mermaid diagrams (for agents) |
| [Multi-Account & Monitoring](en/16-multi-account.md) | Tenants, IAM, invitations, Prometheus exporter, plugin policies (single mode v1.0..v1.6) |
| [HA / Clustering](en/17-ha-architecture.md) | **Design, not implemented.** Multi-instance deployment, RabbitMQ, PostgreSQL, realtime fan-out, External Plugin SDK (v2.x) |

## Images

| File | Content |
|------|---------|
| [architecture.svg](images/architecture.svg) | Current platform architecture (ingress, MQTT transport, core services, storage, integrations) |
| [device-onboarding.svg](images/device-onboarding.svg) | Product → device → firmware → online |
| `dashboard-*.png` | Screenshots of the web UI (overview, devices, device detail, MQTT) |
