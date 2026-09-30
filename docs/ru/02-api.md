# REST API

Все эндпоинты на `/api/v1` (кроме нескольких корневых, перечисленных в [Служебных эндпоинтах](#служебные-эндпоинты)). Авторизация: `Authorization: Bearer <JWT>` или `Bearer <API-токен>`.

WebSocket для real-time обновлений: `GET /api/v1/ws?token=<jwt>` — JSON-сообщения `{ "event", "data" }` только по активному аккаунту токена. `Origin` браузера должен совпадать с хостом платформы, хостом из `PUBLIC_BASE_URL` или `localhost` (клиенты без `Origin` принимаются).

Все пути с `{id}` разрешаются внутри активного аккаунта: ID устройств, продуктов, прошивок, привязок плагинов и внешних плагинов других аккаунтов дают `404`.

Требуемая роль указана у эндпоинтов: **public** (без авторизации), **member** (любой участник активного аккаунта), **write** (`admin` или `operator`), **admin**. Если не указано иное, чтение доступно любому участнику, а изменения требуют write.

---

## Аутентификация

### `GET /auth/config`

Публичный. Возвращает флаги: `register_enabled`, `require_email_verify`, `google_enabled`.

### `POST /auth/register`

```json
{ "login": "jdoe", "email": "jdoe@example.com", "password": "secret123", "display_name": "Jane" }
```

Логин: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`. Опционально `account_slug` (иначе генерируется из логина). Регистрация создаёт личный аккаунт, в котором новый пользователь — `admin`. `REGISTER_DEFAULT_ROLE` (по умолчанию `admin`) записывается только в глобальное поле `role` пользователя.

### `POST /auth/login`

```json
{ "email": "admin@example.com", "password": "<password>" }
```

Поле `email` принимает **логин или email**. Возвращает `{ "token": "<jwt>" }`.

Ограничения частоты (в памяти процесса): `login`, `register`, `resend-verification` и `check-slug` — до 20 запросов подряд с одного IP, затем один раз в 3 с; после 5 неудачных входов для одного логина/email дальнейшие попытки получают `429` (с `Retry-After`), пока не восстановится лимит — одна попытка в минуту.

Подробнее: [03-auth.md](03-auth.md).

### Прочие эндпоинты авторизации

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET | `/auth/check-slug?slug=` | public | Доступность slug аккаунта: `{ "available", "slug" }` |
| GET | `/auth/verify-email?token=` | public | Подтверждение email (HTML-страница) |
| POST | `/auth/resend-verification` | public | `{ "email" }` или `{ "login" }` — повторная отправка письма |
| GET | `/auth/google`, `/auth/google/callback` | public | Google OAuth |
| POST | `/auth/switch-account` | authenticated | `{ "account_id" }` → `{ "token" }` — новый JWT с другим активным аккаунтом |

### Текущий пользователь

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET | `/me` | authenticated | Текущий пользователь (профиль, роль) |
| GET | `/me/accounts` | authenticated | Аккаунты, в которых состоит пользователь |
| GET | `/me/tokens` | authenticated | Список API-токенов |
| POST | `/me/tokens` | authenticated | Создать API-токен `{ "name" }`; токен показывается один раз |
| DELETE | `/me/tokens/{id}` | authenticated | Отозвать API-токен |
| GET | `/me/avatar` | authenticated | Получить аватар |
| POST | `/me/avatar` | authenticated | Загрузить аватар (multipart-поле `file`, до ~3 МБ) |
| DELETE | `/me/avatar` | authenticated | Удалить аватар |

Эндпоинты с ролью «authenticated» не требуют активного аккаунта.

---

## Участники аккаунта, приглашения, пользователи

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET | `/invitations/{token}` | public | Информация о приглашении |
| POST | `/invitations/{token}/accept` | authenticated | Принять приглашение |
| GET | `/invitations` | admin | Список приглашений активного аккаунта |
| POST | `/invitations` | admin | Создать: `{ "email", "role", "delivery_method" }` |
| POST | `/invitations/{id}/resend` | admin | Повторно отправить приглашение |
| DELETE | `/invitations/{id}` | admin | Отозвать приглашение |
| GET | `/members` | admin | Список участников аккаунта |
| POST | `/members` | admin | Добавить участника |
| PATCH | `/members/{userId}` | admin | Сменить роль: `{ "role" }` |
| DELETE | `/members/{userId}` | admin | Удалить участника |
| GET | `/users` | админ платформы | Список всех пользователей |
| PATCH | `/users/{id}` | admin | Изменить профиль (`display_name`, `email`, `password` ≥ 8 символов). Админ аккаунта может менять себя и пользователей, состоящих только в его аккаунте; `role` / `disabled` — только админ платформы |
| DELETE | `/users/{id}` | админ платформы | Удалить пользователя |
| GET | `/audit-log?limit=` | admin | Аудит-лог |

---

## Обзор и здоровье

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/healthz` | Liveness — процесс жив (без авторизации) |
| GET | `/readyz` | Readiness — DB + MQTT готовы (без авторизации); включает `mqtt_clients` |
| GET | `/overview` | Счётчики онлайн/офлайн, статус MQTT, число клиентов |
| GET | `/mqtt` | Живые MQTT-сессии + последние события брокера |
| GET | `/settings` | Retention, broker host, Node-RED URL, часовая зона |

---

## Продукты

Продукт — архетип устройства. Определяет RPC-методы, метрики телеметрии и атрибуты для всех устройств этого типа.

| Метод | Путь |
|-------|------|
| GET | `/products` |
| POST | `/products` |
| GET | `/products/{id}` |
| PUT | `/products/{id}` |
| DELETE | `/products/{id}` |
| POST | `/products/{id}/esp-defaults` |

`POST /products/{id}/esp-defaults` (write) заново применяет к продукту пресетные ESP-методы и метрики, сохраняя специфичные для продукта.

Тело запроса:

```json
{
  "product_id": "esp32-cam",
  "name": "ESP32-CAM",
  "description": ""
}
```

Подробнее о схеме продукта (методы, метрики, атрибуты): [09-product-schema.md](09-product-schema.md).

---

## Устройства

### CRUD

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/devices?product_id=&status=&q=` | Список с фильтрацией |
| POST | `/devices` | Создание в продукте активного аккаунта. Пара `product_id.device_id` — это MQTT-логин, поэтому она уникальна на всей платформе. Возвращает `credentials` **один раз** |
| GET | `/devices/{id}` | Детали + атрибуты + последняя телеметрия |
| PUT | `/devices/{id}` | Обновить метаданные |
| DELETE | `/devices/{id}` | Удаление |

### Управление состоянием

| Метод | Путь |
|-------|------|
| POST | `/devices/{id}/token/rotate` | Ротация токена; возвращает новые credentials **один раз**. Старая MQTT-сессия отключается |
| POST | `/devices/{id}/enable` | Включить устройство |
| POST | `/devices/{id}/disable` | Отключить; MQTT-сессия сбрасывается |

### Ответ при создании устройства

```json
{
  "device": { "id": 1, "product_id": "esp32-cam", "device_id": "cam-001", "status": "offline" },
  "credentials": {
    "product_id": "esp32-cam",
    "device_id": "cam-001",
    "access_token": "...",
    "username": "esp32-cam.cam-001",
    "client_id": "esp32-cam.cam-001",
    "broker_host": "localhost",
    "broker_port": 1883,
    "topic_style": "auto",
    "provision": {
      "wifi": { "ssid": "", "password": "" },
      "cogitor": {
        "product": "esp32-cam",
        "device": "cam-001",
        "host": "192.168.1.100",
        "port": 1883,
        "token": "...",
        "tls": false,
        "topic_short": true
      }
    }
  }
}
```

Дашборд при создании/ротации показывает модалку с этими кредами. Wi-Fi не хранится на платформе — только передаётся в прошивку.

### Плагины устройств

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/plugins` | Каталог встроенных плагинов |
| GET | `/devices/{id}/plugins` | Binding-и для устройства |
| POST | `/devices/{id}/plugins` | Создать binding: `{ "plugin_id", "config"?, "enabled"? }` |
| PUT | `/devices/{id}/plugins/{bindingId}` | Обновить config / enabled |
| DELETE | `/devices/{id}/plugins/{bindingId}` | Удалить binding |
| POST | `/devices/{id}/plugins/{bindingId}/apply` | Выполнить один фрейм плагина (устройство должно быть онлайн) |

Подробнее: [07-internal-plugins.md](07-internal-plugins.md).

---

## Телеметрия и атрибуты

| Метод | Путь |
|-------|------|
| GET | `/devices/{id}/telemetry?keys=a,b&from=&to=&limit=` | Историческая телеметрия |
| GET | `/devices/{id}/telemetry/latest` | Последние сэмплы |
| GET | `/devices/{id}/attributes` | Reported + desired атрибуты |
| PATCH | `/devices/{id}/attributes/desired` | Установить desired-атрибуты |

Тело PATCH — плоский JSON-объект. Платформа сохраняет и публикует `attributes/update` на устройство через MQTT.

---

## RPC

### Вызов метода

`POST /devices/{id}/rpc`

```json
{ "method": "reboot", "params": { "delayMs": 1000 }, "timeout_ms": 30000 }
```

По умолчанию запрос ждёт ответ устройства (до `timeout_ms`, иначе — таймаут платформы по умолчанию). `?wait=0` — fire-and-forget.

**Механизм корреляции:** платформа выдаёт последовательный целочисленный `request_id` и сохраняет связь `{request_id → HTTP-канал}` в памяти (с таймаутом). `request_id` помещается в поле `id` MQTT-сообщения (стандарт JSON-RPC). Устройство копирует `id` из запроса в ответ. Платформа сопоставляет ответ с ожидающим запросом по `request_id` и отправляет HTTP-ответ клиенту.

Подробнее о протоколе: [04-mqtt.md → RPC](04-mqtt.md#rpc).

### История

`GET /devices/{id}/rpc` — список предыдущих RPC-вызовов.

---

## События и логи

| Метод | Путь |
|-------|------|
| GET | `/events?device_id=&severity=&from=&to=&limit=` | События устройств |
| GET | `/logs?device_id=&level=&from=&to=&limit=` | Лог-сообщения устройств |

---

## OTA-обновления

### Управление прошивками

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/ota/firmwares?product_id=` | Список прошивок |
| POST | `/ota/firmwares/upload` | Multipart: `file` + `product_id` + `version` + опционально `module` / `sha256` / `sign_method` / `sign` / `size` |
| POST | `/ota/firmwares` | Зарегистрировать внешний URL: `{ product_id, version, url, sha256?, size?, module? }` |
| DELETE | `/ota/firmwares/{id}` | Удалить образ и связанные OTA-задачи |

### Запуск обновления

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/ota/firmwares/{id}/notify` | Уведомить устройство или весь продукт: `{ "device_id"?, "force"? }` |
| POST | `/devices/{id}/ota` | Запустить OTA: `{ "firmware_id", "force"? }` |
| POST | `/devices/{id}/ota/cancel` | Отменить активное обновление |
| GET | `/ota/jobs?device_id=` | Список OTA-задач |

### Публичный download (без JWT)

`GET /ota/fw/{token}.bin` — ESP скачивает прошивку по HTTP без авторизации. URL: `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`.

`PUBLIC_BASE_URL` должен быть доступен с устройства (LAN IP, не `localhost`).

### Auto-OTA

Если `auto_ota` включён на продукте (и не отключён на устройстве): при подключении или отчёте версии платформа автоматически отправляет новую прошивку, если версии различаются. Оффлайн-уведомления создают **pending** задачу — она отправится при переподключении.

---

## Внешние плагины

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET | `/plugins/external` | member | Список внешних плагинов |
| POST | `/plugins/external` | admin | Регистрация в активном аккаунте; `id` — `^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`, нельзя занять id плагина другого аккаунта. Возвращает `{ ok, id, mqtt_user, mqtt_pass, mqtt_port, mqtt_host, mqtt_ws, rules_bus }` |
| DELETE | `/plugins/external/{id}` | admin | Удаление |
| POST | `/plugins/external/{id}/regenerate-mqtt` | admin | Перевыпустить MQTT-креды |

Формат MQTT-логина: `extplg_{account_slug}_{plugin_id}`. Внешние плагины работают только через MQTT (шина правил); отдельного HTTP API для них нет.

Подробнее: [08-external-plugins.md](08-external-plugins.md).

---

## Политики плагинов (admin)

Поккаунтовый контроль каталожных плагинов (встроенные плагины: LED-гирлянда, расписание, prometheus-exporter, metrics-dashboard).

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/plugins/policies` | Список всех каталожных плагинов с эффективными enabled/settings для активного аккаунта |
| PATCH | `/plugins/policies/{pluginId}` | Обновить политику: `{ "enabled"?, "settings"? }` |

Когда плагин отключён, он перестаёт работать для всех устройств в аккаунте. Настройки — это JSON-конфигурации, специфичные для каждого плагина.

---

## Дашборд устройства

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/devices/{id}/dashboard` | Агрегированные панели дашборда для плагина metrics-dashboard |

Ответ:

```json
{
  "panels": [
    {
      "id": "telemetry_chart",
      "type": "telemetry_chart",
      "title": "WiFi RSSI",
      "data": { "keys": ["wifi_rssi"], "points": [[1234567890, -65], ...] }
    }
  ]
}
```

Типы панелей: `telemetry_chart`, `telemetry_gauge`, `rpc_table`, `online_timeline`, `events_count`.

---

## Служебные эндпоинты

Корневые (не под `/api/v1`), если в пути не указано иное.

| Метод | Путь | Роль | Описание |
|-------|------|------|----------|
| GET | `/metrics` | public | Метрики Prometheus |
| GET | `/api/v1/mqtt/ca.pem` | public | CA-сертификат MQTT TLS (404, если `MQTT_TLS_CA` не задан) |
| любой | `/mcp` | authenticated | Встроенный MCP-сервер (Streamable HTTP), см. [10-mcp.md](10-mcp.md) |

### Мастер настройки

Доступен только пока работает мастер настройки (пустая база при первом запуске); без авторизации.

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/setup/state` | Текущее состояние мастера |
| POST | `/api/v1/setup/step` | Сохранить шаг мастера |
| POST | `/api/v1/setup/apply` | Применить конфигурацию, создать первый аккаунт и администратора |
| POST | `/api/v1/setup/generate-tls` | Сгенерировать MQTT TLS `ca.pem`, `cert.pem`, `key.pem`: `{ "host"?, "dir"? }` |

---

## Роли

| Роль | Чтение | Запись (устройства, RPC, OTA, …) |
|------|--------|-------------------------------|
| `admin` | да | да + управление пользователями |
| `operator` | да | да |
| `viewer` | да | нет |

Первый пользователь и первый аккаунт создаются мастером настройки; этот пользователь — `admin`.
