# HA / Clustering — Архитектура

> **Версия документа**: 1.0 (черновик)
> **Версия платформы**: v2.x (см. [§6 План релиза](#6-план-релиза-ha))
> **Статус**: Черновик → Активен
> **Обновлено**: Сентябрь 2026
> **Область**: архитектура multi-instance deployment, RabbitMQ-based cluster, external plugin SDK
> **Каноническая версия**: этот документ ведётся на русском; английский будет добавлен после стабилизации

Этот документ описывает **HA/clustering** архитектуру платформы — multi-instance deployment для горизонтального масштабирования. Документ является **продолжением** [16-multi-account.md](16-multi-account.md), который описывает single-instance режим (v1.0..v1.6).

**Стратегия:** сначала полностью завершаем v1.x (single mode), затем фокусируемся на v2.x (HA). Миграция single → cluster не предполагается — это **альтернативные deployment options**.

Связанные доки: [01-prd](01-prd.md), [02-api](02-api.md), [04-mqtt](04-mqtt.md), [07-internal-plugins](07-internal-plugins.md), [08-external-plugins](08-external-plugins.md), [16-multi-account](16-multi-account.md).

---

> **Диаграммы** (Mermaid): [HA Architecture](14-diagrams.md#ha-architecture-overview), [Single vs Cluster](14-diagrams.md#single-vs-cluster), [RabbitMQ Topology](14-diagrams.md#rabbitmq-topology), [Telemetry Flow](14-diagrams.md#telemetry-flow-cluster-mode), [Command Flow](14-diagrams.md#command-flow-cluster-mode), [SSE Event Flow](14-diagrams.md#sse-event-flow), [Plugin SDK Architecture](14-diagrams.md#external-plugin-sdk-architecture). Канонические диаграммы — в [14-diagrams.md](14-diagrams.md).

---

## 0. TL;DR

- **HA = альтернативный deployment**, не миграция с single. Single mode остаётся полностью поддерживаемым.
- **3 бинарника** в одном monorepo:
  - `platform-single` — single mode (v1.x, как сейчас)
  - `platform-cluster` — UI/API/WebSocket (SSE) + Prometheus + RabbitMQ client
  - `platform-worker` — stateless execution of internal plugins
- **RabbitMQ** — обязательный middleware для cluster mode: MQTT broker для устройств, AMQP для inter-instance и external plugins
- **PostgreSQL** — обязательная БД (SQLite не поддерживается в cluster)
- **SSE вместо WebSocket** в cluster mode — проще, HTTP-based, не требует sticky sessions
- **External Plugin SDK** — единый API; Transport выбирается режимом деплоя: `mqtt` в single, `amqp` в cluster. Один код плагина работает в обоих режимах.
- **Device registry** (`device_instances` table) — для видимости device → instance mapping
- **Встроенный MQTT broker остаётся** в single mode; в cluster не используется

---

## 1. Архитектурный обзор

### 1.1 Два deployment option'а

| Аспект | Single Mode (v1.x) | Cluster Mode (v2.x) |
|--------|---------------------|----------------------|
| **Бинарник** | `platform-single` | `platform-cluster` + `platform-worker` |
| **MQTT broker** | Встроенный (`mochi-mqtt`) | RabbitMQ (с MQTT plugin) |
| **БД** | SQLite или PostgreSQL | Только PostgreSQL |
| **External plugins** | Только MQTT | Только AMQP (RabbitMQ) |
| **Real-time UI** | WebSocket | SSE (Server-Sent Events) |
| **Internal plugins** | In-process | В workers (stateless) |
| **Dependencies** | Только БД | RabbitMQ + PostgreSQL |
| **Use case** | Малые/средние деплои (≤1k devices) | Большие деплои (1k+ devices) |

### 1.2 Monorepo структура

```
iot-platform-go/
├── cmd/
│   ├── platform-single/       # Single mode (v1.x)
│   ├── platform-cluster/      # Cluster instance (v2.x)
│   └── platform-worker/       # Worker для plugins (v2.x)
├── internal/                   # Общая бизнес-логика (~80%)
│   ├── api/                    # HTTP handlers (общий)
│   ├── auth/                   # JWT, sessions (общий)
│   ├── db/                     # Models, migrations (общий)
│   ├── plugins/                # Internal plugin interface (общий)
│   ├── services/               # Business logic (общий)
│   ├── models/                 # Data models (общий)
│   ├── config/                 # Config loading (общий)
│   └── transport/              # Transport abstraction
│       ├── mqtt/               # MQTT implementation
│       ├── amqp/               # AMQP implementation
│       └── grpc/               # gRPC implementation
├── pkg/
│   └── plugin-sdk/             # External plugin SDK (публичный)
├── web/                        # React SPA (общий)
├── docs/
└── go.mod                      # Один go.mod
```

**Без build tags** — обычные Go packages, разные бинарники через разные `cmd/` директории.

### 1.3 HA architecture overview

```mermaid
graph TB
    subgraph Clients["Клиенты"]
        U1["UI (браузер)"]
        D1["Device A"]
        D2["Device B"]
        EP1["External Plugin<br/>(AMQP)"]
    end

    subgraph Edge["L7 Load Balancer"]
        LB["LB<br/>(round-robin)"]
    end

    subgraph Cluster["Cluster instances (stateless)"]
        CI_A["platform-cluster A<br/>UI/API + SSE + Prometheus"]
        CI_B["platform-cluster B<br/>UI/API + SSE + Prometheus"]
    end

    subgraph Workers["Workers (stateless)"]
        W1["platform-worker 1<br/>internal plugins"]
        W2["platform-worker 2<br/>internal plugins"]
    end

    subgraph Bus["RabbitMQ (обязательный)"]
        MQ_MQTT["MQTT plugin<br/>:1883"]
        MQ_AMQP["AMQP<br/>:5672"]
    end

    subgraph DB["PostgreSQL (обязательный)"]
        PG[("platform DB")]
        DI[("device_instances<br/>(device → instance)")]
    end

    U1 -->|"HTTPS / SSE"| LB
    LB --> CI_A
    LB --> CI_B
    
    D1 -->|"MQTT"| MQ_MQTT
    D2 -->|"MQTT"| MQ_MQTT
    EP1 -->|"AMQP"| MQ_AMQP

    MQ_MQTT -->|"iot.devices"| MQ_AMQP
    MQ_AMQP -->|"iot.cluster.events"| CI_A
    MQ_AMQP -->|"iot.cluster.events"| CI_B
    MQ_AMQP -->|"iot.events"| W1
    MQ_AMQP -->|"iot.events"| W2

    CI_A -->|"command routing"| DI
    CI_B -->|"command routing"| DI
    CI_A -->|"HTTP API"| PG
    CI_B -->|"HTTP API"| PG
    W1 --> PG
    W2 --> PG

    W1 -->|"publish cmd"| MQ_AMQP
    W2 -->|"publish cmd"| MQ_AMQP
    MQ_AMQP -->|"downlink"| MQ_MQTT

    style Clients fill:#16213e,stroke:#1a1a2e,color:#eee
    style Edge fill:#0f3460,stroke:#16213e,color:#eee
    style Cluster fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Workers fill:#16213e,stroke:#1a1a2e,color:#eee
    style Bus fill:#0f3460,stroke:#16213e,color:#eee
    style DB fill:#16213e,stroke:#1a1a2e,color:#eee
```

### 1.4 Single vs Cluster (сравнение)

```mermaid
graph LR
    subgraph Single["Single Mode (v1.x)"]
        S_DEV["Device"] -->|"MQTT"| S_BROKER["Встроенный<br/>mochi-mqtt"]
        S_BROKER --> S_APP["platform-single<br/>(всё in-process)"]
        S_APP --> S_DB[("SQLite/Postgres")]
        S_UI["UI"] -->|"WebSocket"| S_APP
        S_EP["External Plugin"] -->|"MQTT"| S_BROKER
    end

    subgraph Cluster["Cluster Mode (v2.x)"]
        C_DEV["Device"] -->|"MQTT"| C_MQ["RabbitMQ"]
        C_MQ -->|"AMQP"| C_CI["platform-cluster<br/>UI/API + SSE"]
        C_MQ -->|"AMQP"| C_W["platform-worker<br/>internal plugins"]
        C_CI --> C_PG[("PostgreSQL")]
        C_W --> C_PG
        C_UI["UI"] -->|"SSE"| C_CI
        C_EP["External Plugin"] -->|"AMQP"| C_MQ
    end

    style Single fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Cluster fill:#16213e,stroke:#0f3460,color:#eee
```

**Сравнение SDK по режимам:**

| Аспект | Single Mode | Cluster Mode |
|--------|-------------|--------------|
| **External plugin transport** | `mqtt` | `amqp` |
| **Broker** | Встроенный `mochi-mqtt` | RabbitMQ (AMQP) |
| **Примеры в `examples/`** | C++, Go, Java, Python (MQTT) — все работают | Go (AMQP) — нужно добавить |

> Один и тот же код плагина (через SDK) работает в обоих режимах. Разработчик пишет `p.Subscribe("events.#", ...)` один раз — SDK сам выбирает нужный transport и формат топика.

---

## 2. Компоненты

### 2.1 RabbitMQ (обязательный)

**Роль:**
- MQTT broker для устройств (через RabbitMQ MQTT plugin)
- AMQP для inter-instance communication
- AMQP для external plugins
- Queue для команд (routing to target instance)

**Топология:**

| Exchange | Type | Назначение |
|----------|------|------------|
| `iot.devices` | topic | Telemetry от устройств (routing по `account_slug.product_id.device_id.kind`) |
| `iot.commands` | direct | Команды устройствам (routing по `instance_id`) |
| `iot.cluster` | fanout | Inter-instance events (все instance'ы получают) |
| `iot.external` | topic | События для external plugins (routing по `account_slug.#`) |

**Queues per instance:**
- `iot.instance.{instance_id}.commands` — команды для конкретного instance
- `iot.cluster.events.{instance_id}` — events для instance (для SSE)

**RabbitMQ topology:**

```mermaid
graph TB
    subgraph Producers["Producers"]
        D1["Device A"]
        D2["Device B"]
        EP1["External Plugin"]
        W1["Worker 1"]
        W2["Worker 2"]
    end

    subgraph Exchanges["RabbitMQ Exchanges"]
        EX_DEV["iot.devices<br/>(topic)"]
        EX_CMD["iot.commands<br/>(direct)"]
        EX_CLU["iot.cluster<br/>(fanout)"]
        EX_EXT["iot.external<br/>(topic)"]
    end

    subgraph Queues["Queues (per instance/worker)"]
        Q_CI_A["iot.instance.cluster-A.commands"]
        Q_CI_B["iot.instance.cluster-B.commands"]
        Q_W1["iot.worker.worker-1.events"]
        Q_W2["iot.worker.worker-2.events"]
        Q_EXT["iot.external.consumers"]
    end

    subgraph Consumers["Consumers"]
        CI_A["platform-cluster A"]
        CI_B["platform-cluster B"]
        WORK1["platform-worker 1"]
        WORK2["platform-worker 2"]
        EXTC["External Plugin<br/>(AMQP client)"]
    end

    D1 -->|"iot/v1/..."| EX_DEV
    D2 -->|"iot/v1/..."| EX_DEV
    
    W1 -->|"commands.instance.cluster-A"| EX_CMD
    W2 -->|"commands.instance.cluster-B"| EX_CMD
    
    EX_DEV -->|"fanout"| EX_CLU
    EX_DEV -->|"routing key"| EX_EXT
    
    EX_CLU --> Q_CI_A
    EX_CLU --> Q_CI_B
    EX_EXT --> Q_EXT
    EX_CMD --> Q_CI_A
    EX_CMD --> Q_CI_B
    EX_DEV --> Q_W1
    EX_DEV --> Q_W2
    
    Q_CI_A --> CI_A
    Q_CI_B --> CI_B
    Q_W1 --> WORK1
    Q_W2 --> WORK2
    Q_EXT --> EXTC
    
    EP1 -->|"publish"| EX_CMD

    style Producers fill:#16213e,stroke:#1a1a2e,color:#eee
    style Exchanges fill:#0f3460,stroke:#16213e,color:#eee
    style Queues fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Consumers fill:#16213e,stroke:#1a1a2e,color:#eee
```

### 2.2 PostgreSQL (обязательный)

**Роль:**
- Shared state для cluster instances
- Device registry: `device_instances` (device → instance mapping)
- Все данные (telemetry, devices, users, etc.)

**Таблица device_instances:**

```sql
CREATE TABLE device_instances (
    device_pk      BIGINT PRIMARY KEY,
    instance_id    VARCHAR(64) NOT NULL,
    mqtt_client_id VARCHAR(128),
    connected_at   TIMESTAMPTZ,
    last_seen_at   TIMESTAMPTZ
);

CREATE INDEX idx_device_instances_instance ON device_instances(instance_id);
```

**Обновление:**
- Cluster instance обновляет запись при device connect/disconnect через RabbitMQ MQTT plugin events
- TTL: запись удаляется через N минут после `last_seen_at` (с помощью `last_seen_at` cursor в `iot.devices` exchange)

**Зачем нужна таблица:**

| Use case | Описание |
|----------|----------|
| **Command routing** | AMQP topic exchange умеет роутить по routing key, **но не знает**, какой instance держит MQTT-соединение с device. Поэтому lookup в `device_instances` — единственный способ узнать, в чью очередь (`iot.instance.{id}.commands`) положить команду |
| **UI visibility** | UI показывает "device X подключён к instance Y" — полезно для диагностики и балансировки |
| **Health monitoring** | Prometheus метрика `iot_device_instance_age_seconds` — как давно device был активен на текущем instance |

**Без этой таблицы:**
- ❌ Каждый instance должен был бы подписываться на **все** команды для **всех** device'ов
- ❌ Каждый instance пытался бы отправить команду, и только один получал бы PUBACK
- ❌ Лишняя нагрузка на RabbitMQ + дублирование трафика

**Альтернативы (отклонены):**
- ❌ "Header-based routing" в AMQP — сложно, не даёт преимуществ над прямым routing key
- ❌ "Sticky session в LB" — device reconnect может попасть на другой instance
- ❌ "Publish в общую очередь и пусть все instance'ы пробуют" — race condition, неэффективно

**Итого:** `device_instances` — обязательный registry, не оптимизация.

### 2.3 Cluster Instance (`platform-cluster`)

**Роль:**
- UI/API server (HTTP REST)
- SSE server (вместо WebSocket)
- Prometheus exporter (simple, no embedded TSDB)
- RabbitMQ client (consume events, publish commands)

**НЕ имеет:**
- Встроенного MQTT broker
- Internal plugins (они в workers)
- WebSocket server

**Stateless:** может быть запущено N instance'ов за L7 LB. Каждый instance обрабатывает свои HTTP-запросы и SSE-соединения.

**Конфигурация:**
```yaml
mode: cluster
http_addr: ":8080"
rabbitmq_url: "amqp://guest:guest@rabbitmq:5672/"
postgres_dsn: "postgres://user:pass@postgres:5432/platform"
instance_id: "${HOSTNAME}"  # автогенерация
```

### 2.4 Worker (`platform-worker`)

**Роль:**
- Запускает internal plugins (schedule, led-garland, и т.д.)
- Stateless execution (без leader election)
- Может быть запущено N workers

**Архитектурно: worker = "SDK-consumer + in-process plugin handlers"**

> Концептуально worker не отличается от external plugin, который:
> 1. Подписывается на `iot.external` exchange через SDK
> 2. Обрабатывает event'ы
> 3. Публикует команды в `iot.commands`
>
> Единственное отличие: "код плагина" у worker'а — это **Go-пакеты, скомпилированные в бинарник** (in-process), а не отдельный процесс. Всё остальное — такое же.

**Функции:**
- Подписан на RabbitMQ events (через тот же Plugin SDK)
- Запускает internal plugins как handlers (in-process)
- Публикует команды обратно в RabbitMQ

**Stateless execution:**
- Каждый worker получает все events
- Обрабатывает свою часть (например, schedule plugin срабатывает на каждом worker — но **только** если у плагина есть built-in dedup через unique constraints в БД)
- Нет координации между workers

**Сравнение:**

| Аспект | External plugin | Worker + internal plugin |
|--------|-----------------|--------------------------|
| Транспорт | AMQP через SDK | AMQP через SDK (тот же) |
| Код | Отдельный процесс | Go-пакет в бинарнике worker'а |
| Subscribe events | ✓ | ✓ |
| Publish commands | ✓ | ✓ |
| Язык | Любой (Go, Python, C++, Java) | Только Go |
| Deployment | Отдельно (systemd, docker) | Вместе с `platform-worker` |
| Lifecycle | Start/stop/restart отдельно | Вместе с worker'ом |

**Конфигурация:**
```yaml
mode: worker
rabbitmq_url: "amqp://guest:guest@rabbitmq:5672/"
postgres_dsn: "postgres://user:pass@postgres:5432/platform"
worker_id: "${HOSTNAME}"
internal_plugins:
  - schedule
  - led-garland
  - prometheus-exporter  # опционально
```

### 2.5 External Plugins (только AMQP)

**Транспорт:** только AMQP через RabbitMQ

**Подключение:**
- AMQP client подключается к RabbitMQ
- Подписывается на exchange `iot.external` с routing key `events.{account_slug}.#`
- Публикует команды в exchange `iot.commands` с routing key `commands.{account_slug}.{product}.{device}.{kind}`

**Разработка плагинов:**
- Используют `pkg/plugin-sdk` (публичный Go модуль)
- SDK абстрагирует transport — код плагина одинаковый для всех режимов

---

## 3. Потоки данных

### 3.1 Telemetry flow

```mermaid
sequenceDiagram
    participant D as Device
    participant MQ as RabbitMQ<br/>(MQTT plugin)
    participant EX as Exchange<br/>iot.devices
    participant Q as Queue<br/>iot.cluster.events
    participant CIA as Cluster A
    participant CIB as Cluster B
    participant W1 as Worker 1
    participant W2 as Worker 2
    participant U1 as SSE client A
    participant U2 as SSE client B

    D->>MQ: PUBLISH iot/v1/{product}/{device}/telemetry
    MQ->>EX: пересылает в exchange
    EX->>Q: routing по account_slug
    Q->>CIA: fanout (instance A)
    Q->>CIB: fanout (instance B)
    Q->>W1: routing по worker
    Q->>W2: routing по worker
    
    CIA->>CIA: фильтр по account_id пользователя
    CIA->>U1: SSE event
    CIB->>CIB: фильтр по account_id пользователя
    CIB->>U2: SSE event
    
    W1->>W1: run internal plugins
    W2->>W2: run internal plugins
```

**RabbitMQ fanout:** все cluster instances получают все events → каждый фильтрует по account_id пользователя → отправляет через SSE.

**Worker coordination:** каждый worker получает все events → обрабатывает свою часть (stateless).

### 3.2 Command flow

```mermaid
sequenceDiagram
    participant U as UI/API
    participant CIB as Cluster B
    participant DB as PostgreSQL<br/>device_instances
    participant EX as Exchange<br/>iot.commands
    participant Q as Queue<br/>iot.instance.cluster-A.commands
    participant CIA as Cluster A
    participant MQ as RabbitMQ<br/>(MQTT plugin)
    participant D as Device

    U->>CIB: POST /api/v1/devices/{id}/rpc
    CIB->>DB: SELECT device_instances<br/>WHERE device_pk=?
    DB-->>CIB: instance_id = "cluster-A"
    CIB->>EX: PUBLISH routing_key=commands.instance.cluster-A
    EX->>Q: routing по instance_id
    Q->>CIA: deliver to instance A
    CIA->>MQ: PUBLISH iot/v1/{product}/{device}/rpc/{method}
    MQ->>D: downlink (через MQTT)
    D-->>MQ: PUBACK
    MQ-->>CIA: подтверждение
    CIA-->>CIB: status (через отдельный канал)
    CIB-->>U: 200 OK
```

**Routing key:** `commands.instance.{instance_id}` — RabbitMQ routing направляет команду в нужный instance.

### 3.3 Event flow (SSE)

```mermaid
sequenceDiagram
    participant U as UI (браузер)
    participant CI as Cluster Instance
    participant MQ as RabbitMQ
    participant DB as PostgreSQL

    Note over U,DB: Установка SSE-соединения
    U->>CI: GET /api/v1/events/stream<br/>(Authorization: Bearer JWT)
    CI->>DB: SELECT memberships WHERE user_id=?
    DB-->>CI: account_ids пользователя
    CI->>MQ: subscribe to events.{account_slug}.#<br/>для всех accounts пользователя
    CI-->>U: 200 OK<br/>text/event-stream
    
    Note over U,DB: Передача событий
    MQ->>CI: event {account_slug, device_id, kind}
    CI->>CI: фильтр по account_id пользователя
    alt event для account пользователя
        CI->>U: data: {event JSON}\n\n
    else event для чужого account
        CI->>CI: skip
    end
    
    Note over U,DB: Auto-reconnect при разрыве
    U->>CI: GET /api/v1/events/stream<br/>(Last-Event-ID: ...)
    CI->>MQ: resubscribe
    CI-->>U: 200 OK
```

**Преимущества SSE:**
- HTTP-based (нет sticky session)
- Auto-reconnect (браузер)
- Проще WebSocket
- One-way (достаточно для events)

---

## 4. External Plugin SDK

### 4.1 Пакет

`pkg/plugin-sdk` — публичный Go модуль, можно `go get github.com/iot-platform/plugin-sdk`.

### 4.2 API

```go
package plugin

type Plugin interface {
    // Subscribe to events
    Subscribe(pattern string, handler EventHandler) error
    
    // Publish command to device
    Publish(topic string, payload []byte) error
    
    // Run plugin (blocks)
    Run() error
}

type Config struct {
    Transport   string  // "amqp" (cluster mode) или "mqtt" (single mode)
    Connection  string  // AMQP или MQTT connection string
    AccountID   int64
    AccountSlug string
    PluginID    string
    Token       string
    Logger      Logger
    Metrics     Metrics
}

func New(cfg Config) (Plugin, error)
```

**Маппинг Transport → подключение:**

| `Transport` | Когда использовать | Connection string format |
|-------------|---------------------|--------------------------|
| `mqtt` | Single mode (встроенный `mochi-mqtt`) | `mqtt://user:pass@host:1883/` |
| `amqp` | Cluster mode (RabbitMQ) | `amqp://user:pass@host:5672/` |

**Не поддерживаются (удалены в v2.0):**
- ❌ `grpc` — заменён на AMQP для единообразия
- ❌ `webhook` — заменён на AMQP (plugin подключается сам, без NAT traversal проблем)
- ❌ `polling` — заменён на AMQP (push вместо poll)

**Правило:** Transport выбирается режимом деплоя, не разработчиком плагина:
- Single mode → `mqtt`
- Cluster mode → `amqp`
- Один и тот же код плагина работает в обоих режимах через SDK.

### 4.3 Использование

```go
import "github.com/iot-platform/plugin-sdk"

func main() {
    // Cluster mode
    p, _ := plugin.New(plugin.Config{
        Transport:   "amqp",
        Connection:  os.Getenv("IOT_CONNECTION"),
        AccountID:   42,
        AccountSlug: "factory-1",
        PluginID:    "telemetry-logger",
        Token:       os.Getenv("IOT_TOKEN"),
    })
    
    // Subscribe (паттерн одинаковый для amqp/mqtt)
    p.Subscribe("events.{account_slug}.{product_id}.{device_id}.#", func(e Event) {
        log.Printf("event: %s", e.Kind)
        // handle event
    })
    
    // Publish
    p.Publish("commands.{account_slug}.{product_id}.{device_id}.rpc", payload)
    
    p.Run()
}
```

### 4.4 Transport implementations

| Transport | Topic → Routing Key | Protocol | Где работает |
|-----------|---------------------|----------|--------------|
| **MQTT** | `platform/v1/events/{slug}/{product}/{device}/{kind}` | MQTT pub/sub | Single (встроенный broker) |
| **AMQP** | exchange `iot.external` + routing key `events.{slug}.{product}.{device}.{kind}` | AMQP pub/sub | Cluster (RabbitMQ) |

**SDK обрабатывает:**
- Connection management (reconnect, heartbeat)
- Topic/routing key translation (per transport)
- Serialization (JSON, protobuf)
- Error handling
- Metrics

**Преимущества:**
- External plugin developer пишет код **один раз**
- SDK абстрагирует transport (но не скрывает — `Transport` явный в Config)
- Легко добавить новый transport (только реализация в SDK)

**❌ Удалённые транспорты в v2.0:**

| Транспорт | Почему удалён | Замена |
|-----------|---------------|--------|
| `gRPC` | Дублирует AMQP функциональность; усложняет SDK (нужны отдельные proto-файлы) | AMQP |
| `webhook` (platform → plugin) | Требует публичный IP/DNS у плагина; NAT traversal проблема | AMQP (plugin подключается сам) |
| `polling` (plugin → platform) | Неэффективно (latency = poll interval); лишняя нагрузка на API | AMQP (push) |

**Результат:** SDK проще, у разработчика плагина один выбор — `mqtt` (single) или `amqp` (cluster). Transport диктуется режимом деплоя.

### 4.5 SDK architecture

```mermaid
graph TB
    subgraph Plugin["External Plugin (код разработчика)"]
        CODE["p.Subscribe(pattern, handler)<br/>p.Publish(topic, payload)"]
    end

    subgraph SDK["Plugin SDK (pkg/plugin-sdk)"]
        API["API layer<br/>(Subscribe/Publish/Run)"]
        TRANS["Transport abstraction"]
        SER["Serialization<br/>(JSON/protobuf)"]
        CONN["Connection management<br/>(reconnect, heartbeat)"]
    end

    subgraph Impl["Transport implementations"]
        MQTT["MQTT transport<br/>(single)"]
        AMQP["AMQP transport<br/>(cluster)"]
    end

    subgraph Brokers["Brokers"]
        MB["Встроенный<br/>mochi-mqtt<br/>(single)"]
        RB["RabbitMQ<br/>(cluster)"]
    end

    CODE --> API
    API --> TRANS
    TRANS --> SER
    TRANS --> CONN
    TRANS --> MQTT
    TRANS --> AMQP
    MQTT --> MB
    AMQP --> RB

    style Plugin fill:#16213e,stroke:#1a1a2e,color:#eee
    style SDK fill:#0f3460,stroke:#16213e,color:#eee
    style Impl fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Brokers fill:#16213e,stroke:#1a1a2e,color:#eee
```

---

## 5. Функциональные требования (HA-specific)

### 5.1 F2-01..F2-07: HA Foundation (v2.0)

| ID | Требование | Приоритет |
|----|-----------|-----------|
| F2-01 | **Cluster instance** (`platform-cluster`): UI/API + SSE + Prometheus + RabbitMQ client. Без встроенного MQTT | P0 |
| F2-02 | **Worker** (`platform-worker`): stateless execution of internal plugins. Подписан на RabbitMQ events | P0 |
| F2-03 | **RabbitMQ integration**: подключение к RabbitMQ, consume events, publish commands. Auto-reconnect, heartbeat | P0 |
| F2-04 | **Device registry**: таблица `device_instances`. Обновление при MQTT connect/disconnect через RabbitMQ MQTT plugin events | P0 |
| F2-05 | **Command routing**: API → lookup device_instances → publish в `iot.commands` с routing key `commands.instance.{instance_id}` | P0 |
| F2-06 | **SSE endpoint**: `GET /api/v1/events/stream` — HTTP-based, не требует sticky session. Auto-reconnect | P0 |
| F2-07 | **Prometheus exporter**: простой exporter без embedded TSDB. Метрики с лейблом `instance_id` | P1 |

> **Примечание:** F2-11..F2-15 (`rabbitmq-bridge` plugin) **удалены** — см. §7.6. External apps подключаются к RabbitMQ напрямую через SDK.

### 5.2 F2-21..F2-26: Account-level features (v2.2)

| ID | Требование | Приоритет |
|----|-----------|-----------|
| F2-21 | **Custom RBAC**: per-account кастомные роли поверх встроенных admin/operator/viewer | P1 |
| F2-22 | **Per-account API rate limiting**: token bucket в PostgreSQL (без Redis) | P1 |
| F2-23 | **Per-account retention override**: отдельные retention для telemetry/events/audit | P1 |
| F2-24 | **Account import/export (JSON)**: полный дамп products/devices/credentials в JSON | P1 |
| F2-25 | **Webhook signing per-account**: HMAC-SHA256 в заголовке `X-Cogitor-Signature` | P2 |
| F2-26 | **Per-account Prometheus remote_write**: отправка метрик в account-specific Prometheus | P2 |

### 5.3 F2-31..F2-36: Future (v2.3+)

| ID | Требование | Заметка |
|----|-----------|--------|
| F2-31 | **SSO providers per account** (OIDC, SAML) | Заменяет local auth для конкретного account'а |
| F2-32 | **Audit log streaming в SIEM** (Kafka, Loki, Splunk) | Через отдельный sink |
| F2-33 | **Account transfer / merge** | Сложный flow с подтверждением |
| F2-34 | **Cross-account dashboards** | Только для platform-admin |
| F2-35 | **Multi-region / DR** (Postgres streaming replication) | Требует F2-05 (object storage) |
| F2-36 | **Plugin marketplace** (подписанные каталожные плагины) | Отдельный реестр |

### 5.4 Принцип «не делаем» (явный список)

Эти идеи **отвергнуты** после обсуждения:

- ❌ **Multi-DB isolation (отдельная БД на аккаунт)** — operational nightmare
- ❌ **Per-account sub-domain** — DNS-конфигурация лежит на пользователе
- ❌ **Custom CSS / white-label** для account'а — out of product scope
- ❌ **Account-level плагин-каталог** — пока каталожные плагины общие
- ❌ **Embedded NATS** в platform binary — добавляет complexity, отвергнут
- ❌ **WebSocket в cluster mode** — заменён на SSE (проще, HTTP-based)
- ❌ **Redis как middleware** — отвергнут, RabbitMQ покрывает все needs
- ❌ **rabbitmq-bridge plugin** — отвергнут (см. §7.6). External apps подключаются к RabbitMQ напрямую через SDK
- ❌ **gRPC транспорт для external plugins** — отвергнут (см. §7.5). Будет удалён в v1.1
- ❌ **webhook/polling транспорты** — отвергнуты, AMQP покрывает все use case'ы

---

## 6. План релиза HA

### 6.1 Этапы

| Версия | Тема | Ключевые F-ID | Exit-критерий |
|--------|------|---------------|----------------|
| **v2.0** | **HA Foundation** | F2-01..F2-07 | 2 cluster instance + 1 worker за L7 LB; device подключён к instance A, команда от instance B доходит до device; SSE работает; Prometheus scrape'ит оба instance'а |
| **v2.1** | **rabbitmq-bridge** | F2-11..F2-15 | Плагин публикует события в RabbitMQ exchange; sample consumer получает live-события; backpressure работает |
| **v2.2** | **Account-level features** | F2-21..F2-26 | Custom RBAC работает; per-account rate limiting; account import/export round-trip идентичен |
| **v2.3+** | **Future** | F2-31..F2-36 | Multi-region DR; SSO per account; SIEM integration |

### 6.2 Правила перехода

1. **v1.x полностью завершён** до начала v2.0 (single mode работает стабильно)
2. **Каждый этап HA** заканчивается review-чекпоинтом (PR с меткой `ha/v2.x`) и записью в CHANGELOG
3. **Документация обновляется** в том же PR, что и код
4. **Никакого carry-over**: если что-то не вошло — backlog, не блокирует релиз
5. **v1.x single mode** остаётся полностью поддерживаемым параллельно с v2.x

### 6.3 Стратегия разработки

**Сначала single, потом HA:**

1. **v1.0..v1.6** — single mode (multi-account, monitoring, polish)
2. **После v1.6** — retro, фиксация архитектурных решений
3. **v2.0** — HA foundation (cluster instance + worker + RabbitMQ)
4. **v2.1+** — HA-specific features (rabbitmq-bridge, custom RBAC, etc.)

**Нет миграции single → cluster** — это альтернативные deployment options. Пользователь выбирает при установке.

---

## 7. Backlog / Future

### 7.1 Simple migration utility (отложено)

**Идея:** простая утилита для миграции single → cluster (devices, users, default plugins enabled).

**Scope:**
- Export devices + users + plugins из single instance
- Import в cluster instance
- Enable default plugins

**Когда:** после v2.0, если появится спрос.

**Статус:** backlog (не в v2.0).

### 7.2 Git labels/tags для версионирования задач

**Идея:** использовать Git labels для маркировки задач по версиям (`v1.0`, `v1.1`, ..., `v2.0`, `v2.1`).

**Зачем:** упростить tracking, какая задача к какой версии относится.

**Когда:** после стабилизации v1.6, перед началом v2.0.

**Статус:** backlog.

### 7.3 Multi-region / DR

**Идея:** active-passive deployment через Postgres streaming replication.

**Зависимости:** F2-05 (object storage для OTA firmware), F2-26 (remote_write).

**Когда:** v2.3+.

### 7.4 Plugin marketplace

**Идея:** подписанные каталожные плагины, отдельный реестр.

**Когда:** v2.3+.

### 7.5 Удаление gRPC транспорта из примеров и кода

**Контекст:** §4 решил, что в v2.0 транспорт gRPC для external plugins **удалён** (заменён на AMQP). Удаляем gRPC **заранее в v1.1**, чтобы single mode был очищен до начала HA-разработки.

**Целевая версия:** **v1.1**

**Что удаляем:**

| Что | Действие |
|-----|----------|
| `internal/grpc/plugin/` | Удалить директорию целиком (`.proto` + `_grpc.go`) |
| `examples/grpc/` | Удалить директорию целиком |
| `08-external-plugins.md` | Убрать упоминания gRPC |
| `docs/external-plugin-prd.md` | Убрать упоминания gRPC |
| SDK `Config.Transport` | Запретить значение `"grpc"` |

**Нечего мигрировать:** существующих gRPC-плагинов в production нет — удаляем код без migration guide.

**Проверка перед merge:**
- [ ] Удалена директория `internal/grpc/plugin/`
- [ ] Удалена директория `examples/grpc/`
- [ ] SDK возвращает ошибку при `Transport: "grpc"`
- [ ] Документация очищена от gRPC
- [ ] CI passes (нет импортов на удалённые пакеты)

**Статус:** запланировано в v1.1.

### 7.6 rabbitmq-bridge plugin — РЕШЕНО: удалить

**Контекст:** §5.2 описывает `rabbitmq-bridge` как каталожный internal plugin, который публикует события в RabbitMQ exchange. Но в cluster mode (v2.0+) платформа **уже подключена к RabbitMQ** — plugin-мост не нужен.

**Решение:** ✅ **Удалить F2-11..F2-15** (см. §5.2). External apps подключаются к тому же RabbitMQ напрямую через SDK (Transport=`amqp`).

**Что делаем:**
- ❌ Не реализуем `internal/plugins/rabbitmq_bridge.go`
- ❌ Не добавляем F2-11..F2-15 в v2.1
- ❌ Удаляем §5.2 из release plan
- ❌ Удаляем упоминание `rabbitmq-bridge` из §8 (связанная документация)

**Альтернатива (если кому-то нужен forwarding во внешний broker):**
- Это не задача платформы. Используйте стандартный AMQP-клиент в external app + подписку на `iot.external` exchange.

**Статус:** ✅ решено.

---

## 8. Связанная документация

| Док | Что |
|-----|------|
| [16-multi-account.md](16-multi-account.md) | Single mode (v1.0..v1.6) — multi-account и связанные фичи |
| [01-prd.md](01-prd.md) | Общий продуктовый PRD |
| [02-api.md](02-api.md) | REST API reference (будет дополнен HA endpoints) |
| [04-mqtt.md](04-mqtt.md) | MQTT-протокол (RabbitMQ MQTT plugin) |
| [07-internal-plugins.md](07-internal-plugins.md) | Internal plugins (только in-process в v1.x; в workers в v2.x) |
| [08-external-plugins.md](08-external-plugins.md) | External plugins (MQTT в single, AMQP в cluster) |
| [11-operations.md](11-operations.md) | Эксплуатация (cluster deployment) |
| [14-diagrams.md](14-diagrams.md) | Диаграммы (HA architecture будет добавлена) |

---

## 9. Changelog

- **v1.0** (Сентябрь 2026) — первая версия документа. Создан после обсуждения архитектуры HA/clustering (RabbitMQ, PostgreSQL, 3 бинарника, SSE вместо WebSocket, External Plugin SDK).
