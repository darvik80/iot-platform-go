# Multi-Account и встроенный мониторинг — Требования к продукту

> **Версия документа**: 1.1
> **Версия платформы**: v1.x (см. [§4.5 План релиза](#45-план-релиза--этапы))
> **Статус**: Черновик → Активен
> **Обновлено**: Сентябрь 2026
> **Область**: добавление multi-tenant изоляции, IAM-ролей, monitoring-плагинов и управления внутренними плагинами в текущий single-tenant дистрибутив
> **Каноническая версия**: этот документ ведётся на русском; английский ([en/16-multi-account.md](../en/16-multi-account.md)) будет переписан с нуля после стабилизации v1.

Этот документ определяет следующий крупный релиз платформы: **multi-account tenancy** с ролевым доступом, новый формат системных MQTT-топиков и встроенный мониторинг (Prometheus). Релиз additive — пересоздание БД и конфигурации при переходе допустимо (миграционных утилит не предусматривается).

Связанные доки: [01-prd](01-prd.md), [02-api](02-api.md), [03-auth](03-auth.md), [04-mqtt](04-mqtt.md), [07-internal-plugins](07-internal-plugins.md), [08-external-plugins](08-external-plugins.md), [11-operations](11-operations.md).

---

## 0. Что меняется на одной странице

- Появляется сущность **Account**; все per-account ресурсы получают `account_id NOT NULL`.
- **Один пользователь** может состоять в нескольких аккаунтах с разными ролями (`admin` / `operator` / `viewer`).
- **Внешние плагины** жёстко привязаны к аккаунту — `mqtt_user = extplg_{account_slug}_{plugin_id}`, ACL сужается до `platform/v1/{events|commands}/{account_slug}/#`.
- **Системные топики** rules bus получают `account_slug` (hard break). **Топики устройств `iot/v1/...` и `v1/...` не меняются** — устройства не знают про аккаунты (см. [§5.7.1](#571-что-не-меняется-в-топиках-устройств)).
- **Встроенный мониторинг**: каталожные плагины `prometheus-exporter` (HTTP `/metrics`) и `metrics-dashboard` (UI-шаблон).
- **Управление внутренними плагинами** на уровне аккаунта через `account_plugin_policies`.
- **БД**: поддержка двух драйверов — `sqlite` (default) и `postgres`. Драйвер выбирается через `DATABASE_DRIVER` + `DATABASE_DSN`.
- **Мастер первой установки** (полный) — `/setup` доступен, пока `accounts` пуста; `.env` опционален.
- **Версионирование**: релиз разбит на этапы v1.0…v1.6 + явный v2 (см. [§4.5](#45-план-релиза--этапы)).
- **HA / кластеризация** — явно v2; v1 проектируется с учётом будущей HA, но реализует только single-instance (см. [§7.7](#77-архитектурные-ограничения-v1-для-будущей-кластеризации)).
- **Automation page** — депрекейс; замена на документацию по интеграции с Node-RED + inline help в UI.
- **UI modernization** — обновление визуального стиля, сохранение mobile-first подхода.

---

## 1. Видение

Платформа остаётся **одним Go-бинарником, одной SQLite-базой и одним MQTT-брокером**, но обслуживает сразу несколько **изолированных аккаунтов** (tenants). Каждый аккаунт — это отдельный набор продуктов, устройств, прошивок, аудита и настроек. Пользователь с одним логином может состоять в нескольких аккаунтах с разными ролями в каждом. Внешние плагины (Node-RED, gRPC, webhook) жёстко привязаны к аккаунту, который их зарегистрировал, и физически не могут читать данные соседей.

Поверх tenancy добавляются:

- **Встроенный мониторинг**: два новых каталожных плагина — `prometheus-exporter` (scrape-эндпоинт для внешнего Prometheus) и `metrics-dashboard` (шаблон визуализации метрик устройств в дашборде).
- **Управление внутренними плагинами** на уровне аккаунта — admin может отключить любой каталожный плагин, и он перестаёт создавать раннеры и MQTT ACL-события для устройств этого аккаунта.

Главный принцип релиза: **никаких миграций, никаких dual-write**. Это новая начальная схема БД; апгрейд с single-tenant инсталляции — отдельная одноразовая утилита `convert-to-multi-account`, которая заворачивает существующие продукты/устройства/плагины в один bootstrap-аккаунт `default` с логином bootstrap-админа.

---

## 2. Проблемы

| Боль | Текущее состояние | Что меняется |
|------|------------------|--------------|
| Один self-hosted = один tenant. Хочется обслуживать нескольких клиентов / несколько объектов / производство + тест | Все ресурсы общие, разделения нет, UI оперирует всем подряд | Появляется сущность **Account**; все ресурсы (`products`, `devices`, `external_plugins`, `ota_firmwares`, `audit_log`, telemetry и т.д.) получают `account_id` |
| Невозможно разграничить оператора склада и оператора лаборатории | Только глобальные роли `admin/operator/viewer` | Те же три роли, но **per-account**; один пользователь — admin в одном аккаунте, viewer — в другом |
| Внешний плагин (Node-RED / gRPC / webhook) видит все события платформы | ACL только `platform/v1/events/#` и `platform/v1/commands/#` | Топики получают `account_slug` сразу после префикса; ACL сужается до `platform/v1/events/{account_slug}/#` |
| Непонятно, какие метрики доступны и как их снимать — нет ни `/metrics`, ни интеграции с Prometheus | Нет | Новый каталожный плагин `prometheus-exporter` выставляет HTTP `/metrics` (формат Prometheus text) с лейблами `account`, `product`, `device` |
| Каталожные плагины всегда включены — даже если аккаунт не хочет, чтобы что-то тикало в фоне | Нет тоггла | Per-account policy `account_plugin_policies`: admin может выключить любой каталожный плагин в своём аккаунте |
| Нет шаблона для «встроенного» дашборда устройства на основе метрик | Только стандартный `Metrics` + `Events` | Новый каталожный плагин `metrics-dashboard` — шаблон, который добавляет в страницу устройства настраиваемые панели (графики телеметрии, RPC history, online-timeline) |

---

## 3. Целевые пользователи

### 3.1 Основной — оператор multi-tenant инсталляции
- Раздаёт self-hosted платформу нескольким внутренним командам или коммерческим клиентам.
- Хочет выдавать **приглашения по email** и не думать про отдельные БД.
- Хочет **посмотреть общий health** (через Prometheus), но не иметь доступа к данным тенантов.

### 3.2 Вторичный — корпоративный оператор внутри одного аккаунта
- 10–200 устройств на объекте, 3–10 операторов разного уровня.
- Нужен admin/operator/viewer **внутри** аккаунта; никакие глобальные права не нужны.
- Хочет встроенные графики (вместо «открой Grafana»).

### 3.3 Третичный — AI/LLM-агент
- MCP-инструменты должны принимать параметр `account_id` (по умолчанию — активный) и отказывать, если пользователь не состоит в этом аккаунте.

### 3.4 Без изменений
- Устройства (ESP32 и др.). Со стороны устройства платформа по-прежнему «один MQTT-брокер + один токен», прозрачно для firmware.

---

## 4. Цели и метрики успеха

| Цель | Метрика | Целевое значение |
|------|---------|------------------|
| Изоляция аккаунтов | `404` при попытке доступа к чужому `device_id` | 100% отрицаний на integration-тестах ACL |
| Скорость приглашения | `POST /accounts/{id}/invitations` → письмо | < 200 мс до возврата токена |
| Принятие приглашения | Время от `POST /invitations/{token}/accept` до `JWT с активным аккаунтом` | < 1 с |
| Переключение аккаунта | `POST /auth/switch-account` → новый JWT | < 100 мс (без обращения к БД при закэшированном membership) |
| Изоляция топиков | Кросс-аккаунтный `acl_deny` | 100% отрицаний при `subscribe platform/v1/events/other_account/#` |
| Scrape метрик | Время ответа `/metrics` на 1k устройств | < 250 мс (P95) |
| Минимальный footprint | Дополнительных процессов | 0 (всё in-process) |

---

## 4.5 План релиза / Этапы

Релиз разбит на **этапы v1.0…v1.6** плюс явно выделенный **v2 (HA / кластер)**. Каждый этап имеет **entry-критерий** (что должно быть до старта работ) и **exit-критерий** (что считается готовым). Этапы выполняются последовательно, но внутри этапа — параллельно (по фичам).

| Версия | Тема | Ключевые F-ID | Exit-критерий |
|--------|------|---------------|----------------|
| **v1.0** | **DB foundation + bootstrap** | F-101, F-102, F-104…F-106, F-1301…F-1306, F-1401…F-1412 | Чистая установка через визард поднимает платформу; SQLite и Postgres проходят одинаковый smoke; в БД есть один аккаунт `default` с admin |
| **v1.1** | **Auth + Tenancy** | F-201…F-208, F-401…F-407, F-501…F-507, F-108 | Все listing-эндпоинты фильтруют по `account_id`; кросс-аккаунтный доступ = `404`; account switcher в UI работает; JWT содержит `active_account_id` |
| **v1.2** | **External plugin ACL + system topics** | F-601…F-606, F-701…F-703, F-705, F-706 | `mqtt_user = extplg_{slug}_{id}`; ACL в `internal/mqtt/hook.go` сужен до `platform/v1/{events\|commands}/{slug}/#`; cross-account → `acl_deny`; `flows.json` обновлён |
| **v1.3** | **IAM + Invitations** | F-301…F-308 | `POST /invitations` → письмо → `accept` → membership; resend/revoke работают; last-admin защита |
| **v1.4** | **Catalog plugin management** | F-801…F-806 | `account_plugin_policies` управляет hub; `PATCH /policies/{pluginId}` применяется «на лету»; UI Settings → Integrations |
| **v1.5** | **Monitoring** | F-901…F-908, F-1001…F-1010 | `prometheus-exporter` отдаёт метрики; `metrics-dashboard` рисует таб в device page; scrape-конфиг опубликован |
| **v1.6** | **Polish + docs + UI** | F-107, F-1101…F-1106, F-1201…F-1205, F-1501…F-1505, NF-10…NF-12, NF-22…NF-24 | `audit_log` с `actor_kind`; UI-страницы (Members, Invitations, Account Settings) полированы; Automation page удалена, заменена документацией + inline help; UI modernized (NF-22..24); 02/03/04/06/07/08/11 обновлены; examples/plugins/* под новые топики |
| **v2.0** | **HA / Clustering** *(вне v1)* | (см. [§14](#14-будущие-улучшения)) | Multi-instance: N инстансов за L7-балансировщиком; внешний MQTT broker; Redis pub/sub для WS; leader election для plugin runners; Postgres — обязательный драйвер |
| **v2.1** | **rabbitmq-bridge** *(вне v1)* | (см. [§14](#14-будущие-улучшения)) | Каталожный плагин `rabbitmq-bridge` публикует события в RabbitMQ exchanges с лейблом `account` |

**Правила перехода между этапами:**

1. Каждый этап заканчивается **review-чекпоинтом** (PR с меткой `multi-account/v1.x`) и записью в CHANGELOG.
2. **Документация обновляется в том же PR**, что и код (NF-12).
3. **Тесты растут вместе с фичей**: unit (изоляция, ACL, валидаторы) + integration (смоук обоих драйверов БД) + ручной чеклист.
4. **Никакого carry-over**: если что-то из этапа не вошло — задача переходит в `Backlog / v1.x+1`, а не блокирует релиз.
5. **v2 не планируется до завершения v1.6** — после v1.6 делаем retro и пишем отдельный PRD под v2.

**Backlog / кандидаты на v1.x+1 (между v1.6 и v2.0):**

- Custom RBAC (per-account custom roles поверх встроенных трёх)
- Account-level feature flags
- Account import/export (JSON)
- Per-account API rate limiting (без Redis — локально)
- Webhook signing per-account (HMAC secret per account)
- Per-account data retention override

---

## 5. Функциональные требования

### 5.1 Сущность Account

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-101 | Новый каталожный объект `account`: `id`, `slug` (уникальный, `[a-z0-9][a-z0-9-]{1,30}`), `name`, `settings_json`, `status` (`active`/`suspended`), `created_at` | P0 | ☐ |
| F-102 | Bootstrap: при первом запуске платформа создаёт один аккаунт `default` (slug `default`, name `Default`) и привязывает к нему bootstrap-админа (env `ADMIN_LOGIN/PASSWORD`) с ролью `admin`. Если установлен мастер первой установки (см. [§5.14](#514-мастер-первой-установки)) — bootstrap не выполняется, ждём прохождения визарда | P0 | ☐ |
| F-103 | *(удалён в v1.1 — миграционных утилит нет, проект на стадии прототипа, при переходе БД пересоздаётся вручную)* | — | — |
| F-104 | Каждый ресурс с per-account scope получает `account_id NOT NULL`: `products`, `devices`, `external_plugins`, `device_plugin_bindings`, `ota_firmwares`, `ota_jobs`, `events`, `device_logs`, `telemetry_samples`, `device_attributes_*`, `attribute_history`, `rpc_calls`, `audit_log`, `ingest_dedup`, `device_credentials` | P0 | ☐ |
| F-105 | `products`: уникальность `(account_id, product_id)` — два аккаунта могут иметь продукт с одинаковым `product_id` | P0 | ☐ |
| F-106 | `devices` берёт `account_id` через `product_pk`; constraint на уровне приложения — `devices.account_id == products.account_id` | P0 | ☐ |
| F-107 | Аккаунт `suspended` — все API-запросы членов возвращают `403 account suspended`; MQTT-сессии устройств остаются, но новые `CONNECT` от устройств аккаунта отклоняются; UI рисует баннер | P1 | ☐ |
| F-108 | Платформенный admin (`ADMIN_LOGIN`, флаг `is_platform_admin=true`) — может читать и модифицировать любой аккаунт; не обязан состоять в нём. Все super-admin действия логируются в `audit_log` с `actor_kind='platform_admin'` | P0 | ☐ |

### 5.2 Membership (multi-account пользователи)

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-201 | Новая таблица `account_memberships(user_id, account_id, role, status, joined_at, invited_by)` с PK `(user_id, account_id)`. `role ∈ {admin, operator, viewer}`. `status ∈ {active, disabled}` | P0 | ☐ |
| F-202 | Email/логин пользователя **глобально уникальны** (как сейчас). Один email = один `users.id` = много `account_memberships` | P0 | ☐ |
| F-203 | UI: переключатель аккаунтов в шапке (если memberships > 1). API: `POST /api/v1/auth/switch-account` с телом `{"account_id": <id>}`, возвращает новый короткоживущий JWT (TTL = 1 час) с `active_account_id` в claims. Refresh через `POST /api/v1/auth/refresh` (refresh-токен не вводим, используем текущий JWT до его истечения) | P0 | ☐ |
| F-204 | Membership-проверка на **каждом** запросе: middleware читает `X-Account-Id` (или query `?account_id=`), убеждается что `membership.status='active'` и `users.disabled=false`; иначе `401/403` | P0 | ☐ |
| F-205 | Запрет удаления последнего `admin` в аккаунте (HTTP 409 `last_admin`) | P0 | ☐ |
| F-206 | При смене роли/удалении membership все активные JWT пользователя с `active_account_id == этот аккаунт` остаются валидными до истечения, но следующий запрос даст 403 (см. F-204) | P0 | ☐ |
| F-207 | Каждое API-токен (`api_tokens`) создаётся **per-account**: добавляем `account_id`; при вызове токен применяется только в своём аккаунте. Старые токены (если были) трактуются как принадлежащие аккаунту пользователя (best-effort) | P0 | ☐ |
| F-208 | `GET /me` возвращает `[{account_id, account_slug, account_name, role, status}]` | P0 | ☐ |

### 5.3 Приглашения

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-301 | `POST /api/v1/accounts/{id}/invitations` (admin) — создаёт `account_invitations(token_hash, account_id, email, role, expires_at, invited_by, status='pending')`. Возвращает одноразовую ссылку `{PUBLIC_BASE_URL}/invite/{token}` | P0 | ☐ |
| F-302 | Email + письмо: используем тот же `internal/mail` (smtp / log режимы), что и для verify-email | P0 | ☐ |
| F-303 | TTL приглашения: 72 часа (конфиг `INVITE_TTL_HOURS`, default 72) | P0 | ☐ |
| F-304 | Получатель: `GET /api/v1/invitations/{token}` (публичный, по токену) — возвращает `account_name`, `role`, `email`. `POST /api/v1/invitations/{token}/accept` — если пользователь залогинен: создаёт `account_memberships`; если нет: возвращает `redirect_to_register: true` с pre-filled email и deep-link на `/auth/register?invite=<token>` | P0 | ☐ |
| F-305 | Повторное `accept` уже принятого приглашения — идемпотентно возвращает membership. Невалидный/истёкший токен — `404 invite_not_found` | P0 | ☐ |
| F-306 | Resend: `POST /api/v1/accounts/{id}/invitations/{id}/resend` (admin) — генерирует новый токен, помечает старый `revoked` | P1 | ☐ |
| F-307 | Revoke: `DELETE /api/v1/accounts/{id}/invitations/{id}` (admin) | P1 | ☐ |
| F-308 | Audit: `invite.create`, `invite.accept`, `invite.resend`, `invite.revoke` пишутся в `audit_log` с `entity_type='invitation'`, `entity_id=token_prefix` | P0 | ☐ |

### 5.4 Изоляция ресурсов

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-401 | Все запросы `GET/PUT/PATCH/DELETE /products|devices|…/{id}` — middleware проверяет, что `entity.account_id == active_account_id` (или actor is_platform_admin). Нарушение → `404 not_found` (НЕ `403`, чтобы не leak'ать существование) | P0 | ☐ |
| F-402 | Списки (`GET /products`, `GET /devices`, `GET /events`, `GET /logs`, `GET /mqtt/clients`, `GET /audit_log`) фильтруются по `account_id` на уровне SQL | P0 | ☐ |
| F-403 | WebSocket `/api/v1/ws` — payload событий фильтруется на стороне сервера по активным memberships соединения. Клиент видит события **только** своего активного аккаунта | P0 | ☐ |
| F-404 | MCP: инструменты принимают `account_id` (опциональный; default — active). Cross-account доступ — только для `is_platform_admin`. Параметр `account_id` в каждом list/get-туле | P0 | ☐ |
| F-405 | `GET /overview` возвращает counts **по активному аккаунту**: `online_devices`, `offline_devices`, `total_devices`, `recent_events_24h` | P0 | ☐ |
| F-406 | Telemetry/Events/Logs retention (TELEMETRY_RETENTION_DAYS) применяется **per-account** — старые записи удаляются независимо | P0 | ☐ |

### 5.5 IAM: JWT и авторизация

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-501 | `auth.Claims` расширяется: добавляются `Accounts []AccountRef` (`{account_id, account_slug, role}`), `ActiveAccountID int64`, `IsPlatformAdmin bool` | P0 | ☐ |
| F-502 | `auth.IssueJWT(secret, user, activeAccountID, ttl)` — формирует claims; `active_account_id` обязателен (если у пользователя 0 memberships — `login` не пускает) | P0 | ☐ |
| F-503 | `auth.RequireAccount(next)` middleware: парсит `X-Account-Id` → `active_account_id`. Если header отсутствует — берётся `active_account_id` из JWT. Если `X-Account-Id != claims.active_account_id` — `403 account_mismatch` | P0 | ☐ |
| F-504 | `auth.CanWrite(role)` остаётся — admin или operator в активном аккаунте | P0 | ☐ |
| F-505 | `auth.CanAdmin(role)` остаётся — admin в активном аккаунте; для platform-admin действий — отдельный `auth.CanPlatformAdmin(claims)` | P0 | ☐ |
| F-506 | API-токены (`api_tokens`): при создании (`POST /api/v1/me/tokens`) клиент передаёт `account_id`; токен валиден только в нём. `GET /me/tokens` — список per-account | P0 | ☐ |
| F-507 | Старые JWT без `active_account_id` после апгрейда — не принимаются, клиент должен заново сделать login | P0 | ☐ |

### 5.6 Внешние плагины: account-scoping

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-601 | `external_plugins.account_id NOT NULL`. Уникальность `(account_id, id)` | P0 | ☐ |
| F-602 | `mqtt_user` строится как `extplg_{account_slug}_{plugin_id}` — гарантированно уникально и однозначно определяет аккаунт. Старая схема `extplg_{plugin_id}` не используется | P0 | ☐ |
| F-603 | ACL внешнего плагина в `internal/mqtt/hook.go` (метод `aclOK`): для пользователя `extplg_{slug}_*` — разрешены топики **только** `platform/v1/{events|commands}/{slug}/#`. Любой другой префикс (включая `platform/v1/events/other-slug/...`) → `acl_deny` | P0 | ☐ |
| F-604 | Регистрация (`POST /api/v1/plugins/external`): `account_id` берётся из активного membership; API-токен и MQTT-пароль остаются одноразовыми | P0 | ☐ |
| F-605 | Cross-account RPC `platform.invoke_rpc`: `InvokeRPCForPlugin` сверяет `plugin.account_id == device.account_id` (через product). Cross-account RPC → `403` | P0 | ☐ |
| F-606 | `GetPluginIDByToken` и `ValidateMQTTCreds` остаются hash-based, но индекс по `(account_id, id)` используется для сужения поиска | P0 | ☐ |

### 5.7 Системные топики платформы (правила)

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-701 | **Breaking change** формата `platform/v1/{events|commands}/...`: добавляется сегмент `account_slug` сразу после корня. **Новые** топики:<br>• `platform/v1/events/{account_slug}/{product_id}/{device_id}/{kind}`<br>• `platform/v1/commands/{account_slug}/{product_id}/{device_id}/{kind}` | P0 | ☐ |
| F-702 | `internal/protocol/topics.go`: новые константы `SegAccount`, хелперы `RulesEventsRootForAccount(prefix, slug)` и `RulesCommandsRootForAccount(prefix, slug)` | P0 | ☐ |
| F-703 | `publishRules` в `internal/services/ingest.go` / `platform.go` принимает `account_slug`; подписка `OnPublished` в `hook.go` игнорирует события, чей `account_slug` не совпадает с `device.account_slug` (защита от подделки — топик publish'ится платформой, но дополнительная проверка не повредит) | P0 | ☐ |
| F-704 | *(удалён в v1.1 — `RULES_BUS_INCLUDE_LEGACY_PREFIX` отменён. Новый формат топиков — hard break; downstream-сервисы обязаны обновиться одновременно с платформой)* | — | — |
| F-705 | Все downstream-сервисы (Node-RED, gRPC, webhook) должны обновить правила — README и шаблоны `flows.json` обновляются в этом же релизе | P0 | ☐ |
| F-706 | Девайсные топики `iot/v1/{product}/{device}/...` и `v1/...` **не меняются** — устройства не знают про аккаунты. Идентификация устройства = `{product}.{device}` + token. Token уникален глобально (`sha256`), поэтому `ResolveByToken` однозначно находит device → account. Подробности — [§5.7.1](#571-что-не-меняется-в-топиках-устройств) | P0 | ☐ |

#### 5.7.1 Что не меняется в топиках устройств

Этот блок вынесен отдельно, потому что вопрос повторяется на каждом ревью.

| Слой | Формат | Меняется? |
|------|--------|-----------|
| Device → Platform (telemetry) | `iot/v1/{product_id}/{device_id}/telemetry/{key}` | **Нет** |
| Device → Platform (attributes) | `iot/v1/{product_id}/{device_id}/attributes` | **Нет** |
| Device → Platform (logs) | `iot/v1/{product_id}/{device_id}/logs` | **Нет** |
| Device → Platform (RPC) | `iot/v1/{product_id}/{device_id}/rpc/{method}` | **Нет** |
| Platform → Device (RPC) | `iot/v1/{product_id}/{device_id}/rpc/{method}/response` | **Нет** |
| Platform → Device (config) | `iot/v1/{product_id}/{device_id}/config` | **Нет** |
| Platform → Device (OTA) | `iot/v1/{product_id}/{device_id}/ota` | **Нет** |
| Short style (тоже устройства) | `v1/{product_id}/{device_id}/...` | **Нет** |

**Меняется только** `platform/v1/{events|commands}/...` — это **rules bus** для Node-RED, gRPC-плагинов, webhooks и автоматизации; туда добавляется сегмент `account_slug` сразу после корня (F-701). Устройства этот топик никогда не публикуют и не читают.

Следствие: **firmware не требует обновления** для работы с multi-account. Достаточно того, чтобы платформа выдала устройству валидный token. `product_id` + `device_id` остаются глобально уникальными в пределах аккаунта (но допускается коллизия `product_id` между аккаунтами — см. F-105).

### 5.8 Управление внутренними плагинами

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-801 | `account_plugin_policies(account_id, plugin_id, enabled, settings_json, updated_at)` с PK `(account_id, plugin_id)`. По умолчанию записи нет → `enabled=true` (как сейчас) | P0 | ☐ |
| F-802 | API: `GET /api/v1/accounts/{id}/plugins/policies` (admin) — список всех каталожных плагинов с effective enabled и overrides | P0 | ☐ |
| F-803 | API: `PATCH /api/v1/accounts/{id}/plugins/policies/{pluginId}` (admin) — тело `{enabled?: bool, settings?: json}`. Изменения применяются «на лету» — hub перебирает bindings аккаунта и останавливает/запускает runners | P0 | ☐ |
| F-804 | Hub `services.PluginHub.Dispatch` и `ApplyConfig` скипают bindings отключённых плагинов (не вызывают `OnEvent`); device-tick их не будит | P0 | ☐ |
| F-805 | Каталожные плагины (led-garland, schedule) уважают policy; внешние плагины (зарегистрированные через API) — **не** управляются этой таблицей (у них свой account-scope) | P0 | ☐ |
| F-806 | UI: Settings → Account → **Integrations** (доступно **только admin**): список каталожных плагинов с тумблером enabled/disabled и кнопкой «Settings» (открывает form-driven редактор `Descriptor.Form`). Текущая реализация: в `SettingsPage.tsx` уже есть секция External Plugins (admin-only); секция Integrations для каталожных плагинов — **новая**, добавляется в v1.4 | P0 | ☐ |

### 5.9 Внутренний плагин `prometheus-exporter`

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-901 | Каталожный плагин `prometheus-exporter` в `internal/plugins/prometheus_exporter.go`; `Register` в `init()` | P0 | ☐ |
| F-902 | Конфиг binding: `port` (default 9100), `bind` (default `0.0.0.0`), `path` (default `/metrics`), `bearer_token` (опц. — для защиты scrape-эндпоинта), `metrics` (список активных метрик), `update_interval_ms` (default 5000) | P0 | ☐ |
| F-903 | Плагин поднимает **локальный HTTP-сервер** на `(bind, port)` в момент, когда хотя бы один binding активирован в активном аккаунте. Общий сервер на (bind, port) с лейблом `account` — каждый account использует свой port, чтобы лейблы не перемешивались. Если все bindings аккаунта выключены — сервер гасится | P0 | ☐ |
| F-904 | Поддерживаемые метрики (в Prometheus exposition format):<br>• `cogitor_devices_total{account,product,status}` (gauge) — пересчёт раз в `update_interval_ms`<br>• `cogitor_device_online{account,product,device}` (gauge, 0/1)<br>• `cogitor_device_telemetry_samples_total{account,product,device}` (counter, инкремент в `StoreTelemetry`)<br>• `cogitor_device_rpc_calls_total{account,product,device,method,status}` (counter)<br>• `cogitor_device_ota_jobs_total{account,product,device,status}` (counter)<br>• `cogitor_event_bus_published_total{account,product,device,kind}` (counter)<br>• `cogitor_mqtt_clients{account,kind}` (gauge)<br>• `cogitor_external_plugin_online{account,plugin_id}` (gauge, 0/1) | P0 | ☐ |
| F-905 | `bearer_token` проверяется через `Authorization: Bearer <token>`; отсутствие или неверный токен → `401`. Не задан → эндпоинт открыт (как у обычного node_exporter) | P0 | ☐ |
| F-906 | Per-binding `metrics: ["devices", "rpc", "ota", "telemetry", "events", "bus", "external_plugins"]` — пустой список = выключить всё. `update_interval_ms` определяет частоту пересчёта gauge-метрик | P1 | ☐ |
| F-907 | Плагин `prometheus-exporter` управляется через `account_plugin_policies` (F-801). Если admin выключил — HTTP-сервер останавливается, метрики не отдаются | P0 | ☐ |
| F-908 | Примеры scrape-конфига для внешнего Prometheus публикуются в `docs/en/external-prometheus.md` / `docs/ru/external-prometheus.md` | P1 | ☐ |

### 5.10 Внутренний плагин `metrics-dashboard` (шаблон визуализации)

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-1001 | Каталожный плагин `metrics-dashboard` в `internal/plugins/metrics_dashboard.go`. Не стартует HTTP-сервер; только поставляет UI-шаблон и агрегатор данных | P0 | ☐ |
| F-1002 | Конфиг binding (`device_plugin_bindings.config_json`): `panels: [{type, title, query, ...}]`, где `type ∈ {telemetry_chart, telemetry_gauge, rpc_table, online_timeline, events_count}` | P0 | ☐ |
| F-1003 | Плагин регистрирует **виртуальный таб** в странице устройства — `Metrics+` рядом со стандартным `Metrics`. На вкладке рендерятся панели по конфигу | P0 | ☐ |
| F-1004 | Агрегатор: `OnEvent("apply")` перечитывает `telemetry_samples` и `rpc_calls` для `device_pk`; обновляет `GET /api/v1/devices/{id}/dashboard` (новый эндпоинт), отдающий `panels: [{id, type, title, data}]` — frontend просто рендерит | P0 | ☐ |
| F-1005 | `telemetry_chart`: `{keys: ["temperature","humidity"], window: "1h"\|"24h"\|"7d"}` → `[{key, points: [[ts, value]]}]` | P0 | ☐ |
| F-1006 | `telemetry_gauge`: `{key, window}` → `{key, current, min, max, avg}` | P0 | ☐ |
| F-1007 | `rpc_table`: `{limit}` → последние N RPC-вызовов | P0 | ☐ |
| F-1008 | `online_timeline`: `{window}` → `[[ts_start, ts_end, online_bool]]` сегменты | P1 | ☐ |
| F-1009 | `events_count`: `{severity?, window}` → `{critical, error, warning, info}` | P1 | ☐ |
| F-1010 | Шаблон (default binding) — стартовая страница с типичными панелями: `telemetry_chart` для `wifi_rssi` + `telemetry_gauge` для `free_heap` + `online_timeline` за 24h. Админ может кастомизировать через UI Settings → Device → Plugins → metrics-dashboard | P0 | ☐ |

### 5.11 Дашборд

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-1101 | Switcher аккаунтов в шапке (dropdown). Активный аккаунт визуально выделен. Меню «Account settings» ведёт в настройки текущего | P0 | ☐ |
| F-1102 | Settings → Account: имя, slug (read-only после создания), статус (`active`/`suspended`), timezone, retention, integrations (см. F-806), members (см. F-201) | P0 | ☐ |
| F-1103 | Settings → Members: список пользователей аккаунта с ролью и статусом; действия — invite, change role, disable, remove (с запретом last admin) | P0 | ☐ |
| F-1104 | Settings → Invitations: pending/accepted/revoked/expired, resend, revoke | P0 | ☐ |
| F-1105 | Если активный аккаунт не выбран или membership удалена — главная страница показывает «Choose account» | P0 | ☐ |
| F-1106 | Sidebar/footer показывает `account_slug` маленьким шрифтом как подсказку контекста | P2 | ☐ |

### 5.12 Удаление аккаунта

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-1201 | `DELETE /api/v1/accounts/{id}?force=true` (platform-admin). Без `force=true` — только если `devices.count=0` и `external_plugins.count=0` | P0 | ☐ |
| F-1202 | Каскадно: products, devices, plugin bindings, firmwares, ota_jobs, telemetry, events, logs, rpc, audit, memberships, invitations, plugin policies | P0 | ☐ |
| F-1203 | Все активные MQTT-сессии устройств аккаунта дропаются через `MQTT.DropUsername` (по списку `product.device`) | P0 | ☐ |
| F-1204 | Внешние плагины аккаунта — регистрация удаляется, heartbeat monitor снимается; подписчикам шлём `external_plugin.deleted` через rules bus (last will) | P0 | ☐ |
| F-1205 | `audit_log` сохраняется в течение 30 дней после удаления, потом чистится. Запись об удалении сама удаляется с retention | P1 | ☐ |

### 5.13 Драйвер БД: SQLite и Postgres

Платформа поддерживает **два драйвера БД** через единый `database/sql`-слой. Выбор драйвера — на старте, через env; в runtime не переключается.

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-1301 | `DATABASE_DRIVER ∈ {sqlite, postgres}`. По умолчанию `sqlite`. Если пусто — определяется автоматически: если `DATABASE_DSN` начинается с `postgres://` или `postgresql://` → `postgres`, иначе `sqlite` | P0 | ☐ |
| F-1302 | `DATABASE_DSN` — строка подключения. Для sqlite: путь к файлу (например `data/platform.db`). Для postgres: URL (`postgres://user:pass@host:5432/dbname?sslmode=disable`) | P0 | ☐ |
| F-1303 | Драйверы: `modernc.org/sqlite` (pure Go, без CGo) — уже в `go.mod`; `github.com/jackc/pgx/v5/stdlib` (pure Go, даёт `database/sql`-совместимый коннект) — добавляется в `go.mod` | P0 | ☐ |
| F-1304 | Миграции через `github.com/golang-migrate/migrate/v4`. Драйверы: `migrate/v4/database/sqlite3` (через `modernc.org/sqlite`) и `migrate/v4/database/postgres` (через `pgx`). Один источник миграций — `internal/db/migrations/`, без `database-specific` папок | P0 | ☐ |
| F-1305 | Абстракция `services.ScopeAccount(db, accountID)` — обязательная обёртка для всех per-account SQL-запросов. На уровне типов — `db.DB` интерфейс (`Query/QueryRow/Exec`); плейсхолдер `$N` (postgres) и `?` (sqlite) разводятся через `db.Rebind(query)` (helper из `sqlx` или собственный) | P0 | ☐ |
| F-1306 | **Один и тот же набор smoke-тестов** запускается против обеих БД (через `make smoke-sqlite` и `make smoke-postgres`). CI прогоняет оба. Postgres поднимается в `docker-compose.test.yml` рядом с основным compose | P0 | ☐ |
| F-1307 | `FOREIGN KEY` в SQLite включён через `PRAGMA foreign_keys=ON` на каждое соединение; в Postgres — по умолчанию | P0 | ☐ |
| F-1308 | `AUTOINCREMENT` (sqlite) ↔ `SERIAL`/`GENERATED ALWAYS AS IDENTITY` (postgres) — выбирается через миграции per-driver, если расходятся; общий код работает через `db.LastInsertId` / `RETURNING id` | P0 | ☐ |
| F-1309 | `WAL` режим SQLite включается через `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)` в DSN. Postgres — `synchronous=normal` через серверный конфиг (вне платформы) | P1 | ☐ |
| F-1310 | При подключении к Postgres платформа пишет в лог: `using driver=postgres, server_version=X.Y, current_user=...`. Помогает диагностировать «почему внезапно миграция пошла не туда» | P1 | ☐ |
| F-1311 | `convert` (смена драйвера на лету) — **не поддерживается**. Документировано в `11-operations.md`: для перехода с sqlite на postgres — экспорт через `account export` (backlog) → пересоздание схемы → импорт | P1 | ☐ |

### 5.14 Мастер первой установки

Полный визард первой установки через веб. Заменяет env-only bootstrap; `.env` остаётся опциональным (значения из env имеют приоритет, визард подставляет только то, что не задано).

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-1401 | Маршрут `GET /setup` — публичный, доступен **только** пока таблица `accounts` пуста. Как только `accounts.count > 0` — маршрут отключается навсегда (404). Внутри — SPA-страница `SetupWizard.tsx` | P0 | ☐ |
| F-1402 | Шаг 1: **БД**. Если `DATABASE_DRIVER/DSN` заданы — поля pre-filled и read-only. Иначе — radio: SQLite (default) / Postgres + поле DSN. Кнопка «Test connection» делает `db.PingContext`; без успешного пинга — следующий шаг недоступен | P0 | ☐ |
| F-1403 | Шаг 2: **Bootstrap admin**. Поля: `login`, `email`, `password` (с policy: min 8, буква+цифра), `display_name`. Если `ADMIN_LOGIN/EMAIL/PASSWORD` в env — pre-filled, но не read-only. Email-верификация для bootstrap-админа — отключаемая (`REQUIRE_EMAIL_VERIFY=false` пропускает шаг) | P0 | ☐ |
| F-1404 | Шаг 3: **Дефолтный аккаунт**. Поля: `slug` (default `default`, regex `[a-z0-9][a-z0-9-]{1,30}`), `name` (default `Default`), `timezone` (IANA), `retention_days` (default 30). Slug pre-filled из `BOOTSTRAP_ACCOUNT_SLUG` если задан | P0 | ☐ |
| F-1405 | Шаг 4: **SMTP**. Если `EMAIL_MODE=log` или всё уже в env — экран «Skip» доступен. Иначе поля: `host`, `port`, `username`, `password`, `from`, `mode ∈ {smtp, log}`. Кнопка «Send test» шлёт письмо на указанный email — без успешной отправки следующий шаг недоступен (если mode=smtp) | P0 | ☐ |
| F-1406 | Шаг 5: **Безопасность**. `JWT_SECRET` — если задан в env, поле скрыто/disabled. Иначе — генерируется автоматически (32 байта base64) с кнопкой «Regenerate». Кнопка «Copy to clipboard» | P0 | ☐ |
| F-1407 | Шаг 6: **MQTT TLS** *(опционально)*. Radio: `Disabled` (default) / `Self-signed` / `Custom`. Self-signed — генерируем через встроенный `crypto/tls` (Subject CN = `BROKER_HOST_PUBLIC`). Custom — пользователь загружает cert+key | P1 | ☐ |
| F-1408 | Шаг 7: **Public URL**. Pre-filled из `PUBLIC_BASE_URL` если задан. Используется для генерации invite-ссылок. Валидация: должен начинаться с `http://` или `https://` | P0 | ☐ |
| F-1409 | Шаг 8: **Review + Apply**. Экран показывает сводку всех введённых значений. Кнопка «Apply» — записывает `setup_state` (json) в `accounts.settings_json` записи `default`, применяет все настройки, мигрирует схему, создаёт admin, выставляет `is_platform_admin=true`. После успешного apply — redirect на `/auth/login` | P0 | ☐ |
| F-1410 | Все шаги можно пройти повторно до Apply (state хранится в `localStorage`). При выходе из визарда до Apply — состояние сбрасывается при следующем заходе (никаких частично заполненных записей в БД до Apply) | P0 | ☐ |
| F-1411 | Визард доступен **только через HTTP** (без CSRF, без auth). Защита: проверка `accounts.count == 0` на каждый запрос. После первого Apply — `accounts.count > 0` → маршрут отключается | P0 | ☐ |
| F-1412 | Логирование: все попытки Apply пишутся в `audit_log` с `actor_kind='setup'`. Неуспешные — с `last_error`. Если Apply упал посередине — следующая попытка переигрывает с начала (idempotency через transaction) | P0 | ☐ |
| F-1413 | Конфиг сохраняется в `accounts.settings_json` (для runtime-параметров визарда) и в `.env` (опционально, через блок `WIZARD_WRITE_ENV=true` если хочется persistent). По умолчанию `.env` **не** модифицируется | P1 | ☐ |
| F-1414 | `CLI fallback`: команда `platform setup --from-yaml=config.yml` — неинтерактивный Apply для CI/CD. YAML-формат — отдельная схема в `11-operations.md` | P1 | ☐ |

### 5.15 Automation page: депрекейс + документация Node-RED

Текущая страница `/automation` — это iframe, встроенный Node-RED. Такой подход имеет проблемы: Node-RED не всегда доступен по URL из контейнера платформы, iframe не даёт контекста пользователю, а сама страница не несёт функциональной нагрузки сверх «открыть Node-RED в новой вкладке».

**Решение:** убрать Automation page из навигации, заменить на документацию по интеграции с Node-RED + inline help в UI.

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-1501 | Убрать маршрут `/automation` и `AutomationPage.tsx` из роутера (`App.tsx`) и навигации (`AppShell.tsx` NAV-массив). Ссылка «Automation» в sidebar удаляется | P1 | ☐ |
| F-1502 | Новая страница `/docs/automation` (или раздел в Settings) — **руководство по интеграции с Node-RED**: как подключить Node-RED к MQTT broker, какие топики использовать (`platform/v1/events/{account_slug}/...`, `platform/v1/commands/{account_slug}/...`), примеры flows.json, troubleshooting. Контент — на русском и английском | P1 | ☐ |
| F-1503 | **Inline help в UI**: на странице Settings (для admin) — раздел «Automation & Integrations» с кратким описанием rules bus, ссылкой на полную документацию (`/docs/automation`), кнопкой «Copy MQTT topics» (копирует шаблон топиков для текущего аккаунта в clipboard). Для operator/viewer — раздел скрыт (automation — admin-only функциональность) | P1 | ☐ |
| F-1504 | **Frontend documentation**: в самом UI — tooltips и help-иконки (ⓘ) рядом с ключевыми действиями: создание external plugin, настройка rules bus, привязка устройства к плагину. Текст tooltip — краткий (1-2 предложения), со ссылкой на полную документацию. Реализация — через `title` attribute или Bootstrap tooltip | P2 | ☐ |
| F-1505 | Обновить `06-automation.md` / `06-automation.ru.md`: убрать описание iframe-страницы, добавить пошаговое руководство по настройке Node-RED с учётом multi-account (новые топики с `{account_slug}`, ACL для external plugins) | P1 | ☐ |

---

## 6. Нефункциональные требования

| ID | Категория | Требование |
|----|-----------|------------|
| NF-01 | **Изоляция** | SQL-запросы ко всем per-account таблицам обязаны фильтровать по `account_id` — реализуем через обёртку `services.ScopeAccount(db, accountID)` (unit-тест на каждую модель: «запрос без фильтра возвращает 0 строк») |
| NF-02 | **Изоляция** | Случайная утечка на listing-эндпоинте (отсутствие WHERE) считается багом P0 и блокирует релиз |
| NF-03 | **Безопасность** | MQTT username `extplg_*` — формат фиксирован, валидация на регистрации и ACL на каждом пакете |
| NF-04 | **Производительность** | `account_memberships` — индекс `(user_id, account_id)` PK, `idx_memberships_account` |
| NF-05 | **Производительность** | `products` — `UNIQUE(account_id, product_id)`, индекс `(account_id, product_id)` |
| NF-06 | **Производительность** | `devices` — `idx_devices_account` плюс текущие `idx_devices_status`/`idx_devices_token_prefix` |
| NF-07 | **Производительность** | `prometheus-exporter` HTTP-сервер не блокирует hub: handlers используют `atomic` счётчики и read-only DB-запросы через пул `SetMaxOpenConns(2)` |
| NF-08 | **Конфигурация** | ENV: `DATABASE_DRIVER=sqlite`, `DATABASE_DSN=data/platform.db`, `INVITE_TTL_HOURS=72`, `WIZARD_AUTO_DISABLE=true` |
| NF-09 | **Совместимость** | Новая установка стартует сразу на multi-account схеме; БД пересоздаётся вручную при переходе (миграционных утилит нет). Миграции — только forward, через `golang-migrate` |
| NF-10 | **Observability** | `audit_log` уже есть — добавляем `account_id` и `actor_kind` (`user`/`platform_admin`/`api_token`/`setup`); structured logs включают `account_id` где возможно |
| NF-11 | **UI** | Account switcher, Members, Invitations, Account Settings, Integrations, Setup Wizard — новые страницы в SPA, добавлены в роутер |
| NF-12 | **Документация** | Обновить `02-api.md`, `03-auth.md`, `04-mqtt.md`, `06-automation.md` (убрать iframe, добавить руководство по Node-RED + multi-account), `07-internal-plugins.md`, `08-external-plugins.md`, `11-operations.md` и `examples/plugins/*` (MQTT subscribe/put под новые топики) |
| NF-13 | **DB** | `golang-migrate` версия: `>=4.17`. Все миграции должны быть reversible (иметь `down`); CI проверяет `make migrate-down && make migrate-up` на пустой БД обоих драйверов |
| NF-14 | **DB** | Плейсхолдеры: только через `db.Rebind()` или `?` с явным `?`-style (если пишем чисто для sqlite, ставим `// driver:sqlite` в комментарии). Линтер `go vet` + `staticcheck` ловит прямые `fmt.Sprintf` в SQL |
| NF-15 | **DB** | Транзакции: пишем `db.WithTx(ctx, func(tx) {...})` — helper, который корректно откатывает при panic и логирует при ошибке. Не использовать `db.Begin()` напрямую в хендлерах |
| NF-16 | **DB** | Схема `schema_migrations` от `golang-migrate` — единственный источник правды о версии БД. Никаких собственных `migrations` таблиц |
| NF-17 | **Wizard** | `/setup` маршрут **не** отдаёт никакой информации о других эндпоинтах, пока `accounts` пуста (т.е. не показывает «Dashboard», «Devices» и т.п. в навигации до Apply) |
| NF-18 | **Wizard** | Все env-переменные с приоритетом выше, чем UI-значения визарда. Это значит: если `JWT_SECRET` задан в env, визард вообще не запрашивает его; если `DATABASE_DSN` задан — Step 1 read-only |
| NF-19 | **HA-readiness** | Все SQL-запросы — параметризованные. Все `goroutine` — `context.Context`-aware. Все `account_id` — `INTEGER`, не UUID. Никаких глобальных `sync.Mutex` на per-account ресурсы. Подробности — [§7.7](#77-архитектурные-ограничения-v1-для-будущей-кластеризации) |
| NF-20 | **Тесты** | Integration-смоук (devices publish → telemetry stored → ACL scope check → audit log written) запускается на **обоих** драйверах. Покрытие ACL-кейсов (positive + negative) — 100% в `internal/mqtt/acl_test.go` |
| NF-21 | **UI/UX** | Любой `4xx` от сервера, связанный с `account_id` (mismatch / not found / suspended), даёт **читаемое** сообщение в UI («Это устройство принадлежит другому аккаунту — переключите аккаунт»), а не сырой JSON |
| NF-22 | **UI/Modern** | Визуальное обновление UI: пересмотр цветовой схемы, типографики, отступов и компонентов в `theme.scss`. Цель — современный, чистый вид при сохранении Bootstrap 5 как основы. Конкретика: карточки с тенями/скруглениями, улучшенные контрасты для dark theme, консистентные размеры кнопок и инпутов, иконки (SVG inline или icon font) вместо текстовых лейблов где уместно |
| NF-23 | **UI/Mobile** | Mobile-first подход сохраняется и усиливается: все новые страницы (Setup Wizard, Members, Invitations, Integrations, Account Settings) обязаны корректно отображаться на экранах ≥360px. Таблицы — responsive (горизонтальный скролл или card-view на мобильных). Формы — одноколоночные на мобильных, многоколоночные на desktop. Offcanvas-навигация уже работает — новые пункты добавляются в `NAV`-массив `AppShell.tsx` |
| NF-24 | **UI/Inline help** | Все admin-only действия (создание external plugin, управление internal plugins, приглашения, account settings) имеют tooltips или help-иконки (ⓘ) с кратким описанием (1-2 предложения) и ссылкой на полную документацию. Реализация — Bootstrap tooltip или `title` attribute. Текст — на русском и английском |

---

## 7. Архитектура

> **Диаграммы**: [Multi-Account Architecture](14-diagrams.md#multi-account-architecture), [Invitation Flow](14-diagrams.md#invitation-flow), [External Plugin Account ACL](14-diagrams.md#external-plugin-account-acl), [Prometheus Exporter Lifecycle](14-diagrams.md#prometheus-exporter-lifecycle), [Internal Plugin Policy](14-diagrams.md#internal-plugin-policy)

### 7.1 Общая схема

```mermaid
graph TB
    subgraph Clients["Клиенты"]
        U1["Пользователь A<br/>(admin в acme)"]
        U2["Пользователь B<br/>(operator в beta, viewer в acme)"]
        P1["External Plugin<br/>acme/telegram-bot"]
        P2["External Plugin<br/>beta/zapier"]
    end

    subgraph Platform["Cogitor IoT Platform (single binary)"]
        subgraph Auth["Auth & Tenancy"]
            JWT["JWT (active_account_id)"]
            IAM["AccountMembership<br/>Middleware"]
        end

        subgraph Broker["MQTT Broker"]
            B1["mochi-mqtt<br/>per-username ACL"]
        end

        subgraph Core["Core"]
            ACL["Device ACL<br/>(product.device)"]
            ING["Ingest + publishRules"]
            HUB["PluginHub<br/>(per-account policy)"]
        end

        subgraph Plugins["Catalog"]
            LG["led-garland"]
            SCH["schedule"]
            PRO["prometheus-exporter<br/>(HTTP :9100)"]
            MD["metrics-dashboard<br/>(UI tab)"]
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

    %% Clients → Auth
    U1 & U2 -->|"Bearer JWT"| JWT
    P1 & P2 -->|"Bearer token"| JWT

    %% Auth → Core
    JWT --> IAM
    IAM --> ACL
    IAM --> ING
    IAM --> HUB

    %% Devices to broker (not shown to keep diagram compact)
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

    %% External plugins → Broker ACL
    P1 -->|"MQTT extplg_acme_telegram-bot"| B1
    P2 -->|"MQTT extplg_beta_zapier"| B1

    style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
    style Auth fill:#0f3460,stroke:#16213e,color:#eee
    style Broker fill:#0f3460,stroke:#16213e,color:#eee
    style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Plugins fill:#16213e,stroke:#1a1a2e,color:#eee
    style DB fill:#16213e,stroke:#1a1a2e,color:#eee
    style Bus fill:#0f3460,stroke:#16213e,color:#eee
    style Clients fill:#16213e,stroke:#1a1a2e,color:#eee
```

### 7.2 Поток данных (с учётом tenancy)

1. **Login**: пользователь шлёт `POST /auth/login` → платформа проверяет пароль → читает все `memberships` → возвращает JWT с `accounts[]` и `active_account_id=первый_admin_or_first`.
2. **Запрос в API**: `GET /devices/42` с `Authorization: Bearer ...` и `X-Account-Id: 7` → middleware проверяет `membership(user, 7, active)` → handler читает `device` → проверяет `device.account_id == 7` → отдаёт 200.
3. **Устройство → платформа**: `CONNECT` с `username=esp32-cam.cam-001`, `password=<token>`. `OnConnectAuthenticate` вызывает `deviceFor(user)` → `ResolveByToken` → находит device → берёт `product.account_id` → ACL scoped к `iot/v1/{product}/{device}/...`. Account — прозрачен для устройства.
4. **Publish telemetry** → `Ingest` → `StoreTelemetry` (account_id из device) → `publishRules(account.slug, product, device, kind)` → MQTT `platform/v1/events/{account_slug}/{product}/{device}/{kind}`. Топик содержит `account_slug` — внешний плагин другого аккаунта через ACL не подпишется.
5. **External plugin (polling)**: `POST /api/plugins/external/rpc` с токеном `extplg_acme_telegram-bot` → `ValidateToken` → `plugin.account_id = 1` → `platform.invoke_rpc` → middleware `RequireAccount(account_id=1)` → handler проверяет `device.account_id == 1` → 200. Cross-account → 403.
6. **WebSocket**: `/api/v1/ws?token=...` с активным аккаунтом 7 → Hub подписывает соединение на фильтр `account_id=7` → события других аккаунтов не приходят.
7. **Prometheus scrape**: внешний Prometheus с job `{account: "acme", scrape_url: "http://platform:9100/metrics"}` — на аккаунт свой порт (`prometheus-exporter` стартует инстанс на binding-defined port, все метрики с лейблом `account="acme"`).

### 7.3 Поток приглашений

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

### 7.4 Account ACL внешнего плагина

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

### 7.5 Жизненный цикл Prometheus Exporter

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

### 7.6 Policy внутренних плагинов

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

### 7.7 Архитектурные ограничения v1 для будущей кластеризации

v1 — single instance (см. [§4.5](#45-план-релиза--этапы), v2.0). Но ряд архитектурных решений v1 уже сейчас делается так, чтобы не заблокировать переход к HA в v2. Этот раздел — **контракт** между v1 и v2.

| # | Решение v1 | Что это значит для v2 (HA) |
|---|------------|-----------------------------|
| 1 | DB-доступ только через `db.DB` интерфейс + `ScopeAccount(db, accountID)` | Замена `database/sql` на connection pool с per-account sharding не сломает код: интерфейс стабилен |
| 2 | `golang-migrate` как единственный механизм миграций | Миграции одинаковы для sqlite и postgres; добавление нового драйвера в v2 — это вопрос конфигурации, не кода |
| 3 | `mqtt_user` для external plugins — `extplg_{account_slug}_{plugin_id}` | Уникальность имени работает и в shared external broker — broker будет видеть те же имена, ACL в нём будет идентичен нашему |
| 4 | System topics содержат `account_slug` сразу после корня | В shared broker namespace не пересекается между аккаунтами — fan-out по slug'у остаётся корректным |
| 5 | Все `account_id` — `INTEGER`, без UUID, без per-tenant схем | Postgres row-level security и partition by account работают на этом столбце из коробки |
| 6 | Кэши `ScopeAccount`, `productForToken`, `membershipForUser` — обёрнуты в интерфейс `services.Cache` с `Invalidate(key)` | В v2 меняется in-memory реализация на Redis-backed, интерфейс остаётся |
| 7 | `prometheus-exporter` слушает на отдельном порту per binding | В v2 multi-instance — каждая реплика экспозит свой `/metrics` на своём порту; Prometheus scrape через service discovery (Consul/K8s) — известный паттерн |
| 8 | `audit_log` пишется синхронно в текущей транзакции | В v2 на Postgres это работает сразу; на sqlite — деградирует (один writer), но v1 это и так single-instance |
| 9 | WebSocket hub — per-instance in-memory, фильтр по `account_id` | В v2 — Redis pub/sub: `account:{id}:events` канал, hub-инстансы подписываются только на нужные каналы; интерфейс `Hub.Publish(accountID, event)` не меняется |
| 10 | Plugin runners — in-process goroutine, на каждый binding | В v2 — leader election: только лидер-инстанс запускает runner; followers принимают `OnEvent` через Redis stream; контракт `Plugin.OnEvent(ctx, evt)` не меняется |
| 11 | `MQTT` — in-process `mochi-mqtt` | В v2 — внешний broker; код не знает, какой именно брокер — `internal/mqtt.Broker` интерфейс остаётся |
| 12 | Все SQL-запросы идут через `db.DB` — нет прямого `sql.DB` в хендлерах | В v2 — read replica routing (read из `accounts` пишут в master, telemetry queries идут в read replica) — возможно точечно добавить `ReadDB()` метод, не ломая существующий код |

**Что точно НЕЛЬЗЯ делать в v1**, чтобы не заблокировать v2:

- ❌ Писать `db.QueryRow(... fmt.Sprintf("WHERE id=%d", id) ...)` — должен быть `Rebind`/параметризованные запросы
- ❌ Использовать `goroutine + time.Sleep` для retry без контекста — всё через `context.Context`, чтобы лидер-election в v2 корректно отменял
- ❌ Хранить `account_id` в JWT без `accounts[]` claim — нужен полный список, чтобы при v2 multi-instance не делать round-trip к БД на каждый запрос
- ❌ Глобальные мьютексы на per-account данные (например `sync.Mutex` поверх `account.ID`) — сломает горизонтальное масштабирование; используем локальные `sync.Map` / channel
- ❌ Считать, что `mochi-mqtt` — единственный брокер. `Broker` интерфейс с самого начала

**Документированные компромиссы v1** (явно):

- `goroutine leak` в in-memory кэшах — принимаем как known issue до v2 (метрика в `/internal/cache` показывает leak rate)
- Нет `circuit breaker` на запись в БД — в single-instance это не нужно
- `audit_log` без rate limit — в single-instance ОК, в v2 придётся добавить batcher

---

## 8. Модель данных (новые таблицы)

```sql
-- Accounts (tenants)
CREATE TABLE accounts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    slug         TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL,
    settings_json TEXT NOT NULL DEFAULT '{}',
    status       TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','suspended')),
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

-- Per-account user membership
CREATE TABLE account_memberships (
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    status      TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','disabled')),
    joined_at   INTEGER NOT NULL,
    invited_by  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY(user_id, account_id)
);
CREATE INDEX idx_memberships_account ON account_memberships(account_id, status);

-- Email invitations
CREATE TABLE account_invitations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id  INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    role        TEXT NOT NULL CHECK(role IN ('admin','operator','viewer')),
    token_hash  TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    invited_by  INTEGER NOT NULL REFERENCES users(id),
    status      TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','revoked','expired')),
    expires_at  INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    accepted_at INTEGER,
    accepted_user INTEGER REFERENCES users(id)
);
CREATE INDEX idx_invitations_account ON account_invitations(account_id, status);
CREATE INDEX idx_invitations_email ON account_invitations(email);

-- Per-account policy for catalog plugins
CREATE TABLE account_plugin_policies (
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    plugin_id     TEXT NOT NULL,
    enabled       INTEGER NOT NULL DEFAULT 1,
    settings_json TEXT NOT NULL DEFAULT '{}',
    updated_at    INTEGER NOT NULL,
    PRIMARY KEY(account_id, plugin_id)
);

-- Per-account API tokens
ALTER TABLE api_tokens ADD COLUMN account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE;

-- Account_id propagation: add column to all per-account tables
ALTER TABLE products        ADD COLUMN account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE;
ALTER TABLE external_plugins ADD COLUMN account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE;

-- Replace UNIQUE(product_id) on products with UNIQUE(account_id, product_id)
-- (migrated by convert-to-multi-account; new installs start with the new shape)

-- users: platform-admin flag for super-admins
ALTER TABLE users ADD COLUMN is_platform_admin INTEGER NOT NULL DEFAULT 0;
```

> **Существующие таблицы** (`devices`, `events`, `device_logs`, `telemetry_samples`, `rpc_calls`, `attribute_history`, `device_attributes_*`, `ingest_dedup`, `device_credentials`, `ota_firmwares`, `ota_jobs`, `audit_log`) получают `account_id NOT NULL` через миграцию, которая идёт в одном пакете с `convert-to-multi-account`. В новых установках — стартуют с этой колонкой.

---

## 9. Изменения в API (сводка)

| Группа | Эндпоинты | Заметка |
|--------|-----------|---------|
| Auth | `POST /auth/login` — теперь возвращает `accounts[]` и `active_account_id` | брейкинг для клиентов, которые парсили старый shape — minor version bump в URL `/api/v1` остаётся |
| | `POST /auth/switch-account` `{account_id}` → новый JWT | новый |
| | `POST /auth/refresh` — пере-выпускает JWT с тем же `active_account_id` (без refresh-токенов, переиспользует старый до истечения) | новый |
| | `GET /me` — расширенный | новые поля |
| | `GET /auth/config` — без изменений | |
| Accounts | `GET /accounts` — список доступных пользователю (или всех для platform-admin) | новый |
| | `POST /accounts` — platform-admin: создать аккаунт | новый |
| | `GET /accounts/{id}` | новый |
| | `PATCH /accounts/{id}` — admin/operator (operator — только name) | новый |
| | `DELETE /accounts/{id}?force=true` — platform-admin | новый |
| Members | `GET /accounts/{id}/members` | новый |
| | `PATCH /accounts/{id}/members/{userId}` `{role?, status?}` | новый |
| | `DELETE /accounts/{id}/members/{userId}` | новый |
| Invitations | `GET /invitations/{token}` (публичный) | новый |
| | `POST /invitations/{token}/accept` | новый |
| | `GET /accounts/{id}/invitations` | новый |
| | `POST /accounts/{id}/invitations` `{email, role}` | новый |
| | `POST /accounts/{id}/invitations/{id}/resend` | новый |
| | `DELETE /accounts/{id}/invitations/{id}` | новый |
| Plugin policies | `GET /accounts/{id}/plugins/policies` | новый |
| | `PATCH /accounts/{id}/plugins/policies/{pluginId}` `{enabled?, settings?}` | новый |
| Existing | `GET/POST/PUT/DELETE /products|devices|…` — все требуют `X-Account-Id` (или берут из JWT active) и проверяют account-scope; `?account_id=` query поддерживается для platform-admin | все брейкинг |
| MCP | Каждый list/get-тул получает параметр `account_id` (optional, default — active) | minor |
| WebSocket | `GET /api/v1/ws?token=...&account_id=<active>` — фильтр по аккаунту | расширение |

Полный перечень эндпоинтов и формат запросов/ответов — в [02-api.md](02-api.md) (дополнение).

---

## 10. Конфигурация

### Новые переменные окружения

| Переменная | По умолчанию | Назначение |
|-----------|-------------|------------|
| `DATABASE_DRIVER` | авто (см. F-1301) | `sqlite` или `postgres` |
| `DATABASE_DSN` | `data/platform.db` | DSN: путь к файлу (sqlite) или URL (postgres) |
| `BOOTSTRAP_ACCOUNT_SLUG` | `default` | Slug аккаунта, который создаётся визардом (F-1404) или bootstrap-логикой (F-102) |
| `BOOTSTRAP_ACCOUNT_NAME` | `Default` | Display name |
| `INVITE_TTL_HOURS` | `72` | TTL приглашения |
| `WIZARD_AUTO_DISABLE` | `true` | Отключить ли `/setup` маршрут после первого Apply (F-1401) |
| `PROMETHEUS_EXPORTER_DEFAULT_PORT` | `9100` | Default port для плагина |
| `WIZARD_WRITE_ENV` | `false` | Если `true` — Apply пишет конфиг ещё и в `.env` (F-1413) |
| `DB_STATEMENT_TIMEOUT_MS` | `30000` | Postgres `statement_timeout` (sqlite игнорирует) |
| `DB_MAX_OPEN_CONNS` | `8` | Для postgres — connection pool; для sqlite — игнорируется (всегда 1 writer) |

### Удалённые / устаревшие

| Переменная | Статус | Альтернатива |
|------------|--------|--------------|
| `RULES_BUS_INCLUDE_LEGACY_PREFIX` | **Удалена в v1.1** | — (hard break, см. F-704) |
| `CONVERT_TO_MULTI_ACCOUNT` | **Удалена в v1.1** | — (утилиты нет, БД пересоздаётся вручную) |
| `REGISTER_DEFAULT_ROLE` | Сохранён, но **только** для bootstrap-аккаунта | Для новых аккаунтов роль определяется приглашением (F-303) |
| `ADMIN_LOGIN`, `ADMIN_PASSWORD`, `ADMIN_EMAIL` | Сохранены, но **опциональны** | Если заданы — Step 2 визарда pre-filled; если нет — обязательное поле UI |

---

## 11. Bad cases и риски

### 11.1 Identity

| Кейс | Поведение |
|------|-----------|
| Пользователь с JWT, выданным для активного аккаунта A, шлёт `X-Account-Id: B` | `403 account_mismatch` |
| Пользователь — member в A и B; в JWT `active=A`; `GET /devices/42` где device.account_id=B | `404 not_found` (entity не существует в active-аккаунте) |
| Пользователь удалён из аккаунта, но JWT ещё не истёк; следующий запрос | `403 membership_inactive` (revocation check на каждом запросе) |
| Платформенный admin (`is_platform_admin=true`) запрашивает чужой аккаунт | без `X-Account-Id` или с `X-Account-Id: <target>`; запрос от `actor_kind='platform_admin'`, audit |
| Два admin пытаются удалить друг друга | оба получают `200`, но без последнего admin — `409 last_admin` |
| Последний admin покидает аккаунт | `409 last_admin` — нужно сначала повысить другого или удалить аккаунт |
| Refresh-токен старого типа (если бы был) | не применимо — мы не вводим refresh, пользователь заново делает login при истечении JWT (TTL = 24h) |

### 11.2 Devices

| Кейс | Поведение |
|------|-----------|
| Device подключается с валидным token, но аккаунт устройства `suspended` | `auth_fail account_suspended` — устройство отключается (на стороне платформы); UI рисует баннер |
| Device подключается с token, но устройство удалено | `auth_fail device_not_found` |
| Token rotate: device `foo` (account A) передан в прошивку `bar` (account B) | `bar` пытается авторизоваться с username `bar` + password (token от `foo`) → `ResolveByToken` ищет по hash → `foo` device, но username `bar` не совпадает с `foo`'s `product.device` → `auth_fail` (fail-closed) |
| Cross-account ACL: external plugin account A публикует команду на device account B | `acl_deny` + `403` на invoke_rpc |
| Topic style device разный, разные аккаунты | `DetectStyle` смотрит на топик, не на account — поведение не меняется |

### 11.3 External plugins

| Кейс | Поведение |
|------|-----------|
| Plugin `extplg_acme_x` подписывается на `platform/v1/events/beta/y/telemetry` | `acl_deny` |
| Plugin `extplg_acme_x` подписывается на `platform/v1/events/acme/+/+` | `200` (OK) |
| Plugin `extplg_acme_x` публикует на `platform/v1/commands/beta/y/rpc` | `acl_deny` |
| API-токен плагина передан в HTTP RPC с указанием device из другого аккаунта | `403 device_not_in_account` |
| Регистрация плагина с уже занятым `id` в этом аккаунте | `409 plugin_exists` (но другой аккаунт может иметь plugin с тем же id) |
| MQTT-пароль скомпрометирован | `POST /plugins/external/{id}/regenerate-mqtt` — старый пароль инвалидируется; существующая MQTT-сессия отключается |

### 11.4 Promotional / API

| Кейс | Поведение |
|------|-----------|
| API-token (per-account) использован в другом аккаунте | `403 token_account_mismatch` |
| WebSocket: клиент подключился, активный аккаунт сменили (новый login) | старая WS-сессия остаётся со старым фильтром до дисконнекта; новая — с новым |
| Audit log переполнение | retention 90 дней для audit_log per-account, отдельно от telemetry retention |
| Invitation token утерян | resend; старый revoke |
| Invitation token истёк | UI/email помечает `expired`; нельзя accept |
| Email collision: пользователь с email X уже есть, новый invite на тот же email | `409 user_exists`; ссылка на «уже есть аккаунт, войдите и примите invite» — приглашение лежит в БД и показывается при логине |
| Bootstrap admin: `ADMIN_LOGIN` уже существует в `users`, но не привязан к bootstrap-аккаунту | при первом старте конвертации — привязываем существующего пользователя к `default` с ролью `admin` |

### 11.5 Prometheus exporter

| Кейс | Поведение |
|------|-----------|
| Админ выключает `prometheus-exporter` в активном аккаунте через policy | HTTP-сервер останавливается; scrape → `connection refused`; на UI — баннер |
| Port занят другим плагином/инстансом | `bind error: port already in use` лог; runner помечается `last_error`; пользователь должен сменить port в config |
| Account B пытается scrape endpoint account A | ничего — порт у каждого свой; метрики лейблованы `account=acme` всегда |
| Prometheus забыл bearer_token | `401` (если задан) — корректный fail-mode |

### 11.6 Database driver (sqlite / postgres)

| Кейс | Поведение |
|------|-----------|
| `DATABASE_DRIVER=postgres`, `DATABASE_DSN` пустой | Старт падает с `fatal: DATABASE_DSN required for driver=postgres` (до открытия порта) |
| `DATABASE_DRIVER=sqlite`, путь к файлу не существует | Файл создаётся автоматически с правами 0600 (если позволяет ОС) |
| Postgres недоступен на старте | Retry 3 раза с backoff (1s, 2s, 4s), потом fatal. Лог: `postgres unreachable after 3 retries` |
| Миграция на Postgres падает посередине (например, constraint violation) | `migrate` оставляет БД в broken state; CLI `platform migrate-status` показывает, на какой версии застряли. **Ручной fix** — катить `down` до последней good версии; в продакшене — снапшот БД до миграции обязателен |
| Параллельные миграции (два инстанса стартуют одновременно) | `golang-migrate` использует advisory lock в postgres; на sqlite — не применимо (v1 single-instance) |
| `foreign_keys=ON` не сработала (sqlite) | Видно в логе `PRAGMA foreign_keys` warning; тест NF-13 падает в CI |
| Запрос с `$1` отправлен в sqlite | `prepare error: near "$1"` — поймано на смоук-тесте (F-1306) |
| Запрос с `?` отправлен в postgres | `prepare error: syntax error at or near "?"` — то же |
| Переполнение `INTEGER` (devices > 2.1B) | Не в скоупе v1 — на таких объёмах уже давно надо переходить на v2 + read replica + partitioning |
| `DATABASE_DSN` содержит пароль | Логи маскируют DSN (driver=postgres, host=…, user=…, password=***); отображается только в `/api/v1/admin/diagnostics` (platform-admin only) |
| `WAL`-файл `data/platform.db-wal` не удаляется при kill -9 | Нормально: SQLite checkpoint при следующем открытии. Размер WAL — мониторим через `prometheus-exporter` (`cogitor_sqlite_wal_bytes`) |

### 11.7 Setup wizard

| Кейс | Поведение |
|------|-----------|
| `accounts.count > 0`, юзер открывает `/setup` | `404 setup_disabled` (маршрут выключен) |
| Wizard открыт в двух вкладках одновременно | Каждая вкладка имеет свой `localStorage`; если Apply в одной прошёл — вторая получит 404 на следующем шаге. UI рисует «Setup already completed, please log in» |
| Postgres DSN ввели неверно (host не резолвится) | Кнопка «Test connection» показывает ошибку; следующий шаг disabled. После таймаута 30s — UI показывает «Connection timed out» |
| `JWT_SECRET` в env не задан, визард только что дошёл до Step 5, юзер закрыл вкладку | При следующем заходе `/setup` — визард начинает с Step 1, секрет не сохраняется (state в localStorage) |
| Admin в визарде выбрал slug `default`, но `BOOTSTRAP_ACCOUNT_SLUG=acme` в env | Env-приоритет: визард покажет warning «env has different value, please update .env or accept default» |
| Email в Step 2 уже занят в `users` | `409 user_exists`; UI предлагает «Use existing account» (если логин совпадает) или «Use different email» |
| Apply упал на Step 8 (например, миграция БД упала на constraint) | `setup_state.error=true`, лог в audit. Следующий Apply начинает сначала (idempotency через transaction: смотрим на partial state, откатываем) |
| Виртуальный Apply прошёл, но платформа упала до того, как записала `accounts` | На следующем старте платформа проверяет `accounts.count==0` → `/setup` снова доступен. Никаких partial state'ов в БД |
| Wizard работает по HTTPS, а `PUBLIC_BASE_URL=http://...` | UI подсвечивает несоответствие; invite-ссылки генерируются по PUBLIC_BASE_URL, не по текущему origin (важно для реверс-прокси) |
| SMTP Step пропустили (`Skip`), но `EMAIL_MODE=smtp` в env | При попытке отправить invite (F-301) — `500 smtp_not_configured`; UI инструктирует «Run setup again to configure SMTP» (но `/setup` уже выключен — manual fix через SQL: `UPDATE accounts.settings_json SET...`) |

### 11.8 Будущая HA (что мы закладываем уже сейчас)

| Кейс | Поведение в v1 | Как поведёт в v2 (дизайн) |
|------|----------------|-----------------------------|
| Два инстанса стартуют с одной SQLite БД | v1 это запрещает (см. NF-19: инвариант «single instance»); если кто-то нарушит — `database is locked` | Не применимо (HA на Postgres) |
| Два инстанса стартуют с одной Postgres БД | v1 формально не запрещает (один writer в postgres OK), но в WebSocket будет дублирование | Будет требоваться `instance_id` per pod, leader election, sticky session |
| Один инстанс упал, второй подхватил | Не работает (WebSocket-клиенты переподключатся и потеряют события за время простоя) | WebSocket-клиенты реконнектятся; события за downtime теряются (v2.x: добавление persistent buffer) |
| External plugin подключился, инстанс перезапустился | MQTT-сессия теряется; плагин переподключается; за время downtime — пропущенные события | Через external broker + persistent sessions (`cleansession=false`); broker хранит сообщения пока плагин не вернётся |
| Cache (membership, productForToken) изменился на инстансе A, запрос пришёл на B | v1: stale data до следующего reload (per-request check на критичных путях ловит) | Через Redis pub/sub invalidate; cache читает с TTL 5s |
| Postgres failover во время миграции | v1 не применимо (только sqlite) | `golang-migrate` retry + advisory lock; failed migration → manual recovery |

### 11.9 Известные ограничения

- **Single-tenant БД SQLite** — все аккаунты в одной БД. Бэкап — общий, restore — общий. Multi-DB isolation — не в скоупе.
- **MQTT broker** — один процесс mochi-mqtt обслуживает все аккаунты. Разделение по namespace — на уровне ACL. Горизонтальное масштабирование — не в скоупе.
- **Prometheus exporter** — in-process HTTP-сервер; не reverse-proxy-friendly. Если нужен TLS — рекомендуем проксировать через `nginx`/`caddy`.
- **Invitations** — только email; SSO (SAML/OIDC) per-account — не в скоупе.
- **Account transfer** — нет; удаление + recreate.
- **Cross-account dashboards / shared devices** — не в скоупе.

---

## 12. Вне скобки (v1 этого релиза)

- Billing / quota
- Account hierarchy (parent/child, sub-accounts)
- Audit log export (SIEM)
- Per-account database (multi-DB)
- SAML / OIDC SSO per account
- Account transfer (merge / split)
- Cross-account dashboards
- WebSocket multi-account subscription (один WS получает события нескольких аккаунтов)

---

## 13. Критерии приёмки

### 13.1 Tenancy

- [ ] При первом запуске на чистой БД создаётся аккаунт `default` с одним admin (env) и slug `default`
- [ ] Пользователь видит в шапке dropdown со списком своих аккаунтов, может переключиться
- [ ] `GET /devices/42` для device в чужом аккаунте возвращает `404`
- [ ] `GET /devices` фильтрует список по активному аккаунту
- [ ] Удаление последнего admin в аккаунте возвращает `409 last_admin`
- [ ] API-токен, выданный для account A, отвергается в account B (`403 token_account_mismatch`)

### 13.2 Приглашения

- [ ] `POST /accounts/{id}/invitations` создаёт запись и шлёт письмо с ссылкой `{PUBLIC_BASE_URL}/invite/{token}`
- [ ] Ссылка в UI предзаполняет email при регистрации (`/auth/register?invite=...`)
- [ ] После `accept` пользователь сразу видит аккаунт в switcher
- [ ] Resend revoke'ит старый токен и шлёт новый
- [ ] TTL по умолчанию 72h; expired токен → `404 invite_not_found`

### 13.3 Внешние плагины и ACL

- [ ] `mqtt_user` имеет формат `extplg_{account_slug}_{plugin_id}`
- [ ] External plugin account A подписывается на `platform/v1/events/{slug_A}/#` — `200`
- [ ] External plugin account A подписывается на `platform/v1/events/{slug_B}/#` — `acl_deny`
- [ ] External plugin account A публикует на `platform/v1/commands/{slug_A}/{product}/{device}/rpc` — `200`
- [ ] External plugin account A публикует на `platform/v1/commands/{slug_B}/...` — `acl_deny`
- [ ] `platform.invoke_rpc` через HTTP API от плагина account A на device account B — `403`
- [ ] Правила bus публикуются в новом формате с `{account_slug}`; старые правила в `flows.json` обновлены

### 13.4 Внутренние плагины / policy

- [ ] По умолчанию `account_plugin_policies` пуст, все каталожные плагины работают
- [ ] `PATCH /accounts/{id}/plugins/policies/led-garland {enabled:false}` останавливает runner'ы; в `GET /devices/{id}/plugins` плагин показывается как `disabled`; события `tick` к нему не приходят
- [ ] `PATCH ... {enabled:true}` перезапускает раннер на ближайшем online-устройстве
- [ ] `metrics-dashboard` отображает виртуальный таб «Metrics+» на странице устройства с дефолтным шаблоном

### 13.5 Prometheus

- [ ] При активном `prometheus-exporter` на `http://platform:9100/metrics` отдаётся текст в формате Prometheus exposition с лейблом `account`
- [ ] Метрики `cogitor_devices_total{account,product,status}` обновляются не реже чем раз в `update_interval_ms`
- [ ] `cogitor_device_telemetry_samples_total{account,product,device}` инкрементируется при каждом `StoreTelemetry`
- [ ] `bearer_token` в config — `Authorization: Bearer <token>` проходит, без заголовка — `401`
- [ ] Admin выключает exporter в account → scrape возвращает `connection refused`

### 13.6 Database (sqlite / postgres)

- [ ] `DATABASE_DRIVER=sqlite` с `DATABASE_DSN=data/platform.db` поднимает платформу, прогоняет миграции, доступен `/setup`
- [ ] `DATABASE_DRIVER=postgres` с `DATABASE_DSN=postgres://...` поднимает платформу, прогоняет те же миграции, `/setup` доступен
- [ ] `make smoke-sqlite` и `make smoke-postgres` зелёные; оба создают одинаковую структуру таблиц
- [ ] `make migrate-up && make migrate-down && make migrate-up` на пустой БД — отрабатывает без ошибок
- [ ] `golang-migrate` advisory lock виден в `pg_locks` во время миграции на postgres
- [ ] `WAL`-файл SQLite создаётся при первой записи; размер мониторится через `cogitor_sqlite_wal_bytes`
- [ ] Один и тот же смоук-тест (devices publish → telemetry stored → ACL scope) проходит на обоих драйверах

### 13.7 Setup Wizard

- [ ] На чистой установке `GET /setup` отдаёт SPA-страницу со Step 1
- [ ] После `Apply` (Step 8) — `accounts.count > 0` → `GET /setup` возвращает `404`
- [ ] `Apply` с `DATABASE_DSN=postgres://...` создаёт схему, admin user, default account в одной транзакции
- [ ] Если в env задан `DATABASE_DRIVER` — Step 1 визарда read-only, показывает «Using existing config»
- [ ] Если Apply упал на Step 8 (constraint violation) — следующий Apply корректно откатывает partial state
- [ ] `platform setup --from-yaml=...` CLI fallback работает идентично веб-визарду
- [ ] Все попытки Apply пишутся в `audit_log` с `actor_kind='setup'`

### 13.8 HA-readiness (для v2)

- [ ] Все SQL-запросы параметризованы — `staticcheck` (или кастомный линтер) находит 0 нарушений в `internal/services/`
- [ ] Все `goroutine` стартуют через `context.Context` (no leaks при shutdown) — тест в `internal/services/lifecycle_test.go`
- [ ] `services.ScopeAccount` — единственная точка фильтрации per-account (grep по `WHERE account_id` показывает только этот helper)
- [ ] `prometheus-exporter` `cogitor_sqlite_wal_bytes`, `cogitor_db_open_connections`, `cogitor_db_in_use` экспозятся

### 13.9 UI: Automation депрекейс + modernization + inline help

- [ ] Маршрут `/automation` удалён из `App.tsx` и `AppShell.tsx` NAV-массива; `AutomationPage.tsx` удалён
- [ ] Страница/раздел `/docs/automation` (или в Settings) содержит руководство по интеграции с Node-RED: подключение к MQTT broker, топики `platform/v1/events/{account_slug}/...`, примеры flows.json, troubleshooting
- [ ] Settings → «Automation & Integrations» (admin-only): краткое описание rules bus, ссылка на документацию, кнопка «Copy MQTT topics»
- [ ] Tooltips / help-иконки (ⓘ) присутствуют на admin-only действиях: создание external plugin, управление internal plugins (Integrations), приглашения, account settings
- [ ] UI прошёл визуальное обновление (NF-22): пересмотрена `theme.scss` — карточки с тенями/скруглениями, улучшенные контрасты, консистентные размеры
- [ ] Все новые страницы (Setup Wizard, Members, Invitations, Integrations, Account Settings) корректно отображаются на экранах ≥360px (NF-23)
- [ ] `06-automation.md` обновлён: убрано описание iframe, добавлено пошаговое руководство по Node-RED с multi-account

---

## 14. Будущие улучшения (v2+)

Этот раздел — не «идеи на салфетке», а **зафиксированный backlog** с критериями приёмки. Каждый пункт имеет версию-целевую, scope, exit-критерий и явное «не делаем в v1, потому что…».

### 14.1 v2.0 — HA / Кластеризация

**Scope:** переход от single-instance к multi-instance деплою. Postgres становится обязательным драйвером.

| ID | Фича | Exit-критерий |
|----|------|----------------|
| F2-01 | **External MQTT broker**: `MQTT_EXTERNAL_URL` (через `paho.mqtt.golang` или EMQX client) — платформа подключается к внешнему broker'у вместо поднятия `mochi-mqtt`. ACL реализуется на broker'е через `$CONTEXT/clientid`-style auth | `docker-compose` с одним EMQX + двумя инстансами платформы; device, подключённый к инстансу A, получает downlink от REST API на инстансе B |
| F2-02 | **Redis pub/sub для WebSocket**: hub fan-out через каналы `account:{id}:events`. Каждый инстанс подписывается только на каналы своих активных WS-соединений | Событие от инстанса A доходит до пользователя, подключённого к инстансу B, в пределах 100 мс |
| F2-03 | **Leader election** для plugin runners: только лидер-инстанс запускает `Plugin.OnEvent`. Followers подписываются на Redis stream `plugin:events:{account_id}:{plugin_id}` | External plugin получает ровно одну копию события при наличии ≥2 инстансов платформы |
| F2-04 | **Read replica routing**: telemetry queries (тяжёлые `SELECT * FROM telemetry_samples WHERE...`) идут в `pg_read_replica`, запись — в master. `services.ScopeAccount` получает опциональный параметр `ReadDB()` | Dashboard с 1M точек telemetry не нагружает master; p95 latency dashboard /graphs < 500 мс |
| F2-05 | **Object storage для OTA firmware**: `OTA_STORAGE=s3` (или `minio`), прошивки хранятся в S3-совместимом storage. `OTA_STORAGE=local` — legacy | Загрузка 100MB прошивки не блокирует основной процесс; quota per-account |
| F2-06 | **Instance ID + Prometheus labels**: каждый инстанс экспозит `cogitor_instance_info{instance_id="...", version="...", started_at="..."}`. Prometheus service discovery через Consul или K8s | В Grafana видно, какой инстанс с какой версией; rolling deploy без потери метрик |
| F2-07 | **DB connection pool per-account sharding** (опционально): для крупных деплоев — отдельный postgres schema per account. Через `db.Conn(accountID)` | БД 1 аккаунта с 10k устройств не мешает другому аккаунту с 10k |

**Почему не в v1:** каждый из этих пунктов сам по себе — большой объём работы (F2-01 — отдельный PR на 500-1000 строк). Совокупно — это 2-3 месяца. v1 закладывает интерфейсы, чтобы v2 не ломал API.

### 14.2 v2.1 — Catalog plugin `rabbitmq-bridge`

**Scope:** новый каталожный плагин, публикующий события платформы в RabbitMQ exchange'и.

| ID | Фича | Exit-критерий |
|----|------|----------------|
| F2-11 | **Plugin `rabbitmq-bridge`** в `internal/plugins/rabbitmq_bridge.go` | Каталожный, регистрируется в `init()`, подчиняется `account_plugin_policies` |
| F2-12 | **Binding config**: `url` (AMQP), `exchange` (default `cogitor.events`), `routing_key_template` (default `{account_slug}.{product}.{device}.{kind}`), `tls` (off by default) | Конфиг через `device_plugin_bindings.config_json`; форма валидируется через `Descriptor.Form` |
| F2-13 | **Account scope**: каждый аккаунт может создать binding; плагин держит одно соединение с broker'ом per account (свой vhost, если задан) | Broker admin видит разные connections от разных аккаунтов |
| F2-14 | **Backpressure**: при недоступности broker'а — буферизация 10k событий в памяти (per-account), при переполнении — отбрасывание с метрикой `cogitor_rabbitmq_dropped_total` | При выключении broker'а на 5 мин и его возврате — все события доставлены, метрика показывает 0 dropped |
| F2-15 | **Sample consumer** в `examples/plugins/rabbitmq/python_consumer.py` — пример consumer, который подписывается и печатает события | Пример запускается через `make example-rabbitmq` и получает live-события |

**Почему не в v1:** внешняя зависимость (RabbitMQ), нужен отдельный deployment story. Кандидат после v2.0, когда HA уже есть и можно думать о federation'е между несколькими инсталляциями.

### 14.3 v2.2 — Account-level features

| ID | Фича | Exit-критерий |
|----|------|----------------|
| F2-21 | **Custom RBAC**: per-account кастомные роли поверх встроенных admin/operator/viewer | `POST /accounts/{id}/roles {name, permissions: [...]}`; permission check через `auth.Can(action, claims)` |
| F2-22 | **Per-account API rate limiting**: `RATE_LIMIT_PER_MINUTE` per account; реализация — token bucket в Redis | Превышение → `429` с `Retry-After` |
| F2-23 | **Per-account retention override**: `accounts.settings_json.retention_days` уже есть, но пока single-value; v2.2 — отдельные `telemetry_retention`, `events_retention`, `audit_retention` | Admin меняет retention для одного типа, остальные не трогаются |
| F2-24 | **Account import/export (JSON)**: `GET /accounts/{id}/export` (platform-admin) — полный дамп products/devices/credentials (без secrets) в JSON; `POST /accounts/import` восстанавливает | Round-trip: export → import даёт идентичную конфигурацию |
| F2-25 | **Webhook signing per-account**: каждый account может загрузить свой `webhook_signing_secret` (HMAC-SHA256); заголовок `X-Cogitor-Signature` в каждом webhook | Sample webhook receiver верифицирует подпись |
| F2-26 | **Per-account Prometheus remote_write**: вместо локального `/metrics` — отправка в account-specific Prometheus/VictoriaMetrics | `prometheus-exporter` отправляет через `remote_write` API, локальный endpoint отключается |

### 14.4 v2.3+ — Более далёкое будущее

| ID | Фича | Заметка |
|----|------|---------|
| F2-31 | **SSO providers per account** (OIDC, SAML) | Заменяет local auth для конкретного account'а; default behavior остаётся |
| F2-32 | **Audit log streaming в SIEM** (Kafka, Loki, Splunk) | Через отдельный sink в `audit_log` writer'е |
| F2-33 | **Account transfer / merge** | «Передать аккаунт другому владельцу» — сложный flow с подтверждением обоих сторон |
| F2-34 | **Cross-account dashboards** (агрегация данных нескольких аккаунтов) | Только для platform-admin; не нарушает изоляцию |
| F2-35 | **Multi-region / DR** (active-passive через Postgres streaming replication) | Требует F2-05 (object storage) |
| F2-36 | **Plugin marketplace** (подписанные каталожные плагины) | Отдельный реестр; сейчас каталог = код |

### 14.5 Принцип «не делаем» (явный список)

Эти идеи **отвергнуты** после обсуждения, фиксируем чтобы не возвращаться:

- ❌ **Multi-DB isolation (отдельная БД на аккаунт)** — operational nightmare, бэкапы, миграции ×N. Не делаем.
- ❌ **Per-account sub-domain (`acme.platform.local`)** — DNS-конфигурация лежит на пользователе; платформа не в праве это навязывать
- ❌ **Custom CSS / white-label** для account'а — out of product scope, лежит на уровне reverse proxy
- ❌ **Account-level плагин-каталог** (свой набор плагинов) — пока каталожные плагины общие; custom plugins = external plugins (уже есть)

---

## 15. Связанная документация

| Док | Что |
|-----|------|
| [01-prd.md](01-prd.md) | Общий продуктовый PRD (обновлён: out-of-scope multi-tenancy → реализован в этом релизе) |
| [02-api.md](02-api.md) | REST API reference (будет дополнен новыми эндпоинтами) |
| [03-auth.md](03-auth.md) | Аутентификация (JWT claims, active account) |
| [04-mqtt.md](04-mqtt.md) | MQTT-протокол (правила bus — новый формат) |
| [06-automation.md](06-automation.md) | Node-RED / rules bus (**обновить**: убрать iframe-описание, добавить пошаговое руководство по интеграции с multi-account, новые топики с `{account_slug}`, ACL для external plugins) |
| [07-internal-plugins.md](07-internal-plugins.md) | Catalog + новые плагины (prometheus-exporter, metrics-dashboard) |
| [08-external-plugins.md](08-external-plugins.md) | Account-scoped external plugins (mqtt_user формат, ACL) |
| [11-operations.md](11-operations.md) | Эксплуатация (конверсия, конфигурация) |
| [14-diagrams.md](14-diagrams.md) | Все диаграммы релиза (5 новых) |
