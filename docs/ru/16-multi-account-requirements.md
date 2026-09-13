# Multi-Account — Детализированные требования (F-xxx / NF-xxx)

| Поле | Значение |
|------|----------|
| **Версия документа** | 1.1 |
| **Версия платформы** | v1.x (см. [§4.5 План релиза](16-multi-account.md#45-план-релиза--этапы)) |
| **Статус** | Черновик → Активен |
| **Обновлено** | Сентябрь 2026 |
| **Назначение** | вынесенный из основного PRD перечень функциональных (F-xxx) и нефункциональных (NF-xxx) требований; основной документ — [16-multi-account.md](16-multi-account.md) |
| **Каноническая версия** | этот документ ведётся на русском; английский перевод не предусмотрен (см. [en/16-multi-account.md](../en/16-multi-account.md) — deprecated) |

> **Это sub-документ.** Здесь только детальные таблицы требований с F-IDs и NF-IDs. Видение, проблемы, цели, план релиза, архитектура, модель данных, API, конфигурация, bad cases, критерии приёмки и будущие улучшения остаются в [16-multi-account.md](16-multi-account.md). Нумерация §5.* и §6.* сохранена, чтобы ссылки из основного документа и README работали без перерасчёта.

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
| F-309 | **Ручное создание пользователя** (без email): `POST /api/v1/accounts/{id}/members` (admin) — тело `{login, email, password, role}`. Платформа создаёт `users` + `account_memberships` напрямую, не генерируя invitation. Email-верификация пропускается (admin подтверждает email вручную). Пароль должен соответствовать policy (min 8, буква+цифра). Если `email` уже занят — `409 user_exists`. UI: кнопка «Create user manually» в Settings → Members | P0 | ☐ |
| F-310 | **Shareable link** (альтернатива email): при создании приглашения (`POST /invitations`) админ может выбрать `delivery_method ∈ {email, link}`. Если `link` — платформа **не отправляет письмо**, а возвращает ссылку `{PUBLIC_BASE_URL}/invite/{token}` в ответе. Админ копирует ссылку и расшаривает её через любой канал (мессенджер, телефон, лично). UI: radio «Send via email» / «Generate shareable link» в диалоге приглашения | P0 | ☐ |
| F-311 | **UI для альтернативных методов**: в Settings → Members — три кнопки: «Invite via email» (открывает диалог с email-полем), «Create shareable link» (генерирует ссылку с выбором роли), «Create user manually» (форма с login/email/password/role). Для operator/viewer — только «Invite via email» (остальные — admin-only) | P0 | ☐ |
| F-312 | **Самостоятельная регистрация создаёт персональный аккаунт**: `POST /auth/register` (при `REGISTER_ENABLED=true`) в одной транзакции создаёт `users` + `accounts` + `account_memberships`. Slug генерируется из login (lowercase, non-alphanumeric → `-`, trim, collapse `--`). Если slug занят — добавляется суффикс `-1`, `-2`, … Пользователь получает роль `admin` в своём аккаунте. Name = `display_name` (или login, если display_name пуст). После регистрации пользователь может сразу войти — `LoadAccountRefs` возвращает его аккаунт | P0 | ✅ |

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
| F-705 | Все downstream-сервисы (внешние автоматизации) должны обновить правила — README и шаблоны `flows.json` обновляются в этом же релизе | P0 | ☐ |
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

**Меняется только** `platform/v1/{events|commands}/...` — это **rules bus** для внешних автоматизаций и интеграций; туда добавляется сегмент `account_slug` сразу после корня (F-701). Устройства этот топик никогда не публикуют и не читают.

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
| F-1107 | **Real-time через SSE в обоих режимах** — заменить `internal/ws/hub.go` (gorilla/websocket) на SSE-стрим `GET /api/v1/events/stream`. Hub: `map[account_id]map[conn]chan Event` с per-account fanout; auto-cleanup при disconnect; `Last-Event-ID` для resume после reconnect. Единый endpoint в single и cluster mode (cluster доставляет через `iot.cluster.events` fanout exchange) | P0 | ☐ |
| F-1108 | **SSE wire format** — `Content-Type: text/event-stream`, каждое событие — `id: <seq>\nevent: <kind>\ndata: <json>\n\n`. Heartbeat: `:\n\n` (`:keepalive`) каждые 15s, чтобы не отваливались мобильные и прокси. На disconnect — flush последнего `id` через `Last-Event-ID` при следующем подключении | P0 | ☐ |
| F-1109 | **Frontend: EventSource вместо WebSocket** — `web/src/api/realtime.ts` (или где сейчас WS-клиент) переписать на `new EventSource('/api/v1/events/stream?token=' + jwt, { withCredentials: true })`. Удалить ручной heartbeat/ping, ручной reconnect — браузер делает это сам через `EventSource`. JWT передаётся через query (EventSource API не поддерживает custom headers) | P0 | ☐ |
| F-1110 | **Reverse proxy: SSE-friendly конфиг** — `proxy_buffering off; proxy_cache off; proxy_read_timeout 86400s;` для location `/api/v1/events/stream` (nginx, caddy, traefik). `Upgrade/Connection: upgrade` заголовки **не** нужны (это WebSocket) | P0 | ☐ |
| F-1111 | **Deprecation старого WebSocket endpoint** — `GET /api/v1/ws` помечен `Sunset: 2027-01-01` и `Deprecation: true` (HTTP-заголовки); работает в v1.x с предупреждением в логах. Удаляется в v2.0 (cluster) или v1.7 (single). Внутри: общий hub вызывает оба writer'а (SSE + WS) при наличии legacy-клиентов | P2 | ☐ |

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
| F-1305 | Абстракция `services.ScopeAccount(db, accountID)` — обязательная обёртка для всех per-account SQL-запросов. На уровне типов — `db.DB` интерфейс (`Query/QueryRow/Exec`); плейсхолдеры `$N` (postgres) и `?` (sqlite) разводятся через `db.Rebind(query)` (helper из `sqlx` или собственный) | P0 | ☐ |
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

### 5.15 Automation page: удаление + документация Node-RED

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
| NF-19 | **HA-readiness** | Все SQL-запросы — параметризованные. Все `goroutine` — `context.Context`-aware. Все `account_id` — `INTEGER`, не UUID. Никаких глобальных `sync.Mutex` на per-account ресурсы. Подробности — [§7.7 в 16-multi-account.md](16-multi-account.md#77-ha-readiness-контракт-с-v2) |
| NF-20 | **Тесты** | Integration-смоук (devices publish → telemetry stored → ACL scope check → audit log written) запускается на **обоих** драйверах. Покрытие ACL-кейсов (positive + negative) — 100% в `internal/mqtt/acl_test.go` |
| NF-21 | **UI/UX** | Любой `4xx` от сервера, связанный с `account_id` (mismatch / not found / suspended), даёт **читаемое** сообщение в UI («Это устройство принадлежит другому аккаунту — переключите аккаунт»), а не сырой JSON |
| NF-22 | **UI/Modern** | Визуальное обновление UI: пересмотр цветовой схемы, типографики, отступов и компонентов в `theme.scss`. Цель — современный, чистый вид при сохранении Bootstrap 5 как основы. Конкретика: карточки с тенями/скруглениями, улучшенные контрасты для dark theme, консистентные размеры кнопок и инпутов, иконки (SVG inline или icon font) вместо текстовых лейблов где уместно |
| NF-23 | **UI/Mobile** | Mobile-first подход сохраняется и усиливается: все новые страницы (Setup Wizard, Members, Invitations, Integrations, Account Settings) обязаны корректно отображаться на экранах ≥360px. Таблицы — responsive (горизонтальный скролл или card-view на мобильных). Формы — одноколоночные на мобильных, многоколоночные на desktop. Offcanvas-навигация уже работает — новые пункты добавляются в `NAV`-массив `AppShell.tsx` |
| NF-24 | **UI/Inline help** | Все admin-only действия (создание external plugin, управление internal plugins, приглашения, account settings) имеют tooltips или help-иконки (ⓘ) с кратким описанием (1-2 предложения) и ссылкой на полную документацию. Реализация — Bootstrap tooltip или `title` attribute. Текст — на русском и английском |

---

## 7. Связанная документация

| Док | Что |
|-----|------|
| [← 16-multi-account.md](16-multi-account.md) | основной документ: видение, проблемы, цели, план релиза, архитектура, модель данных, API, конфигурация, bad cases, критерии приёмки, будущее |
| [01-prd.md](01-prd.md) | общий продуктовый PRD |
| [02-api.md](02-api.md) | REST API reference (будет дополнен новыми эндпоинтами) |
| [03-auth.md](03-auth.md) | аутентификация (JWT claims, active account) |
| [04-mqtt.md](04-mqtt.md) | MQTT-протокол (правила bus — новый формат) |
| [07-internal-plugins.md](07-internal-plugins.md) | catalog + новые плагины (prometheus-exporter, metrics-dashboard) |
| [08-external-plugins.md](08-external-plugins.md) | account-scoped external plugins (mqtt_user формат, ACL) |
| [11-operations.md](11-operations.md) | эксплуатация (конверсия, конфигурация) |
| [14-diagrams.md](14-diagrams.md) | все диаграммы релиза (5 новых) |
| [17-ha-architecture.md](17-ha-architecture.md) | HA / Clustering архитектура (v2.x) |
