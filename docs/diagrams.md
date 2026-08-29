# Diagrams

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
            Bus["Шина устройств<br/>(MQTT ingest)"]
            Hub["PluginHub<br/>(ticker, dispatch)"]
        end

        subgraph Support[" "]
            Reg["External Plugin<br/>Registry"]
            Queue["Event Queue<br/>(для polling)"]
            HB["Heartbeat<br/>Monitor"]
        end

        HTTP["HTTP / gRPC клиент"]
    end

    subgraph Plugin["Внешний плагин (любой язык)"]
        subgraph Receivers[" "]
            EP["HTTP endpoint<br/>(webhook)"]
            Poll["Poller<br/>(polling)"]
            HBClient["Heartbeat Client"]
        end

        Logic["Бизнес-логика плагина"]
    end

    %% Ingest pipeline
    Bus -->|"MQTT события / tick"| Hub

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
    participant D as Устройство
    participant P as Платформа
    participant Pl as Плагин

    D->>P: MQTT telemetry
    P->>P: PluginHub.Dispatch(kind, payload)
    P->>Pl: POST plugin.on_event (JSON-RPC notification)
    Pl->>Pl: обработка события
    Pl->>P: POST platform.invoke_rpc (JSON-RPC request)
    P->>D: MQTT downlink (RPC команда)
```

## External Plugin Tick Flow

Plugins with `auto: true` and `interval_ms > 0` receive periodic tick events.
The platform owns the timer; the plugin only reacts.

```mermaid
sequenceDiagram
    participant Hub as PluginHub
    participant Pl as Плагин

    loop каждые interval_ms
        Hub->>Pl: POST plugin.on_event {kind: "tick"}
        Pl->>Pl: обработка
        Pl-->>Hub: HTTP 200
    end
```

## External Plugin Heartbeat

The plugin periodically reports liveness to the platform. If no heartbeat arrives
within 90 seconds the plugin is marked offline and event delivery is paused.

```mermaid
sequenceDiagram
    participant Pl as Плагин
    participant P as Платформа

    loop каждые 30 сек
        Pl->>P: platform.heartbeat {status: "ok"}
        P-->>Pl: {ok: true, pending_events: 5}
    end

    Note over Pl,P: Если нет heartbeat 90 сек → плагин offline
    P->>P: помечает плагин offline
    P->>P: приостанавливает доставку событий
```

## External Plugin End-to-End Flow

Complete interaction flow for the Python temperature-alert plugin: registration,
config apply, telemetry ingest, threshold check, and RPC dispatch.

```mermaid
sequenceDiagram
    participant Pl as Плагин (Python)
    participant P as Платформа
    participant U as Пользователь (UI)
    participant D as Устройство

    Pl->>P: POST /register → получает token
    U->>P: Привязать плагин к устройству
    P->>Pl: plugin.apply_config {threshold_c: 30}
    D->>P: MQTT telemetry {temperature: 32.5}
    P->>Pl: plugin.on_event {kind: "telemetry", payload: {...}}
    Pl->>Pl: 32.5 > 30 → нужно вызвать RPC
    Pl->>P: platform.invoke_rpc("set_fan", {on: true})
    P->>D: MQTT downlink → RPC команда
```

## External Plugin Lifecycle

Full lifecycle from plugin process start through registration, heartbeat,
binding, device events, apply, offline detection, and detach.

```mermaid
sequenceDiagram
    participant Pl as Плагин
    participant P as Платформа
    participant U as Пользователь
    participant D as Устройство

    Note over Pl: Старт процесса
    Pl->>P: POST /register
    P-->>Pl: {ok: true, token: "..."}

    loop каждые 30 сек
        Pl->>P: platform.heartbeat
        P-->>Pl: {ok: true}
    end

    Note over U: Привязка плагина к устройству
    U->>P: Создать binding
    P->>Pl: plugin.apply_config
    Pl-->>P: HTTP 200

    Note over D: Устройство подключается
    D->>P: MQTT status {online: true}
    P->>Pl: plugin.on_event {kind: "status", online: true}

    Note over D: Телеметрия
    D->>P: MQTT telemetry
    P->>Pl: plugin.on_event {kind: "telemetry"}
    Pl->>P: platform.invoke_rpc (опционально)

    Note over P: Тик (если auto=true)
    P->>Pl: plugin.on_event {kind: "tick"}

    Note over U: Пользователь нажимает «Apply now»
    U->>P: Обновить binding
    P->>Pl: plugin.apply_config
    P->>Pl: plugin.on_event {kind: "apply"}

    Note over Pl: Плагин остановлен
    Pl->>P: (нет heartbeat 90 сек)
    P->>P: помечает плагин offline

    Note over U: Удаление binding
    U->>P: Удалить binding
    P->>Pl: plugin.detach (если online)
    Pl->>Pl: Освобождение ресурсов
```

## Internal Plugin Architecture

In-process plugin architecture showing how device MQTT events flow through ingest
and PluginHub to per-binding runners, and how the dashboard manages bindings.

```mermaid
flowchart LR
  Dev[Device MQTT] -->|status / telemetry / rpc result| Ing[ingest]
  UI[Dashboard Plugins] -->|Save / Apply now| API["REST /devices/id/plugins"]
  Ing --> PR[publishRules]
  PR --> Hub[PluginHub]
  PR -.->|optional| NR[Node-RED]
  API --> SQ[(device_plugin_bindings)]
  API --> Hub
  SQ --> Hub
  Cat[Catalog Register init] --> Hub
  Hub --> R[runner per binding]
  R --> AC[ApplyConfig]
  AC --> OE[OnEvent]
  OE --> RPC[InvokeRPC]
  RPC --> Dev
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
