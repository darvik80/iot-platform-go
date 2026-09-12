# HA Architecture — Детализированные требования (F2-xxx)

| Поле | Значение |
|------|----------|
| **Версия документа** | 1.0 |
| **Версия платформы** | v2.x (см. [§7 План релиза HA](17-ha-architecture.md#7-план-релиза-ha)) |
| **Статус** | Черновик → Активен |
| **Обновлено** | Сентябрь 2026 |
| **Назначение** | вынесенный из основного PRD перечень функциональных требований с F2-IDs; основной документ — [17-ha-architecture.md](17-ha-architecture.md) |
| **Каноническая версия** | этот документ ведётся на русском; английский перевод не предусмотрен |

> **Это sub-документ.** Здесь только детальные таблицы требований с F2-IDs. Видение, архитектура, потоки данных, External Plugin SDK, принцип «не делаем», план релиза и backlog остаются в [17-ha-architecture.md](17-ha-architecture.md). Нумерация §5.1…§5.3 сохранена, чтобы ссылки из основного документа и README работали без перерасчёта.

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

> **Примечание:** F2-11..F2-15 (`rabbitmq-bridge` plugin) **удалены** — см. [§7.6 в 17-ha-architecture.md](17-ha-architecture.md#76-rabbitmq-bridge-plugin--решено-удалить). External apps подключаются к RabbitMQ напрямую через SDK.

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

**Соглашения по ID:**

- `F2-0xx` — HA Foundation (v2.0): cluster instance, worker, RabbitMQ, device registry, command routing, SSE, Prometheus
- `F2-1xx` — *(удалённый диапазон, бывший `rabbitmq-bridge` plugin; см. §7.6 в main)*
- `F2-2xx` — Account-level features (v2.2): RBAC, rate limiting, retention, import/export, signing, remote_write
- `F2-3xx` — Future (v2.3+): SSO, SIEM, transfer, dashboards, DR, marketplace
- Удалённые ID: `F2-11..F2-15` (v2.1, отменены вместе с `rabbitmq-bridge`).

**Правило для PR:** новые F2-IDs добавляются в sub-doc; в main-документе упоминается только диапазон (например, `F2-4xx` для нового HA-фичи). Это держит основной HA-PRD компактным и читаемым.

---

## 6. Связанная документация

| Док | Что |
|-----|------|
| [← 17-ha-architecture.md](17-ha-architecture.md) | основной документ: видение, архитектура, потоки данных, SDK, принцип «не делаем», план релиза, backlog |
| [16-multi-account.md](16-multi-account.md) | Single mode (v1.0..v1.6) — multi-account и связанные фичи |
| [16-multi-account-requirements.md](16-multi-account-requirements.md) | F-xxx / NF-xxx sub-doc для single mode (sibling-документ, та же структура) |
| [01-prd.md](01-prd.md) | общий продуктовый PRD |
| [02-api.md](02-api.md) | REST API reference (будет дополнен HA endpoints) |
| [04-mqtt.md](04-mqtt.md) | MQTT-протокол (RabbitMQ MQTT plugin) |
| [07-internal-plugins.md](07-internal-plugins.md) | Internal plugins (in-process в v1.x; в workers в v2.x) |
| [08-external-plugins.md](08-external-plugins.md) | External plugins (MQTT в single, AMQP в cluster) |
| [11-operations.md](11-operations.md) | эксплуатация (cluster deployment) |
| [14-diagrams.md](14-diagrams.md) | все диаграммы (HA architecture diagrams) |
