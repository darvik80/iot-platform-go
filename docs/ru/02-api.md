# REST API (`/api/v1`)

Авторизация: `Authorization: Bearer <jwt>` после `POST /auth/login`, или персональный API-токен.

WebSocket: `GET /api/v1/ws?token=<jwt>` — JSON-сообщения `{ "event", "data" }`.

## Аутентификация

### `GET /auth/config`

Публичные флаги: `register_enabled`, `require_email_verify`, `google_enabled`.

### `POST /auth/register`

```json
{ "login": "jdoe", "email": "jdoe@example.com", "password": "secret123", "display_name": "Jane" }
```

### `POST /auth/login`

```json
{ "email": "admin@cogitor.local", "password": "admin" }
```

`email` принимает логин **или** email. См. [03-auth.md](03-auth.md) для Google OAuth и подтверждения email.

### `GET /me`

Текущий пользователь.

## Обзор и здоровье

| Метод | Путь | Примечания |
|-------|------|-----------|
| GET | `/healthz` | liveness (без авторизации) |
| GET | `/readyz` | DB + MQTT ready (без авторизации); включает `mqtt_clients` |
| GET | `/overview` | счётчики онлайн/офлайн, MQTT флаг + кол-во клиентов |
| GET | `/mqtt` | живые MQTT-сессии + последние события брокера |
| GET | `/settings` | retention, broker host, Node-RED URL, часовая зона |

## Продукты

| Метод | Путь |
|-------|------|
| GET | `/products` |
| POST | `/products` |
| GET | `/products/{id}` |
| PUT | `/products/{id}` |
| DELETE | `/products/{id}` |

```json
{
  "product_id": "esp32-cam",
  "name": "ESP32-CAM",
  "description": ""
}
```

## Устройства

| Метод | Путь | Примечания |
|-------|------|-----------|
| GET | `/devices?product_id=&status=&q=` | список |
| POST | `/devices` | возвращает `credentials` **один раз**. Стиль топиков определяется из MQTT. |
| GET | `/devices/{id}` | детали + атрибуты + последняя телеметрия |
| PUT | `/devices/{id}` | метаданные |
| DELETE | `/devices/{id}` | |
| POST | `/devices/{id}/token/rotate` | новые credentials один раз |
| POST | `/devices/{id}/enable` | |
| POST | `/devices/{id}/disable` | |
| GET | `/plugins` | каталог встроенных плагинов |
| GET | `/devices/{id}/plugins` | binding-и для этого устройства |
| POST | `/devices/{id}/plugins` | `{ "plugin_id", "config"?, "enabled"? }` |
| PUT | `/devices/{id}/plugins/{bindingId}` | обновить config / enabled |
| DELETE | `/devices/{id}/plugins/{bindingId}` | |
| POST | `/devices/{id}/plugins/{bindingId}/apply` | выполнить один фрейм плагина |

### Ответ при создании

```json
{
  "device": { "id": 1, "product_id": "esp32-cam", "device_id": "cam-001", "status": "offline", "...": "..." },
  "credentials": {
    "product_id": "esp32-cam",
    "device_id": "cam-001",
    "access_token": "...",
    "username": "esp32-cam.cam-001",
    "client_id": "esp32-cam.cam-001",
    "broker_host": "localhost",
    "broker_port": 1883,
    "topic_style": "auto",
    "snippet": "CogitorCredentials::createAccessToken(...);",
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

## Телеметрия и атрибуты

| Метод | Путь |
|-------|------|
| GET | `/devices/{id}/telemetry?keys=a,b&from=&to=&limit=` |
| GET | `/devices/{id}/telemetry/latest` |
| GET | `/devices/{id}/attributes` |
| PATCH | `/devices/{id}/attributes/desired` |

Тело desired — плоский JSON-объект; платформа сохраняет и публикует `attributes/update`.

## RPC

`POST /devices/{id}/rpc?wait=1`

```json
{ "method": "reboot", "params": { "delayMs": 1000 }, "timeout_ms": 30000 }
```

`GET /devices/{id}/rpc` — история.

## События / логи / OTA

| Метод | Путь |
|-------|------|
| GET | `/events?device_id=&severity=&from=&to=&limit=` |
| GET | `/logs?device_id=&level=&from=&to=&limit=` |
| GET | `/ota/firmwares?product_id=` |
| POST | `/ota/firmwares` | зарегистрировать **внешний** URL `{ product_id, version, url, sha256?, size?, module? }` |
| POST | `/ota/firmwares/upload` | multipart: `file` + `product_id` + `version` + опционально `module` / `sha256` / `sign` |
| DELETE | `/ota/firmwares/{id}` | удалить образ, хостинг и OTA-задачи для этой прошивки |
| POST | `/ota/firmwares/{id}/notify` | `{ "device_id"?: pk, "force"?: bool }` — без `device_id` уведомляет весь продукт |
| POST | `/devices/{id}/ota` | `{ "firmware_id": 1, "force"?: false }` |
| POST | `/devices/{id}/ota/cancel` | |
| GET | `/ota/jobs?device_id=` |

Публичный (без JWT — ESP HTTP GET): `GET /ota/fw/{token}.bin` — хостинг прошивки. URL: `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`.

## Внешние плагины (только admin)

| Метод | Путь | Примечания |
|-------|------|-----------|
| GET | `/plugins/external` | список внешних плагинов |
| POST | `/plugins/external` | регистрация; возвращает `token`, `mqtt_user`, `mqtt_pass` |
| DELETE | `/plugins/external/{id}` | удаление |
| POST | `/plugins/external/{id}/regenerate-mqtt` | перевыпустить MQTT-креды |

JSON-RPC endpoint для внешних плагинов: `POST /api/plugins/external/rpc` (авторизация по токену плагина).

Методы: `platform.heartbeat`, `platform.poll_events`, `platform.ack_events`, `platform.invoke_rpc`.

## Роли

| Роль | Чтение | Запись (устройства, RPC, OTA, …) |
|------|--------|-------------------------------|
| `admin` | да | да |
| `operator` | да | да |
| `viewer` | да | нет |

Начальный пользователь — `admin`.
