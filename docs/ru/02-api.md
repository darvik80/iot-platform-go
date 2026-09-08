# REST API

Все эндпоинты на `/api/v1`. Авторизация: `Authorization: Bearer <JWT>` или `Bearer <API-токен>`.

WebSocket для real-time обновлений: `GET /api/v1/ws?token=<jwt>` — JSON-сообщения `{ "event", "data" }`.

---

## Аутентификация

### `GET /auth/config`

Публичный. Возвращает флаги: `register_enabled`, `require_email_verify`, `google_enabled`.

### `POST /auth/register`

```json
{ "login": "jdoe", "email": "jdoe@example.com", "password": "secret123", "display_name": "Jane" }
```

Логин: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`. Роль по умолчанию — `REGISTER_DEFAULT_ROLE` (обычно `viewer`).

### `POST /auth/login`

```json
{ "email": "admin@cogitor.local", "password": "admin" }
```

Поле `email` принимает **логин или email**. Возвращает `{ "token": "<jwt>" }`.

Подробнее: [03-auth.md](03-auth.md).

### `GET /me`

Текущий пользователь (профиль, роль).

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
| POST | `/devices` | Создание. Возвращает `credentials` **один раз** |
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

`POST /devices/{id}/rpc?wait=1`

```json
{ "method": "reboot", "params": { "delayMs": 1000 }, "timeout_ms": 30000 }
```

`wait=1` — ждать ответ устройства. Без `wait` — fire-and-forget.

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
| POST | `/ota/firmwares/upload` | Multipart: `file` + `product_id` + `version` + опционально `module` / `sha256` |
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

## Внешние плагины (admin)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/plugins/external` | Список внешних плагинов |
| POST | `/plugins/external` | Регистрация; возвращает `token`, `mqtt_user`, `mqtt_pass` |
| DELETE | `/plugins/external/{id}` | Удаление |
| POST | `/plugins/external/{id}/regenerate-mqtt` | Перевыпустить MQTT-креды |

JSON-RPC endpoint для внешних плагинов: `POST /api/plugins/external/rpc` (авторизация по токену плагина).

Методы: `platform.heartbeat`, `platform.poll_events`, `platform.ack_events`, `platform.invoke_rpc`.

Подробнее: [08-external-plugins.md](08-external-plugins.md).

---

## Роли

| Роль | Чтение | Запись (устройства, RPC, OTA, …) |
|------|--------|-------------------------------|
| `admin` | да | да + управление пользователями |
| `operator` | да | да |
| `viewer` | да | нет |

Начальный пользователь — `admin` (из env `ADMIN_LOGIN` / `ADMIN_PASSWORD`).
