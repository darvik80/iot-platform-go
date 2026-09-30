# HA / Clustering — Архитектура

| Поле | Значение |
|------|----------|
| **Версия документа** | 1.0 (черновик) |
| **Версия платформы** | v2.x (см. [§6 План релиза HA](#6-план-релиза-ha)) |
| **Статус** | Черновик → Активен |
| **Обновлено** | Сентябрь 2026 |
| **Область** | архитектура multi-instance deployment, RabbitMQ-based cluster, external plugin SDK |
| **Каноническая версия** | этот документ ведётся на русском; английский ([en/17-ha-architecture.md](../en/17-ha-architecture.md)) — сокращённое изложение основных разделов |

> ⚠️ **Проектный документ для v2.x — не реализован.** В текущем коде нет RabbitMQ/AMQP, нет SSE и нет кластеризации: один бинарник (`cmd/platform`) со встроенным брокером mochi-mqtt, real-time обновления идут через WebSocket `/api/v1/ws`, внешние плагины работают только по MQTT, `pkg/plugin-sdk`, `platform-cluster` и `platform-worker` отсутствуют. Текущее поведение — в [01-prd](01-prd.md) и [11-operations](11-operations.md).

Этот документ описывает **HA/clustering** архитектуру платформы — multi-instance deployment для горизонтального масштабирования. Документ является **продолжением** [16-multi-account.md](16-multi-account.md), который описывает single-instance режим (v1.0..v1.6).

**Стратегия:** сначала полностью завершаем v1.x (single mode), затем фокусируемся на v2.x (HA). Миграция single → cluster не предполагается — это **альтернативные deployment options**.

Связанные доки: [01-prd](01-prd.md), [02-api](02-api.md), [04-mqtt](04-mqtt.md), [07-internal-plugins](07-internal-plugins.md), [08-external-plugins](08-external-plugins.md), [16-multi-account](16-multi-account.md).

---

> **Диаграммы** (Mermaid): [HA Architecture](14-diagrams.md#ha-architecture-overview), [Single vs Cluster](14-diagrams.md#single-vs-cluster), [RabbitMQ Topology](14-diagrams.md#rabbitmq-topology), [Telemetry Flow](14-diagrams.md#telemetry-flow-cluster-mode), [Command Flow](14-diagrams.md#command-flow-cluster-mode), [SSE Event Flow](14-diagrams.md#sse-event-flow), [Plugin SDK Architecture](14-diagrams.md#external-plugin-sdk-architecture). Канонические диаграммы — в [14-diagrams.md](14-diagrams.md).

---

## 0. Краткое содержание

- **Кластер — это альтернативный способ развёртывания**, а не миграция с одиночного режима. Одиночный режим (single mode) остаётся полностью поддерживаемым.
- **Три бинарника** в одном репозитории:
  - `platform-single` — одиночный режим (v1.x, как сейчас)
  - `platform-cluster` — интерфейс, REST API, SSE (на базе HTTP, не WebSocket) + Prometheus + клиент RabbitMQ
  - `platform-worker` — выполнение внутренних плагинов без состояния
- **RabbitMQ** — обязательный компонент для кластера: брокер MQTT для устройств, AMQP для связи между экземплярами и для внешних плагинов
- **PostgreSQL** — обязательная база данных (SQLite не поддерживается в кластере)
- **SSE (Server-Sent Events)** для обновлений интерфейса в **обоих** режимах — на базе HTTP, не WebSocket, не требует закреплённых сессий, единая конечная точка `GET /api/v1/events/stream`
- **Внешние плагины** — единый SDK; транспорт выбирается режимом развёртывания: `mqtt` в одиночном режиме, `mqtt` или `amqp` в кластере (оба идут в RabbitMQ). Один код плагина работает в обоих режимах
- **Встроенный брокер MQTT** (`mochi-mqtt) остаётся в одиночном режиме; в кластере не используется — вместо него RabbitMQ с модулем MQTT

---

## 1. Архитектурный обзор

### 1.1 Два deployment option'а

| Аспект | Single Mode (v1.x) | Cluster Mode (v2.x) |
|--------|---------------------|----------------------|
| **Бинарник** | `platform-single` | `platform-cluster` + `platform-worker` |
| **MQTT broker** | Встроенный (`mochi-mqtt`) | RabbitMQ (с MQTT plugin) |
| **БД** | SQLite или PostgreSQL | Только PostgreSQL |
| **Внешние плагины** | Только MQTT (встроенный брокер) | MQTT или AMQP (через RabbitMQ) |
| **Real-time UI** | SSE (Server-Sent Events) | SSE (Server-Sent Events) |
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
        U1["Интерфейс (браузер)"]
        D1["Устройство A"]
        D2["Устройство B"]
        EP1["Внешний плагин<br/>(MQTT или AMQP)"]
    end

    subgraph Edge["Балансировщик L7"]
        LB["LB<br/>(round-robin)"]
    end

    subgraph Cluster["Экземпляры кластера (без состояния)"]
        CI_A["platform-cluster A<br/>Интерфейс/API + SSE + Prometheus"]
        CI_B["platform-cluster B<br/>Интерфейс/API + SSE + Prometheus"]
    end

    subgraph Workers["Рабочие процессы (без состояния)"]
        W1["platform-worker 1<br/>внутренние плагины"]
        W2["platform-worker 2<br/>внутренние плагины"]
    end

    subgraph Bus["RabbitMQ (обязательный)"]
        MQ_MQTT["Модуль MQTT<br/>:1883"]
        MQ_AMQP["AMQP<br/>:5672"]
    end

    subgraph DB["PostgreSQL (обязательный)"]
        PG[("база платформы")]
    end

    U1 -->|"HTTPS / SSE"| LB
    LB --> CI_A
    LB --> CI_B
    
    D1 -->|"MQTT"| MQ_MQTT
    D2 -->|"MQTT"| MQ_MQTT
    EP1 -->|"MQTT или AMQP"| MQ_AMQP

    MQ_MQTT -->|"iot.devices"| MQ_AMQP
    MQ_AMQP -->|"iot.cluster.events"| CI_A
    MQ_AMQP -->|"iot.cluster.events"| CI_B
    MQ_AMQP -->|"iot.events"| W1
    MQ_AMQP -->|"iot.events"| W2

    CI_A -->|"HTTP API"| PG
    CI_B -->|"HTTP API"| PG
    W1 --> PG
    W2 --> PG

    W1 -->|"публикация команд"| MQ_AMQP
    W2 -->|"публикация команд"| MQ_AMQP
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
    subgraph Single["Одиночный режим (v1.x)"]
        S_DEV["Устройство"] -->|"MQTT"| S_BROKER["Встроенный<br/>mochi-mqtt"]
        S_BROKER --> S_APP["platform-single<br/>(всё в процессе)"]
        S_APP --> S_DB[("SQLite/Postgres")]
        S_UI["Интерфейс"] -->|"SSE"| S_APP
        S_EP["Внешний плагин"] -->|"MQTT"| S_BROKER
    end

    subgraph Cluster["Кластер (v2.x)"]
        C_DEV["Устройство"] -->|"MQTT"| C_MQ["RabbitMQ"]
        C_MQ -->|"AMQP"| C_CI["platform-cluster<br/>Интерфейс/API + SSE"]
        C_MQ -->|"AMQP"| C_W["platform-worker<br/>внутренние плагины"]
        C_CI --> C_PG[("PostgreSQL")]
        C_W --> C_PG
        C_UI["Интерфейс"] -->|"SSE"| C_CI
        C_EP["Внешний плагин"] -->|"MQTT или AMQP"| C_MQ
    end

    style Single fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Cluster fill:#16213e,stroke:#0f3460,color:#eee
```

**Сравнение SDK по режимам:**

| Аспект | Одиночный режим | Кластер |
|--------|-----------------|---------|
| **Транспорт внешнего плагина** | `mqtt` (встроенный брокер) | `mqtt` (RabbitMQ MQTT) или `amqp` (RabbitMQ AMQP) |
| **Брокер** | Встроенный `mochi-mqtt` | RabbitMQ (порт MQTT :1883 или AMQP :5672) |
| **Примеры в `examples/`** | C++, Go, Java, Python (MQTT) — все работают | Go (AMQP) — нужно добавить |

> Один и тот же код плагина (через SDK) работает в обоих режимах. Разработчик пишет `p.Subscribe("events.#", ...)` один раз — SDK сам выбирает нужный транспорт и формат топика. В кластере плагин может подключиться как к MQTT-порту RabbitMQ (простой вариант, аналогично одиночному режиму), так и к AMQP-порту (для расширенных возможностей маршрутизации).

---

## 2. Компоненты

### 2.1 RabbitMQ (обязательный)

**Роль:**
- MQTT broker для устройств (через RabbitMQ MQTT plugin)
- AMQP для inter-instance communication
- AMQP для external plugins
- Queue для команд (routing to target instance)

**Топология:**

| Обмен | Тип | Назначение |
|-------|-----|------------|
| `iot.devices` | topic | Телеметрия от устройств (маршрутизация по `account_slug.product_id.device_id.kind`) |
| `iot.commands` | topic | Команды устройствам (маршрутизация по `account_slug.product_id.device_id.kind`) |
| `iot.cluster` | fanout | События между экземплярами (все экземпляры получают) |
| `iot.external` | topic | События для внешних плагинов (маршрутизация по `account_slug.#`) |

**Очереди на экземпляр:**
- `iot.cluster.events.{instance_id}` — события для экземпляра (для SSE)

**Примечание:** RabbitMQ сам доставляет команды устройствам через MQTT-сессию (см. [§3.2](#32-маршрутизация-команд-и-корреляция-ответов)).

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
- Общее состояние для всех экземпляров кластера
- Все данные (телеметрия, устройства, пользователи и т. д.)

**Отслеживание состояния устройств (онлайн/офлайн):**

RabbitMQ предоставляет встроенные механизмы:
- **MQTT Last Will and Testament (LWT)** — устройство при подключении может задать «последнее волеизъявление» (сообщение, которое RabbitMQ опубликует при разрыве соединения)
- **События подключения/отключения** — RabbitMQ MQTT plugin генерирует события `$SYS/broker/connected` и `$SYS/broker/disconnected`, которые можно подписать и использовать для обновления статуса устройства в PostgreSQL

Для отслеживания состояния устройств достаточно обновлять поле `last_seen_at` в существующей таблице `devices` на основе событий RabbitMQ.

**Таблицы для аудита и RPC:**

В кластерном режиме используются две **независимые** таблицы (не связаны между собой, даже если данные дублируются):

- **`rpc_calls`** — история RPC-вызовов (запросы и ответы). Запись создаётся при отправке команды (`status='pending'`), обновляется при получении ответа (`status='ok'/'error'/'timeout'`). Используется для отображения истории RPC в интерфейсе.
- **`audit_log`** — общий журнал событий: подключения/отключения устройств, изменения статусов, логи устройств (через специальный MQTT-топик), RPC-запросы и ответы. Каждая запись содержит `event_type`, `device_pk`, `instance_id` (какой экземпляр записал), `payload` (JSONB). Используется для аудита и отладки.

### 2.3 Экземпляр кластера (`platform-cluster`)

**Роль:**
- Сервер интерфейса и REST API (HTTP)
- Сервер SSE (на базе HTTP, не WebSocket)
- Экспортёр Prometheus (простой, без встроенной TSDB)
- Клиент RabbitMQ (приём событий, публикация команд)

**НЕ имеет:**
- Встроенного брокера MQTT
- Внутренних плагинов (они в рабочих процессах)
- Сервера WebSocket (заменён на SSE)

**Без состояния:** может быть запущено N экземпляров за балансировщиком L7. Каждый экземпляр обрабатывает свои HTTP-запросы и SSE-соединения.

**Конфигурация:**
```yaml
mode: cluster
http_addr: ":8080"
rabbitmq_url: "amqp://guest:guest@rabbitmq:5672/"
postgres_dsn: "postgres://user:pass@postgres:5432/platform"
instance_id: "${HOSTNAME}"  # автогенерация, для диагностики и логов
```

### 2.4 Рабочий процесс (`platform-worker`)

**Роль:**
- Запускает внутренние плагины (schedule, led-garland и т. д.)
- Выполнение без состояния (без выбора лидера)
- Может быть запущено N рабочих процессов

**Архитектурно: рабочий процесс = «потребитель SDK + обработчики плагинов в процессе»**

> Концептуально рабочий процесс не отличается от внешнего плагина, который:
> 1. Подписывается на обмен `iot.external` через SDK
> 2. Обрабатывает события
> 3. Публикует команды в `iot.commands`
>
> Единственное отличие: «код плагина» у рабочего процесса — это **Go-пакеты, скомпилированные в бинарник** (в процессе), а не отдельный процесс. Всё остальное — такое же.

**Функции:**
- Подписан на события RabbitMQ (через тот же Plugin SDK)
- Запускает внутренние плагины как обработчики (в процессе)
- Публикует команды обратно в RabbitMQ

**Выполнение без состояния:**
- Каждый рабочий процесс получает все события
- Обрабатывает свою часть (например, плагин schedule срабатывает на каждом рабочем процессе — но **только** если у плагина есть встроенная дедупликация через уникальные ограничения в БД)
- Нет координации между рабочими процессами

**Сравнение:**

| Аспект | Внешний плагин | Рабочий процесс + внутренний плагин |
|--------|----------------|-------------------------------------|
| Транспорт | MQTT или AMQP через SDK | AMQP через SDK (тот же) |
| Код | Отдельный процесс | Go-пакет в бинарнике рабочего процесса |
| Подписка на события | ✓ | ✓ |
| Публикация команд | ✓ | ✓ |
| Язык | Любой (Go, Python, C++, Java) | Только Go |
| Развёртывание | Отдельно (systemd, docker) | Вместе с `platform-worker` |
| Жизненный цикл | Запуск/остановка/перезапуск отдельно | Вместе с рабочим процессом |

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

### 2.5 Внешние плагины (MQTT или AMQP)

В кластерном режиме внешние плагины подключаются **напрямую к RabbitMQ**, а не к конкретному экземпляру платформы. Поскольку RabbitMQ держит все MQTT-соединения (включая устройства), плагин может использовать тот же протокол, что и в одиночном режиме.

**Два транспорта на выбор:**

| Транспорт | Когда использовать | Подключение |
|-----------|-------------------|-------------|
| **MQTT** | Простой плагин, один экземпляр, совместимость с одиночным режимом | `mqtt://<rabbitmq>:1883` (порт MQTT RabbitMQ) |
| **AMQP** | Расширенная маршрутизация, несколько плагинов, обмен сообщениями | `amqp://<rabbitmq>:5672` (порт AMQP RabbitMQ) |

**Подключение через MQTT (простой вариант):**
- Плагин подключается к порту MQTT RabbitMQ (:1883) с учётными данными `extplg_{slug}_{id}`
- Подписывается на топики `platform/v1/events/{slug}/+/+/telemetry` (и другие виды событий)
- Публикует команды в топики `platform/v1/commands/{slug}/{product}/{device}/rpc`
- RabbitMQ MQTT plugin транслирует MQTT-подписки во внутренние очереди AMQP
- **Преимущество:** код плагина не меняется при переходе с одиночного режима на кластер — меняется только адрес брокера

**Подключение через AMQP (расширенный вариант):**
- Плагин подключается к порту AMQP RabbitMQ (:5672) с теми же учётными данными
- Подписывается на exchange `iot.external` с ключом маршрутизации `events.{slug}.#`
- Публикует команды в exchange `iot.commands` с ключом маршрутизации `commands.{slug}.{product}.{device}.{kind}`
- **Преимущество:** прямой доступ к механизмам маршрутизации RabbitMQ (обмены, ключи маршрутизации, очереди)

**Разработка плагинов:**
- Используют `pkg/plugin-sdk` (публичный модуль Go)
- SDK абстрагирует транспорт — код плагина одинаковый для обоих режимов
- Транспорт выбирается параметром `Transport` в конфигурации SDK (`"mqtt"` или `"amqp"`)

### 2.5.1 ACL внешних плагинов в кластере

Поскольку плагины подключаются напрямую к RabbitMQ, контроль доступа (ACL) обеспечивается **самим RabbitMQ**, а не экземпляром платформы.

**Для MQTT-подключений:**
- RabbitMQ MQTT plugin поддерживает проверку учётных данных через `auth_backend` (платформа регистрирует пользователя `extplg_{slug}_{id}` с паролем в RabbitMQ)
- Проверка топиков: RabbitMQ MQTT plugin поддерживает `mqtt_topic_authorization` — проверка того, что плагин с username `extplg_{slug}_*` может подписываться только на топики, содержащие `{slug}`, и публиковать только в топики с `{slug}`
- Пример правила: пользователь `extplg_acme_*` → подписка только на `platform/v1/events/acme/#`, публикация только в `platform/v1/commands/acme/#`

**Для AMQP-подключений:**
- RabbitMQ поддерживает права доступа на уровне виртуального хоста (vhost): `configure`, `write`, `read` с регулярными выражениями для обменов и ключей маршрутизации
- Для каждого внешнего плагина создаётся пользователь RabbitMQ с правами:
  - `write` на exchange `iot.commands` с шаблоном ключа маршрутизации `commands.{slug}.*`
  - `read` на exchange `iot.external` с шаблоном ключа маршрутизации `events.{slug}.#`
- Попытка подписаться на чужой аккаунт или опубликовать в чужой топик будет отклонена RabbitMQ

**Гарантия изоляции:** плагин аккаунта `acme` физически не может получить данные аккаунта `beta`, потому что RabbitMQ отклонит подписку/публикацию на уровне протокола.

### 2.5.2 Node-RED: переход на кластер

Node-RED — самый популярный внешний плагин. В одиночном режиме он подключается к платформе через **MQTT-ноды**. В кластерном режиме Node-RED может **продолжить использовать MQTT-ноды** — достаточно изменить адрес брокера с адреса платформы на адрес RabbitMQ (порт :1883).

**Сравнение для Node-RED:**

| Параметр | Одиночный режим | Кластер (MQTT) | Кластер (AMQP) |
|----------|-----------------|----------------|----------------|
| **Нода** | `mqtt in` / `mqtt out` | `mqtt in` / `mqtt out` (без изменений) | `amqp in` / `amqp out` |
| **Адрес брокера** | `<platform>:1883` | `<rabbitmq>:1883` | `<rabbitmq>:5672` |
| **Подписка на события** | Topic: `platform/v1/events/{slug}/#` | Topic: `platform/v1/events/{slug}/#` (тот же) | Exchange `iot.external` + routing key `events.{slug}.#` |
| **Публикация команд** | Topic: `platform/v1/commands/{slug}/...` | Topic: `platform/v1/commands/{slug}/...` (тот же) | Exchange `iot.commands` + routing key `commands.{slug}...` |
| **Учётные данные** | `extplg_{slug}_{id}` | `extplg_{slug}_{id}` (те же) | `extplg_{slug}_{id}` (те же) |

**Рекомендация:** для простоты миграции используйте MQTT-ноды в кластере — нужно изменить только адрес брокера. AMQP-ноды стоит использовать, если нужна расширенная маршрутизация или работа с несколькими обменами.

**Пошагово для MQTT (простой переход):**

1. В конфигурации Node-RED изменить адрес MQTT-брокера: было `<platform>:1883`, стало `<rabbitmq>:1883`.
2. Учётные данные (`extplg_{slug}_{id}` и пароль) остаются без изменений.
3. Топики подписки и публикации остаются без изменений.
4. **Deploy** — соединение должно стать зелёным.

**Пошагово для AMQP:**

1. Установить палетт: меню → Manage palette → Install `node-red-contrib-amqp`.
2. Создать `amqp-broker` config-ноду:
   - Host: `<rabbitmq>` (в docker-compose) или `<rabbitmq-host>` (bare-metal)
   - Port: `5672`
   - Username: `extplg_<account_slug>_<plugin_id>` (из интерфейса)
   - Password: тот же пароль из интерфейса
3. В flow заменить `mqtt in` → `amqp in`, `mqtt out` → `amqp out`:
   - **Подписка на события**: exchange = `iot.external`, exchange type = `topic`, routing key = `events.{account_slug}.#`
   - **Публикация команд**: exchange = `iot.commands`, exchange type = `topic`, routing key = `commands.{account_slug}.{product_id}.{device_id}.rpc`
4. **Deploy** — соединение должно стать зелёным.

**Куда положить Node-RED:** в кластерном режиме Node-RED должен иметь **сетевой доступ к RabbitMQ** (порт 5672 для AMQP или 1883 для MQTT). В docker-compose — `networks: [cluster_net]` на обоих сервисах.

**Полная инструкция с JSON-примерами нод:** см. [06-automation.md → HA / Cluster mode (RabbitMQ)](06-automation.md#ha--cluster-mode-rabbitmq).

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

### 3.2 Маршрутизация команд и корреляция ответов

RabbitMQ держит все MQTT-соединения устройств, поэтому маршрутизация команд предельно проста: любой экземпляр `platform-cluster` публикует команду в RabbitMQ, а RabbitMQ доставляет её устройству через MQTT-сессию.

#### 3.2.1 Отправка команды

```mermaid
sequenceDiagram
    participant U as Интерфейс / API
    participant CI as Любой экземпляр<br/>platform-cluster
    participant EX as Exchange<br/>iot.commands
    participant MQ as RabbitMQ<br/>(MQTT plugin)
    participant D as Устройство

    U->>CI: POST /api/v1/devices/{id}/rpc
    CI->>CI: генерация request_id (UUID)
    CI->>EX: PUBLISH routing_key=<br/>commands.{account_slug}.{product}.{device}.{kind}<br/>payload: {"id": "uuid-...", "method": "...", "params": {...}}
    EX->>MQ: маршрутизация в MQTT-топик
    MQ->>D: downlink через MQTT-сессию<br/>(iot/v1/{product}/{device}/rpc/{method})
    D-->>MQ: PUBACK
    MQ-->>CI: подтверждение (через AMQP)
    CI->>CI: сохранение {request_id → HTTP-канал} в памяти
    CI-->>U: 200 OK (или ожидание ответа)
```

#### 3.2.2 Возврат ответа и корреляция

Устройство обрабатывает команду и публикует ответ в MQTT-топик `iot/v1/{product}/{device}/rpc/response`. Поле `id` в ответе содержит тот же `request_id` (UUID), что и в запросе (стандарт JSON-RPC).

```mermaid
sequenceDiagram
    participant D as Устройство
    participant MQ as RabbitMQ<br/>(MQTT plugin)
    participant EX as Exchange<br/>iot.devices
    participant Q as Queue<br/>iot.cluster.events
    participant CIA as Cluster Instance A
    participant CIB as Cluster Instance B

    D->>MQ: PUBLISH iot/v1/{product}/{device}/rpc/response<br/>payload: {"id": "uuid-...", "result": {...}}
    MQ->>EX: пересылает в exchange iot.devices
    EX->>Q: маршрутизация
    Q->>CIA: fanout
    Q->>CIB: fanout
    
    CIA->>CIA: проверка: есть ли ожидающий запрос<br/>с таким request_id?
    CIA->>CIA: ДА → отправка HTTP-ответа клиенту
    
    CIB->>CIB: проверка: есть ли ожидающий запрос<br/>с таким request_id?
    CIB->>CIB: НЕТ → игнорирование
```

**Механизм корреляции:**

1. **Генерация `request_id`:** экземпляр, получивший HTTP-запрос, генерирует UUID для `request_id` и сохраняет в памяти связь `{request_id → HTTP-канал}` (с таймаутом).

2. **Публикация команды:** `request_id` помещается в поле `id` полезной нагрузки MQTT-сообщения (стандарт JSON-RPC).

3. **Ответ устройства:** устройство обрабатывает команду и публикует ответ с тем же `id` (копирует из запроса — стандартное поведение JSON-RPC).

4. **Fanout и фильтрация:** RabbitMQ доставляет ответ всем экземплярам кластера через fanout. Каждый экземпляр проверяет: есть ли у него ожидающий запрос с таким `request_id`?
   - Если **да** — отправляет HTTP-ответ клиенту.
   - Если **нет** — игнорирует ответ.

**Формат полезной нагрузки:**

*Запрос (платформа → устройство):*

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "method": "reboot",
  "params": {}
}
```

*Ответ (устройство → платформа):*

```json
{
  "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "result": {"status": "ok"}
}
```

**Ключевые свойства:**

- **Уникальность:** UUID гарантирует уникальность `request_id` без координации между экземплярами кластера.
- **Простота устройства:** устройство не обязано понимать, что такое `instance_id` или кластер — оно просто копирует `id` из запроса в ответ (стандартное поведение JSON-RPC).
- **Не требуется изменение прошивки:** любая нормальная реализация JSON-RPC уже эхоирует `id` из запроса в ответ.
- **Масштабируемость:** fanout + фильтрация по `request_id` работает эффективно при умеренном количестве одновременных RPC-вызовов.

#### 3.2.3 Синхронный RPC — только для интерфейса

**Важно:** механизм синхронного ожидания ответа (fanout + проверка в памяти) используется **только для HTTP-API** (интерфейс пользователя). Внешние плагины и автоматизация работают **асинхронно**:

- **Интерфейс (UI):** `POST /api/v1/devices/{id}/rpc?wait=1` → платформа генерирует `request_id`, сохраняет `{request_id → HTTP-канал}` в памяти, ждёт ответ через fanout, отправляет HTTP-ответ клиенту.
- **Внешние плагины:** подписываются на MQTT-топики событий (`iot/v1/{product}/{device}/rpc/response`) и получают ответы асинхронно. Плагины сами фильтруют события по `request_id` или другим критериям.
- **Автоматизация (Node-RED):** работает через MQTT-шину правил, получает события асинхронно.

**Вывод:** сложная система корреляции (БД как арбитр, временные очереди, `instance_id` в payload) **не нужна**. Простой fanout + проверка `request_id` в памяти достаточно, потому что синхронный RPC используется только для UI, а UI не генерирует тысячи одновременных запросов.

**Если RabbitMQ кластеризован** (несколько узлов RabbitMQ): MQTT-плагин RabbitMQ автоматически маршрутизирует сообщение к тому узлу, где установлена MQTT-сессия устройства. Это стандартное поведение RabbitMQ MQTT plugin — дополнительная маршрутизация на уровне платформы не требуется.

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

// Event — событие от устройства или платформы
type Event struct {
    AccountSlug string
    ProductID   string
    DeviceID    string
    Kind        string  // telemetry, rpc, status, log, etc.
    Payload     []byte
    Timestamp   int64
}

// EventHandler — функция обработки событий
type EventHandler func(event Event)

// Logger — интерфейс логирования
type Logger interface {
    Info(msg string, args ...any)
    Error(msg string, args ...any)
}

// Metrics — интерфейс метрик
type Metrics interface {
    IncrCounter(name string, labels map[string]string)
}

// Plugin — основной интерфейс плагина
type Plugin interface {
    // Подписка на события
    Subscribe(pattern string, handler EventHandler) error
    
    // Публикация команды устройству
    Publish(topic string, payload []byte) error
    
    // Запуск плагина (блокирующий вызов)
    Run() error
}

// Config — конфигурация плагина
type Config struct {
    Transport   string  // "mqtt" или "amqp"
    Connection  string  // Строка подключения MQTT или AMQP
    AccountID   int64
    AccountSlug string
    PluginID    string
    Token       string
    Logger      Logger
    Metrics     Metrics
}
```

**Конструктор:** `func New(cfg Config) (Plugin, error)` — создаёт новый экземпляр плагина. Реализация зависит от `Transport` (mqtt или amqp).

**Соответствие транспорта и режима развёртывания:**

| `Transport` | Когда использовать | Строка подключения |
|-------------|---------------------|---------------------|
| `mqtt` | Одиночный режим (встроенный брокер) | `mqtt://user:pass@host:1883/` |
| `mqtt` | Кластер (RabbitMQ MQTT plugin) | `mqtt://user:pass@rabbitmq:1883/` |
| `amqp` | Кластер (RabbitMQ AMQP) | `amqp://user:pass@rabbitmq:5672/` |

**Правило:** транспорт выбирается режимом развёртывания, а не разработчиком плагина:
- Одиночный режим → только `mqtt` (встроенный брокер)
- Кластер → `mqtt` (RabbitMQ MQTT plugin, простой вариант) или `amqp` (RabbitMQ AMQP, расширенный вариант)
- Один и тот же код плагина работает в обоих режимах через SDK

### 4.3 Использование

*Пример кода внешнего плагина (псевдокод):*

```
// Инициализация плагина
p := plugin.New(Config{
    Transport:   "mqtt",                                          // или "amqp" для кластера
    Connection:  "mqtt://extplg_acme_bot:pass@rabbitmq:1883/",   // строка подключения
    AccountID:   42,
    AccountSlug: "factory-1",
    PluginID:    "telemetry-logger",
    Token:       "<api-token>",
})

// Подписка на события (паттерн одинаковый для mqtt/amqp)
p.Subscribe("events.{account_slug}.{product_id}.{device_id}.#", func(e Event) {
    log.Printf("событие: %s", e.Kind)
    // обработка события
})

// Публикация команды устройству
p.Publish("commands.{account_slug}.{product_id}.{device_id}.rpc", payload)

// Запуск (блокирующий вызов)
p.Run()
```

### 4.4 Реализации транспорта

| Транспорт | Топик → ключ маршрутизации | Протокол | Где работает |
|-----------|---------------------------|----------|--------------|
| **MQTT** | `platform/v1/events/{slug}/{product}/{device}/{kind}` | MQTT pub/sub | Одиночный режим (встроенный брокер) или кластер (RabbitMQ MQTT plugin) |
| **AMQP** | exchange `iot.external` + ключ маршрутизации `events.{slug}.{product}.{device}.{kind}` | AMQP pub/sub | Кластер (RabbitMQ AMQP) |

**SDK обрабатывает:**
- Управление соединением (переподключение, контрольные сигналы)
- Преобразование топиков и ключей маршрутизации (для каждого транспорта)
- Сериализация (JSON, protobuf)
- Обработка ошибок
- Метрики

**Преимущества:**
- Разработчик внешнего плагина пишет код **один раз**
- SDK абстрагирует транспорт (но не скрывает — `Transport` задаётся явно в конфигурации)
- Легко добавить новый транспорт (только реализация в SDK)

### 4.5 Архитектура SDK

```mermaid
graph TB
    subgraph Plugin["Внешний плагин (код разработчика)"]
        CODE["p.Subscribe(pattern, handler)<br/>p.Publish(topic, payload)"]
    end

    subgraph SDK["Plugin SDK (pkg/plugin-sdk)"]
        API["API layer<br/>(Subscribe/Publish/Run)"]
        TRANS["Абстракция транспорта"]
        SER["Сериализация<br/>(JSON/protobuf)"]
        CONN["Управление соединением<br/>(переподключение, контроль)"]
    end

    subgraph Impl["Реализации транспорта"]
        MQTT["MQTT транспорт<br/>(одиночный + кластер)"]
        AMQP["AMQP транспорт<br/>(кластер)"]
    end

    subgraph Brokers["Брокеры"]
        MB["Встроенный<br/>mochi-mqtt<br/>(одиночный)"]
        RB["RabbitMQ<br/>(кластер)"]
    end

    CODE --> API
    API --> TRANS
    TRANS --> SER
    TRANS --> CONN
    TRANS --> MQTT
    TRANS --> AMQP
    MQTT --> MB
    MQTT --> RB
    AMQP --> RB

    style Plugin fill:#16213e,stroke:#1a1a2e,color:#eee
    style SDK fill:#0f3460,stroke:#16213e,color:#eee
    style Impl fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Brokers fill:#16213e,stroke:#1a1a2e,color:#eee
```

---

## 5. Функциональные требования (HA-specific)

Детальные таблицы F2-xxx вынесены в отдельный документ: **[17-ha-architecture-requirements.md](17-ha-architecture-requirements.md)**. Здесь — только навигация по группам, чтобы было видно, какие ID покрывают какие подсистемы.

| Подсистема | Секция в sub-doc | ID-диапазон |
|------------|------------------|-------------|
| HA Foundation (v2.0): экземпляр кластера, рабочий процесс, RabbitMQ, маршрутизация команд, SSE, Prometheus | [§5.1](17-ha-architecture-requirements.md#51-f2-01f2-07-ha-foundation-v20) | F2-01…F2-07 |
| Account-level features (v2.2): RBAC, rate limiting, retention, import/export, signing, remote_write | [§5.2](17-ha-architecture-requirements.md#52-f2-21f2-26-account-level-features-v22) | F2-21…F2-26 |
| Future (v2.3+): SSO, SIEM, transfer, dashboards, DR, marketplace | [§5.3](17-ha-architecture-requirements.md#53-f2-31f2-36-future-v23) | F2-31…F2-36 |

**Соглашения по ID:**

- `F2-0xx` — HA Foundation (v2.0); `F2-1xx` — *(удалённый диапазон, бывший `rabbitmq-bridge` plugin; см. §7.6 в main)*; `F2-2xx` — Account-level features (v2.2); `F2-3xx` — Future (v2.3+).
- Удалённые ID: `F2-11..F2-15` (v2.1, отменены вместе с `rabbitmq-bridge`) — указаны явно в sub-doc.

**Правило для PR:** новые F2-IDs добавляются в sub-doc; в main-документе упоминается только диапазон (например, `F2-4xx` для новой HA-фичи). Это держит основной HA-PRD компактным и читаемым.

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

---

## 7.6 HA-readiness: контракт с v1

В v1 (single mode) HA-сценарии не применимы, но **закладываются архитектурно**, чтобы не заблокировать переход к HA в v2. Полный список инвариантов — в [NF-19 в 16-multi-account-requirements.md](16-multi-account-requirements.md).

| Кейс | v1 поведение | v2 поведение |
|------|--------------|--------------|
| Два инстанса с одной SQLite БД | v1 запрещает (NF-19: инвариант «single instance») | Не применимо (HA на Postgres) |
| Два инстанса с одной Postgres БД | v1 формально не запрещает, но broadcast дублируется между instance'ами | Без изменений: SSE не требует sticky session; `instance_id` используется только для diagnostics/log. Каждый instance вещает свои события, клиент в любой момент может переподключиться к другому LB-target |
| Один инстанс упал, второй подхватил | Не работает (real-time клиенты теряют события в момент переподключения) | SSE auto-reconnect через `Last-Event-ID`; события за downtime теряются (v2.x: persistent buffer) |
| External plugin подключился, инстанс перезапустился | MQTT-сессия теряется; плагин переподключается | Через external broker + persistent sessions |
| Cache изменился на инстансе A, запрос пришёл на B | v1: stale data до reload (per-request check) | Через PostgreSQL LISTEN/NOTIFY или cache TTL |
| Postgres failover во время миграции | v1 не применимо (только sqlite) | `golang-migrate` retry + advisory lock; failed migration → manual recovery |

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

| Дата | Тип | Описание |
|------|-----|----------|
| 12.09.2026 | Начало | Первая версия документа. Архитектура HA/clustering: RabbitMQ, PostgreSQL, три бинарника, SSE для интерфейса, External Plugin SDK |
| 13.09.2026 | Корректировка | Переписан §0 (убраны англоязычные вставки). Переработан §2.5 — внешние плагины могут работать через MQTT или AMQP (оба транспорта идут в RabbitMQ). Упрощена маршрутизация команд (§3.2) — RabbitMQ сам держит MQTT-соединения устройств. Добавлен механизм корреляции запрос-ответ через `request_id` (UUID) с fanout-фильтрацией (§3.2.1–3.2.2). Добавлен §3.2.3 — синхронный RPC только для UI, плагины работают асинхронно. Добавлено описание таблиц `rpc_calls` и `audit_log` (§2.2). Добавлены подробности ACL для плагинов (§2.5.1). Обновлены §1.1, §4 |
