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

## Архитектура Multi-Account

> Источник: [16-multi-account.md](16-multi-account.md#71-общая-схема)

Высокоуровневая схема релиза: общий брокер с per-username ACL, account-scope ресурсы, каталожные плагины (включая новые `prometheus-exporter` и `metrics-dashboard`), и таблица per-account политик для каталожных плагинов.

```mermaid
graph TB
    subgraph Clients["Клиенты"]
        U1["Пользователь A<br/>(admin в acme)"]
        U2["Пользователь B<br/>(operator в beta, viewer в acme)"]
        P1["External Plugin<br/>acme/telegram-bot"]
        P2["External Plugin<br/>beta/zapier"]
    end

    subgraph Platform["Cogitor IoT Platform (single binary)"]
        subgraph Auth["Auth и Tenancy"]
            JWT["JWT<br/>(active_account_id)"]
            IAM["AccountMembership<br/>Middleware"]
        end

        subgraph Broker["MQTT Broker"]
            B1["mochi-mqtt<br/>per-username ACL"]
        end

        subgraph Core["Ядро"]
            ACL["Device ACL<br/>(product.device)"]
            ING["Ingest + publishRules"]
            HUB["PluginHub<br/>(per-account policy)"]
        end

        subgraph Catalog["Каталожные плагины"]
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

    %% Клиенты → Auth
    U1 & U2 -->|"Bearer JWT"| JWT
    P1 & P2 -->|"Bearer token"| JWT

    %% Auth → Core
    JWT --> IAM
    IAM --> ACL
    IAM --> ING
    IAM --> HUB

    %% Брокер
    B1 --> ACL
    ACL --> ING
    ING --> HUB
    HUB --> LG & SCH & PRO & MD
    ING --> EV
    CM --> B1

    %% DB
    IAM --> MEM
    IAM --> ACC
    IAM --> INV
    IAM --> POL
    ING --> DV
    ING --> PR
    HUB --> POL

    %% Внешние плагины → брокер
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

## Поток приглашений

> Источник: [16-multi-account.md](16-multi-account.md#53-приглашения)

Полный цикл приглашения: admin создаёт, получатель принимает (залогинен или свежая регистрация), membership появляется, JWT несёт новый active account.

```mermaid
sequenceDiagram
    participant A as Admin (UI)
    participant P as Платформа
    participant M as Mail (SMTP/log)
    participant R as Получатель
    participant DB as SQLite

    Note over A: Активный аккаунт = acme
    A->>P: POST /api/v1/accounts/{id}/invitations {email, role}
    P->>DB: INSERT account_invitations (token_hash, expires_at = +72h)
    P-->>A: {token: "raw-once", link: "/invite/{token}"}
    P->>M: отправить email со ссылкой
    M-->>R: письмо доставлено

    R->>P: GET /api/v1/invitations/{token}
    P->>DB: поиск по token_hash
    P-->>R: {account_name: "acme", role: "operator"}

    alt Получатель уже залогинен
        R->>P: POST /invitations/{token}/accept (с JWT)
        P->>DB: INSERT account_memberships(user_id, account_id, role)
        P-->>R: {ok: true, account_id}
        R->>P: POST /auth/switch-account {account_id}
        P-->>R: {token: новый JWT с active=acme}
    else Новый пользователь
        R->>P: GET /auth/register?invite=...&email=...
        R->>P: POST /auth/register (email подставлен, создан user)
        R->>P: POST /invitations/{token}/accept (с новым JWT)
        P->>DB: INSERT account_memberships
        P-->>R: {ok: true, account_id}
    end

    Note over P: Resend / Revoke
    A->>P: POST /accounts/{id}/invitations/{id}/resend
    P->>DB: UPDATE old.status='revoked', INSERT new
    P->>M: отправить новое письмо
```

## Account ACL внешнего плагина

> Источник: [16-multi-account.md](16-multi-account.md#56-внешние-плагины-account-scoping)

Регистрация внешнего плагина, выдача MQTT-креда и ACL-проверка, запрещающая кросс-аккаунтный subscribe/publish.

```mermaid
sequenceDiagram
    participant Adm as Admin (UI)
    participant P as Платформа
    participant Pl as Внешний плагин
    participant Br as MQTT Broker

    Note over Adm: Активный аккаунт = acme
    Adm->>P: POST /api/v1/plugins/external {id: "telegram-bot"}
    P->>P: mqtt_user = "extplg_acme_telegram-bot"
    P->>P: mqtt_pass = random, token = random
    P->>P: хешируем, сохраняем в external_plugins (account_id=acme)
    P-->>Adm: {token, mqtt_user, mqtt_pass, mqtt_host, mqtt_port}
    Adm->>Pl: передаёт креды

    Pl->>Br: CONNECT mqtt_user=extplg_acme_telegram-bot, mqtt_pass=...
    Br->>P: OnConnectAuthenticate
    P->>P: ValidateMQTTCreds -> plugin.account_id=acme
    Br-->>Pl: CONNACK ok

    Note over Pl: Subscribe на свой аккаунт (OK)
    Pl->>Br: SUBSCRIBE platform/v1/events/acme/+/+/telemetry
    Br->>P: OnACLCheck(user, topic, write=false)
    P->>P: account_slug топика совпадает с plugin.account_id
    P-->>Br: allow
    Br-->>Pl: SUBACK

    Note over Pl: Subscribe на чужой аккаунт (DENY)
    Pl->>Br: SUBSCRIBE platform/v1/events/beta/+/+/telemetry
    Br->>P: OnACLCheck
    P->>P: topic slug "beta" != plugin.account_id
    P-->>Br: deny
    Br-->>Pl: SUBACK fail, "acl_deny"

    Note over Pl: Publish команды на свой device (OK)
    Pl->>Br: PUBLISH platform/v1/commands/acme/esp32-cam/cam-001/rpc
    Br->>P: OnACLCheck(user, topic, write=true)
    P-->>Br: allow
    P->>P: парсим команду, ищем device, проверяем device.account_id == plugin.account_id
    P-->>D: MQTT downlink RPC

    Note over Pl: Publish на чужой device (DENY)
    Pl->>Br: PUBLISH platform/v1/commands/beta/sensor/sensor-01/rpc
    Br->>P: OnACLCheck
    P-->>Br: deny
    Br-->>Pl: PUBACK fail
```

## Жизненный цикл Prometheus Exporter

> Источник: [16-multi-account.md](16-multi-account.md#59-внутренний-плагин-prometheus-exporter)

Как каталожный плагин `prometheus-exporter` поднимает HTTP-сервер, отдаёт Prometheus-метрики с лейблом `account` и останавливается, когда per-account policy отключает плагин.

```mermaid
sequenceDiagram
    participant Adm as Admin (UI)
    participant H as PluginHub
    participant EX as prometheus-exporter runner
    participant Pr as Prometheus (внешний)
    participant DB as SQLite

    Note over Adm: Settings → Integrations → prometheus-exporter enabled
    Adm->>H: PATCH /accounts/{id}/plugins/policies/prometheus-exporter {enabled:true}
    H->>DB: UPSERT account_plugin_policies
    H->>H: пересборка runners аккаунта

    Note over H: binding создан на устройстве (или hub стартует с существующим)
    Adm->>H: POST /devices/{d}/plugins {plugin_id: "prometheus-exporter"}
    H->>EX: ApplyConfig({port: 9100, bind: 0.0.0.0, bearer_token: ...})
    EX->>EX: http.Server.ListenAndServe на 0.0.0.0:9100

    loop каждые update_interval_ms
        EX->>DB: SELECT counts grouped by account/product/device/status
        EX->>EX: refresh atomic gauges
    end

    Pr->>EX: GET /metrics (Authorization: Bearer <token>)
    EX->>EX: проверка bearer (если задан)
    EX-->>Pr: 200 text/plain (Prometheus exposition, account=acme labels)
    EX->>EX: counter cogitor_device_telemetry_samples_total{account=acme,...}++ на StoreTelemetry (in-process hook)
    EX->>EX: counter cogitor_device_rpc_calls_total{...}++ на InvokeRPC

    Note over Adm: Отключаем exporter для аккаунта
    Adm->>H: PATCH ... {enabled:false}
    H->>H: перебор bindings аккаунта, остановка runners
    H->>EX: detach -> http.Server.Shutdown
    EX-->>Pr: connection refused (на следующем scrape)
```

## Policy внутренних плагинов

> Источник: [16-multi-account.md](16-multi-account.md#58-управление-внутренними-плагинами)

Как `account_plugin_policies` управляет каталожными плагинами per-account: dispatch, tick и очистка при удалении аккаунта.

```mermaid
flowchart TB
    subgraph Catalog["Каталожные плагины (init)"]
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

    subgraph Accounts["Аккаунты"]
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
    UI["Admin UI<br/>Settings → Integrations"] -->|"PATCH policy"| Policy

    style Catalog fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Policy fill:#0f3460,stroke:#16213e,color:#eee
    style Hub fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Accounts fill:#16213e,stroke:#1a1a2e,color:#eee
```
