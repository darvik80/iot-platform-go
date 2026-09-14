# Diagrams

Architecture diagrams and sequence flow diagrams. Rendered with Mermaid — works in GitHub, GitLab, Obsidian, and most Markdown viewers.

Related docs: [01-PRD](01-prd.md), [07-internal-plugins](07-internal-plugins.md), [08-external-plugins](08-external-plugins.md).

---

## Platform Architecture

High-level system architecture of the Cogitor IoT Platform showing all
embedded components and external clients.

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

## External Plugin Architecture

High-level architecture of the external plugin system showing the IoT Platform
components and an out-of-process plugin communicating over HTTP/gRPC.

```mermaid
graph TB
    subgraph Platform["IoT Platform (Go)"]
        subgraph Ingest[" "]
            Bus["Device Bus<br/>(MQTT ingest)"]
            Hub["PluginHub<br/>(ticker, dispatch)"]
        end

        subgraph Support[" "]
            Reg["External Plugin<br/>Registry"]
            Queue["Event Queue<br/>(for polling)"]
            HB["Heartbeat<br/>Monitor"]
        end

        HTTP["HTTP / gRPC client"]
    end

    subgraph Plugin["External Plugin (any language)"]
        subgraph Receivers[" "]
            EP["HTTP endpoint<br/>(webhook)"]
            Poll["Poller<br/>(polling)"]
            HBClient["Heartbeat Client"]
        end

        Logic["Plugin Business Logic"]
    end

    %% Ingest pipeline
    Bus -->|"MQTT events / tick"| Hub

    %% Platform → Plugin (webhook)
    Hub -->|"webhook: POST"| HTTP
    HTTP -->|"plugin.on_event"| EP

    %% Polling
    Hub -->|"polling: queue"| Queue
    Poll -->|"poll_events"| HTTP

    %% Plugin internal
    EP --> Logic
    Poll --> Logic

    %% Plugin → Platform (RPC)
    Logic -->|"invoke_rpc"| HTTP

    %% Registry
    Reg -.->|"endpoint URL"| HTTP

    %% Heartbeat
    HBClient -->|"heartbeat"| HTTP
    HTTP --> HB
    HB -.->|"online/offline"| Reg

    style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
    style Ingest fill:#0f3460,stroke:#16213e,color:#eee
    style Support fill:#16213e,stroke:#1a1a2e,color:#eee
    style Plugin fill:#0f3460,stroke:#16213e,color:#eee
    style Receivers fill:#1a1a2e,stroke:#0f3460,color:#eee
```

## External Plugin Event Flow

Sequence diagram showing how a device telemetry event flows through the platform
to an external plugin and back as an RPC command.

```mermaid
sequenceDiagram
    participant D as Device
    participant P as Platform
    participant Pl as Plugin

    D->>P: MQTT telemetry
    P->>P: PluginHub.Dispatch(kind, payload)
    P->>Pl: POST plugin.on_event (JSON-RPC notification)
    Pl->>Pl: process event
    Pl->>P: POST platform.invoke_rpc (JSON-RPC request)
    P->>D: MQTT downlink (RPC command)
```

## External Plugin Tick Flow

Plugins with `auto: true` and `interval_ms > 0` receive periodic tick events.
The platform owns the timer; the plugin only reacts.

```mermaid
sequenceDiagram
    participant Hub as PluginHub
    participant Pl as Plugin

    loop every interval_ms
        Hub->>Pl: POST plugin.on_event {kind: "tick"}
        Pl->>Pl: process
        Pl-->>Hub: HTTP 200
    end
```

## External Plugin Heartbeat

The plugin periodically reports liveness to the platform. If no heartbeat arrives
within 90 seconds the plugin is marked offline and event delivery is paused.

```mermaid
sequenceDiagram
    participant Pl as Plugin
    participant P as Platform

    loop every 30 sec
        Pl->>P: platform.heartbeat {status: "ok"}
        P-->>Pl: {ok: true, pending_events: 5}
    end

    Note over Pl,P: No heartbeat for 90 sec → plugin offline
    P->>P: mark plugin offline
    P->>P: pause event delivery
```

## External Plugin End-to-End Flow

Complete interaction flow for the Python temperature-alert plugin: registration,
config apply, telemetry ingest, threshold check, and RPC dispatch.

```mermaid
sequenceDiagram
    participant Pl as Plugin (Python)
    participant P as Platform
    participant U as User (UI)
    participant D as Device

    Note over U: Admin creates plugin in UI / API
    U->>P: POST /api/v1/plugins/external → token + MQTT creds
    U-->>Pl: pass credentials to plugin
    U->>P: Bind plugin to device
    P->>Pl: plugin.apply_config {threshold_c: 30}
    D->>P: MQTT telemetry {temperature: 32.5}
    P->>Pl: plugin.on_event {kind: "telemetry", payload: {...}}
    Pl->>Pl: 32.5 > 30 → need to call RPC
    Pl->>P: platform.invoke_rpc("set_fan", {on: true})
    P->>D: MQTT downlink → RPC command
```

## External Plugin Lifecycle

Full lifecycle from plugin process start through registration, heartbeat,
binding, device events, apply, offline detection, and detach.

```mermaid
sequenceDiagram
    participant Pl as Plugin
    participant P as Platform
    participant U as User
    participant D as Device

    Note over U: Admin creates plugin in UI
    U->>P: POST /api/v1/plugins/external
    P-->>U: {ok: true, token: "...", mqtt_user: "...", ...}
    U-->>Pl: pass token + MQTT creds to plugin

    loop every 30 sec
        Pl->>P: platform.heartbeat
        P-->>Pl: {ok: true}
    end

    Note over U: Bind plugin to device
    U->>P: Create binding
    P->>Pl: plugin.apply_config
    Pl-->>P: HTTP 200

    Note over D: Device connects
    D->>P: MQTT status {online: true}
    P->>Pl: plugin.on_event {kind: "status", online: true}

    Note over D: Telemetry
    D->>P: MQTT telemetry
    P->>Pl: plugin.on_event {kind: "telemetry"}
    Pl->>P: platform.invoke_rpc (optional)

    Note over P: Tick (if auto=true)
    P->>Pl: plugin.on_event {kind: "tick"}

    Note over U: User presses "Apply now"
    U->>P: Update binding
    P->>Pl: plugin.apply_config
    P->>Pl: plugin.on_event {kind: "apply"}

    Note over Pl: Plugin stopped
    Pl->>P: (no heartbeat for 90 sec)
    P->>P: mark plugin offline

    Note over U: Remove binding
    U->>P: Delete binding
    P->>Pl: plugin.detach (if online)
    Pl->>Pl: Release resources
```

## Internal Plugin Architecture

In-process plugin architecture showing how device MQTT events flow through ingest
and PluginHub to per-binding runners, and how the dashboard manages bindings.

```mermaid
flowchart TB
  subgraph Ingest["MQTT Ingest"]
    Dev[Device MQTT] -->|status / telemetry / rpc result| Ing[ingest]
  end

  subgraph Core["Plugin Processing"]
    Ing --> PR[publishRules]
    PR --> Hub[PluginHub]
    PR -.->|optional| NR[Node-RED]
    Cat[Catalog<br/>Register init] --> Hub
    Hub --> R[runner per binding]
    R --> AC[ApplyConfig]
    AC --> OE[OnEvent]
    OE --> RPC[InvokeRPC]
  end

  subgraph Management["Dashboard"]
    UI[Dashboard Plugins] -->|Save / Apply now| API["REST /devices/id/plugins"]
    API --> SQ[(device_plugin_bindings)]
    API --> Hub
    SQ --> Hub
  end

  %% Core → Device
  RPC --> Dev

  style Ingest fill:#0f3460,stroke:#16213e,color:#eee
  style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
  style Management fill:#16213e,stroke:#1a1a2e,color:#eee
```

## Internal Plugin Lifecycle

Sequence diagram showing the lifecycle of an internal plugin runner: device
connect/disconnect, tick loop, apply on save, and RPC dispatch.

```mermaid
sequenceDiagram
  participant Dev as Device MQTT
  participant Ing as ingest
  participant Hub as PluginHub
  participant P as Instance
  participant RPC as InvokeRPC

  Note over Hub: StartPlugins / bind / save → ApplyConfig + syncRunner
  Dev->>Ing: CONNECT / status online
  Ing->>Hub: Dispatch(status, online)
  Hub->>Hub: syncRunner (ticker if Auto)
  Hub->>P: OnEvent(status, online=true)
  P->>RPC: led_attach / …
  loop while online and Auto()
    Hub->>P: OnEvent(tick)
    P->>RPC: set_led / schedule jobs
  end
  Dev->>Ing: disconnect
  Ing->>Hub: Dispatch(status, online=false)
  Hub->>Hub: stop ticker
  Hub->>P: OnEvent(status, online=false)

  Note over Hub: UI Save or Apply now
  Hub->>P: OnEvent(apply)
  P->>RPC: led_attach then paint / run jobs now
```

## Multi-Account Architecture

> Source: [16-multi-account.md](16-multi-account.md#71-общая-схема)

High-level view of the multi-tenant release: shared broker with per-username
ACL, account-scoped resources, catalog plugins (including the new
`prometheus-exporter` and `metrics-dashboard`), and a per-account policy table
that controls catalog plugins.

```mermaid
graph TB
    subgraph Clients["Clients"]
        U1["User A<br/>(admin in acme)"]
        U2["User B<br/>(operator in beta, viewer in acme)"]
        P1["External Plugin<br/>acme/telegram-bot"]
        P2["External Plugin<br/>beta/zapier"]
    end

    subgraph Platform["Cogitor IoT Platform (single binary)"]
        subgraph Auth["Auth and Tenancy"]
            JWT["JWT<br/>(active_account_id)"]
            IAM["AccountMembership<br/>Middleware"]
        end

        subgraph Broker["MQTT Broker"]
            B1["mochi-mqtt<br/>per-username ACL"]
        end

        subgraph Core["Core"]
            ACL["Device ACL<br/>(product.device)"]
            ING["Ingest and publishRules"]
            HUB["PluginHub<br/>(per-account policy)"]
        end

        subgraph Catalog["Catalog plugins"]
            LG["led-garland"]
            SCH["schedule"]
            PRO["prometheus-exporter<br/>HTTP :9100"]
            MD["metrics-dashboard<br/>UI tab"]
        end

        subgraph DB["SQLite (WAL)"]
            ACC[("accounts")]
            MEM[("account_memberships")]
            INV[("account_invitations")]
            POL[("account_plugin_policies")]
            PR[("products<br/>UNIQUE(account_id, product_id)")]
            DV[("devices")]
            EP[("external_plugins")]
        end

        subgraph Bus["Rules Bus"]
            EV["platform/v1/events/{slug}/..."]
            CM["platform/v1/commands/{slug}/..."]
        end
    end

    %% Clients to Auth
    U1 & U2 -->|"Bearer JWT"| JWT
    P1 & P2 -->|"Bearer token"| JWT

    %% Auth to Core
    JWT --> IAM
    IAM --> ACL
    IAM --> ING
    IAM --> HUB

    %% Devices to broker
    B1 --> ACL
    ACL --> ING
    ING --> HUB
    HUB --> LG & SCH & PRO & MD
    ING --> EV
    CM --> B1

    %% DB links
    IAM --> MEM
    IAM --> ACC
    IAM --> INV
    IAM --> POL
    ING --> DV
    ING --> PR
    HUB --> POL

    %% External plugins to broker
    P1 -->|"MQTT extplg_acme_telegram-bot"| B1
    P2 -->|"MQTT extplg_beta_zapier"| B1

    style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
    style Auth fill:#0f3460,stroke:#16213e,color:#eee
    style Broker fill:#0f3460,stroke:#16213e,color:#eee
    style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Catalog fill:#16213e,stroke:#1a1a2e,color:#eee
    style DB fill:#16213e,stroke:#1a1a2e,color:#eee
    style Bus fill:#0f3460,stroke:#16213e,color:#eee
    style Clients fill:#16213e,stroke:#1a1a2e,color:#eee
```

## Invitation Flow

> Source: [16-multi-account.md](16-multi-account.md#53-приглашения)

End-to-end invitation lifecycle: admin creates, recipient accepts (logged-in or
fresh registration), membership appears, JWT carries the new active account.

```mermaid
sequenceDiagram
    participant A as Admin (UI)
    participant P as Platform
    participant M as Mail (SMTP/log)
    participant R as Recipient
    participant DB as SQLite

    Note over A: Active account = acme
    A->>P: POST /api/v1/accounts/{id}/invitations {email, role}
    P->>DB: INSERT account_invitations (token_hash, expires_at = +72h)
    P-->>A: {token: "raw-once", link: "/invite/{token}"}
    P->>M: send invite email with link
    M-->>R: email arrives

    R->>P: GET /api/v1/invitations/{token}
    P->>DB: lookup by token_hash
    P-->>R: {account_name: "acme", role: "operator"}

    alt Recipient already has an account
        R->>P: POST /invitations/{token}/accept (with JWT)
        P->>DB: INSERT account_memberships(user_id, account_id, role)
        P-->>R: {ok: true, account_id}
        R->>P: POST /auth/switch-account {account_id}
        P-->>R: {token: new JWT with active=acme}
    else New user
        R->>P: GET /auth/register?invite=...&email=...
        R->>P: POST /auth/register (auto-fills email, creates user)
        R->>P: POST /invitations/{token}/accept (with new JWT)
        P->>DB: INSERT account_memberships
        P-->>R: {ok: true, account_id}
    end

    Note over P: Resend / Revoke paths
    A->>P: POST /accounts/{id}/invitations/{id}/resend
    P->>DB: UPDATE old.status='revoked', INSERT new
    P->>M: send new email
```

## External Plugin Account ACL

> Source: [16-multi-account.md](16-multi-account.md#56-внешние-плагины-account-scoping)

External plugin registration, MQTT credential issuance, and the ACL check
that prevents cross-account subscription or publish.

```mermaid
sequenceDiagram
    participant Adm as Admin (UI)
    participant P as Platform
    participant Pl as External Plugin
    participant Br as MQTT Broker

    Note over Adm: Active account = acme
    Adm->>P: POST /api/v1/plugins/external {id: "telegram-bot"}
    P->>P: mqtt_user = "extplg_acme_telegram-bot"
    P->>P: mqtt_pass = random, token = random
    P->>P: hash both, store in external_plugins (account_id=acme)
    P-->>Adm: {token, mqtt_user, mqtt_pass, mqtt_host, mqtt_port}
    Adm->>Pl: pass credentials

    Pl->>Br: CONNECT mqtt_user=extplg_acme_telegram-bot, mqtt_pass=...
    Br->>P: OnConnectAuthenticate
    P->>P: ValidateMQTTCreds -> plugin.account_id=acme
    Br-->>Pl: CONNACK ok

    Note over Pl: Subscribe to own account events (OK)
    Pl->>Br: SUBSCRIBE platform/v1/events/acme/+/+/telemetry
    Br->>P: OnACLCheck(user, topic, write=false)
    P->>P: account_slug in topic matches plugin.account_id
    P-->>Br: allow
    Br-->>Pl: SUBACK

    Note over Pl: Try to subscribe to another account (DENY)
    Pl->>Br: SUBSCRIBE platform/v1/events/beta/+/+/telemetry
    Br->>P: OnACLCheck
    P->>P: topic slug "beta" != plugin.account_id
    P-->>Br: deny
    Br-->>Pl: SUBACK fail, "acl_deny"

    Note over Pl: Publish command to own account device (OK)
    Pl->>Br: PUBLISH platform/v1/commands/acme/esp32-cam/cam-001/rpc
    Br->>P: OnACLCheck(user, topic, write=true)
    P-->>Br: allow
    P->>P: parse command, lookup device, check device.account_id == plugin.account_id
    P-->>D: MQTT downlink RPC

    Note over Pl: Try to publish to another account (DENY)
    Pl->>Br: PUBLISH platform/v1/commands/beta/sensor/sensor-01/rpc
    Br->>P: OnACLCheck
    P-->>Br: deny
    Br-->>Pl: PUBACK fail
```

## Prometheus Exporter Lifecycle

> Source: [16-multi-account.md](16-multi-account.md#59-внутренний-плагин-prometheus-exporter)

How the `prometheus-exporter` catalog plugin starts an HTTP server when a
binding is enabled, exposes Prometheus text with `account` labels, and shuts
down when the per-account policy disables it.

```mermaid
sequenceDiagram
    participant Adm as Admin (UI)
    participant H as PluginHub
    participant EX as prometheus-exporter runner
    participant Pr as Prometheus (external)
    participant DB as SQLite

    Note over Adm: Settings -> Integrations -> prometheus-exporter enabled
    Adm->>H: PATCH /accounts/{id}/plugins/policies/prometheus-exporter {enabled:true}
    H->>DB: UPSERT account_plugin_policies
    H->>H: rebuild account runners

    Note over H: binding created on a device (or hub starts with existing binding)
    Adm->>H: POST /devices/{d}/plugins {plugin_id: "prometheus-exporter"}
    H->>EX: ApplyConfig({port: 9100, bind: 0.0.0.0, bearer_token: ...})
    EX->>EX: http.Server.ListenAndServe on 0.0.0.0:9100

    loop every update_interval_ms
        EX->>DB: SELECT counts grouped by account/product/device/status
        EX->>EX: refresh atomic gauges
    end

    Pr->>EX: GET /metrics (Authorization: Bearer <token>)
    EX->>EX: validate bearer (if configured)
    EX-->>Pr: 200 text/plain (Prometheus exposition, account=acme labels)
    EX->>EX: counter cogitor_device_telemetry_samples_total{account=acme,...}++ on StoreTelemetry (in-process hook)
    EX->>EX: counter cogitor_device_rpc_calls_total{...}++ on InvokeRPC

    Note over Adm: Disable exporter for the account
    Adm->>H: PATCH ... {enabled:false}
    H->>H: iterate account bindings, stop runners
    H->>EX: detach -> http.Server.Shutdown
    EX-->>Pr: connection refused (next scrape)
```

## Internal Plugin Policy

> Source: [16-multi-account.md](16-multi-account.md#58-управление-внутренними-плагинами)

How `account_plugin_policies` controls catalog plugin runners per account,
including dispatch, tick, and account deletion cleanup.

```mermaid
flowchart TB
    subgraph Catalog["Catalog plugins (in init)"]
        LG["led-garland"]
        SCH["schedule"]
        PRO["prometheus-exporter"]
        MD["metrics-dashboard"]
    end

    subgraph Policy["account_plugin_policies"]
        T1["(acme, led-garland, enabled=true)"]
        T2["(acme, schedule, enabled=false)"]
        T3["(beta, led-garland, enabled=true)"]
    end

    subgraph Hub["PluginHub"]
        D["Dispatch(product, device, kind, payload)"]
        AR["ApplyConfig(binding)"]
        TI["Tick loop"]
    end

    subgraph Accounts["Accounts"]
        AC1["acme<br/>devices[]"]
        AC2["beta<br/>devices[]"]
    end

    AR -->|"lookup policy"| Policy
    TI -->|"lookup policy"| Policy
    D -->|"lookup policy"| Policy

    Policy -->|enabled=true| LG
    Policy -->|enabled=true| SCH
    Policy -->|enabled=false| SKIP["skip runner (no OnEvent)"]

    LG -->|bind/apply/tick| AC1
    SCH -->|bind/apply/tick| AC2
    SKIP -.-> AC1

    %% UI control
    UI["Admin UI<br/>Settings -> Integrations"] -->|"PATCH policy"| Policy

    style Catalog fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Policy fill:#0f3460,stroke:#16213e,color:#eee
    style Hub fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Accounts fill:#16213e,stroke:#1a1a2e,color:#eee
```
