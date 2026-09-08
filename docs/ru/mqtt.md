# MQTT-протокол (вид платформы)

Канонический контракт устройства: **протокол v1** (см. embed-framework `docs/iot-platform-mqtt-spec.md`).
Go-имена: [`internal/protocol`](../internal/protocol/topics.go) (`UpTelemetry` / `ShortTelemetry`, downlink-и, шина правил).

Платформа **является** MQTT-брокером ([mochi-mqtt](https://github.com/mochi-mqtt/server)). Устройства и Node-RED подключаются к `MQTT_ADDR` (по умолчанию `:1883`). Опционально MQTT-over-WebSocket на `MQTT_WS_ADDR` (по умолчанию `:8083`, URL `ws://{BROKER_HOST_PUBLIC}:8083/`, подпротокол `mqtt`). Установить `MQTT_WS_ADDR=off` для отключения. MQTT-over-TLS на `MQTT_TLS_ADDR` (по умолчанию `:8883`) — см. [mqtt-tls.md](mqtt-tls.md). Внешнего RabbitMQ нет.

## Стили топиков

Стиль **определяется**, не настраивается. Платформа определяет его из Will / subscribe / publish (идентификатор сессии, аналог ThingsBoard `/me/`):

| Префикс | Стиль |
|---------|-------|
| `iot/v1/{product}/{device}/…` | **полный** |
| `v1/me/…` (алиас `v1/dev/me/…`) | **короткий** |
| `v1/t`, `v1/r/req`, … | **короткий** |

Downlink использует последний определённый стиль. До первого MQTT-пакета downlink по умолчанию короткий.

**Полный**

```
iot/v1/{product_id}/{device_id}/{up|down}/{capability}/{operation}
```

**Короткий** (идентификатор из сессии: `client_id=username={product}.{device}`, `password=access_token`)

| Короткий | Направление | Полный |
|----------|-------------|--------|
| `v1/s` | up | `…/up/status` (только LWT) |
| `v1/t` | up | `…/up/telemetry/data` |
| `v1/e` | up | `…/up/events/post` |
| `v1/a` | up | `…/up/attributes/report` |
| `v1/a/req` | up | `…/up/attributes/request` |
| `v1/a/res` | down | `…/down/attributes/response` |
| `v1/a/upd` | down | `…/down/attributes/update` |
| `v1/r/req` | down | `…/down/rpc/request` |
| `v1/r/res` | up | `…/up/rpc/response` |
| `v1/r/creq` | up | `…/up/rpc/request` |
| `v1/r/cres` | down | `…/down/rpc/response` |
| `v1/n/req` | up | `…/up/ntp/request` |
| `v1/n/res` | down | `…/down/ntp/response` |
| `v1/o/ver` | up | `…/up/ota/version` |
| `v1/o/q` | up | `…/up/ota/query` |
| `v1/o/upd` | down | `…/down/ota/update` (`url` обычно `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`) |
| `v1/o/can` | down | `…/down/ota/cancel` |
| `v1/o/p` | up | `…/up/ota/progress` |
| `v1/l` | up | `…/up/logs/report` |

Корреляция: JSON-поле `"id"` (uint32). RPC успех: `"code": 0`.

Встроенный firmware RPC **`rpc-list`** (алиас `rpc_list`) возвращает каталог методов. Каждый параметр: `{ type, required, default? }` (`int` / `bool` / `string` / `number`). Дашборд вызывает его при онлайн-устройстве.

## Присутствие

- **Онлайн**: MQTT-сессия установлена (и первый принятый uplink).
- **Офлайн**: отключение, или LWT на статус-топике (`online: false`, `reason: "lwt"`).

## Ingest платформы

PUBLISH от не-inline клиентов обрабатывается в процессе:

- `iot/v1/{product}/{device}/up/#`
- `v1/#`
- `platform/v1/commands/#` (Node-RED → RPC / desired / OTA)

Сообщения коротких топиков атрибутируются по MQTT-username (`{product}.{device}`). Платформа републикует на шине правил через inline-клиент, и эти публикации **не** обрабатываются повторно.

## Авторизация устройств

1. Дашборд создаёт устройство → генерирует токен → SHA-256 сохраняется.
2. MQTT CONNECT: `username={product}.{device}`, `password=token`. ACL ограничен для этого устройства (`up`/`down` + `v1/#`).
3. Ротация → новый хеш в SQLite; живая сессия для этого username отключается.
4. Отключение / удаление → сессия отключена; авторизация не проходит при переподключении.

## Мониторинг

Страница **MQTT** в дашборде (`GET /api/v1/mqtt`): живые сессии (client id, username, remote IP, подписки) и in-memory лента событий (авторизация, connect/disconnect, ACL deny, ingest, publish).

Stdout (`slog`, `LOG_LEVEL`, по умолчанию Info): `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe`, `ingest` (не телеметрия), `command`, downlink `publish`. Телеметрия ingest — Debug. Docker: `docker compose logs -f platform`.

WebSocket-событие `mqtt` транслируется при connect / disconnect / auth_fail / subscribe / ACL deny / kick.

## Node-RED

Node-RED подключается как **внешний плагин** (Дашборд → Settings → External Plugins). Создать плагин, взять сгенерированные `MQTT User` (`extplg_…`) и `MQTT Password`, и использовать в MQTT-нодах Node-RED:

- подписка `platform/v1/events/#`
- публикация `platform/v1/commands/#`
