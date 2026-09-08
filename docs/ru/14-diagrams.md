# Диаграммы

Архитектурные диаграммы и sequence-диаграммы потоков. Используют Mermaid — рендерятся в GitHub, GitLab, Obsidian и большинстве Markdown-просмотрщиков.

Связанные доки: [01-PRD](01-prd.md), [07-внутренние плагины](07-internal-plugins.md), [08-внешние плагины](08-external-plugins.md).

---

## Архитектура платформы

Высокоуровневая архитектура Cogitor IoT Platform со всеми встроенными компонентами и внешними клиентами.

```mermaid
graph TB
    subgraph Devices["ESP32 Устройства"]
        ESP1["Датчик / Актуатор"]
        ESP2["LED Контроллер"]
        ESP3["Камера / Другое"]
    end

    subgraph Platform["Cogitor IoT Platform (один Go бинарник)"]
        subgraph Transport["MQTT Транспорт"]
            Broker["MQTT Broker<br/>(mochi-mqtt, встроенный)"]
        end

        subgraph Core["Основная обработка"]
            ACL["Device ACL<br/>(product.device идентичность)"]
            Ingest["Приём телеметрии"]
            PluginHub["Plugin Hub<br/>(в процессе)"]
            OTA["OTA Manager<br/>(Auto-OTA)"]
        end

        subgraph Services["Сервисы и хранилище"]
            Rules["Rules Bus<br/>publisher"]
            WS["WebSocket Hub<br/>(real-time)"]
            DB["SQLite<br/>(WAL mode)"]
            API["REST API<br/>(chi router)"]
            Auth["JWT / OAuth<br/>Auth"]
            MCP["MCP Endpoint<br/>/mcp"]
        end
    end

    subgraph External["Внешние клиенты"]
        UI["React Dashboard<br/>(embedded SPA)"]
        NR["Node-RED<br/>(опционально)"]
        LLM["LLM / AI Client<br/>(Cursor, Claude)"]
    end

    %% Устройство → Транспорт
    ESP1 & ESP2 & ESP3 -->|"MQTT TCP :1883<br/>MQTT WS :8083"| Broker

    %% Транспорт → Ядро
    Broker --> ACL
    ACL --> Ingest

    %% Ядро внутреннее
    Ingest --> PluginHub
    Ingest --> OTA

    %% Ядро → Сервисы
    Ingest --> DB
    Ingest --> WS
    Ingest --> Rules
    PluginHub -->|"InvokeRPC"| Broker
    OTA -->|"MQTT downlink<br/>v1/o/upd"| Broker

    %% Сервисы внутреннее
    API --> Auth
    API --> DB
    API --> WS
    API --> PluginHub
    MCP --> API

    %% Сервисы ↔ Внешние
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

## Архитектура внешних плагинов

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

## Поток событий внешнего плагина

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

## Поток тиков внешнего плагина

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

## Heartbeat внешнего плагина

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

## Жизненный цикл внешнего плагина

```mermaid
sequenceDiagram
    participant Pl as Плагин
    participant P as Платформа
    participant U as Пользователь
    participant D as Устройство

    Note over U: Админ создаёт плагин в UI
    U->>P: POST /api/v1/plugins/external
    P-->>U: {ok: true, token: "...", mqtt_user: "...", ...}
    U-->>Pl: передаёт токен и MQTT-креды плагину

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

## Архитектура внутренних плагинов

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

## Жизненный цикл внутреннего плагина

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
