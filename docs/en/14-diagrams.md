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
