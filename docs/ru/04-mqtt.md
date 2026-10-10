# MQTT-протокол

Платформа **встроена** в процесс и является MQTT-брокером ([mochi-mqtt](https://github.com/mochi-mqtt/server)). Внешнего RabbitMQ нет.

### Подключение

| Параметр | Значение |
|----------|----------|
| TCP | `MQTT_ADDR` (по умолчанию `:1883`) |
| WebSocket | `MQTT_WS_ADDR` (по умолчанию `:8083`, URL `ws://{BROKER_HOST_PUBLIC}:8083/` или `MQTT_WS_PUBLIC_URL` за reverse proxy, подпротокол `mqtt`). `MQTT_WS_ADDR=off` — отключить |
| TLS | `MQTT_TLS_ADDR` (по умолчанию `:8883`) — см. [05-mqtt-tls.md](05-mqtt-tls.md) |
| Username | `{product_id}.{device_id}` |
| Password | access token (генерируется в дашборде) |
| Client ID | произвольный, платформа определяет устройство по username |

После CONNECT платформа определяет устройство по username и проверяет токен (SHA-256 хранится в базе данных). ACL: в полном формате устройство может публиковать только в свои `iot/v1/{product_id}/{device_id}/up/…` и подписываться только на свои `…/down/…`; короткие топики (`v1/…`) разрешены и привязываются к устройству по username сессии.

Это параметры нативного протокола (стили `short` / `full`). Диалекты **alink** и **ThingsBoard** используют другие параметры CONNECT — см. [Стили устройств (диалекты)](#стили-устройств-диалекты).

---

## Адресация

Каждое устройство идентифицируется парой **product_id / device_id**. Платформа поддерживает два формата топиков — **полный** и **короткий**. Устройство выбирает формат первым опубликованным пакетом; downlink отвечает в том же формате. До первого пакета от устройства downlink идёт в коротком формате.

### Полный формат

Топик явно содержит product_id и device_id. Устройство может использовать только топики со своими product_id / device_id. Node-RED и другие внешние клиенты не используют топики устройств — они работают через шину правил (см. ниже).

```
iot/v1/{product_id}/{device_id}/{up|down}/{capability}/{operation}
```

Пример: `iot/v1/sensors/temp-01/up/telemetry/data`

### Короткий формат

Топик не содержит идентификатор устройства — платформа определяет его по username из MQTT-сессии. Удобно для прошивки: не нужно знать свои product_id/device_id в каждом топике.

```
v1/me/{mnemonic}
```

Пример: `v1/me/t` — телеметрия от текущего устройства.

`v1/me/{mnemonic}` — каноническая форма. Для uplink платформа также принимает `v1/{mnemonic}` без `me`; downlink всегда отправляется в `v1/me/…`, поэтому устройство должно подписываться на топики `v1/me/…`.

### Список топиков

| Полный топик | Короткий | Направление | Назначение |
|---|---|---|---|
| `…/up/status` | `v1/me/s` | ↑ устройство | Статус (только LWT) |
| `…/up/telemetry/data` | `v1/me/t` | ↑ устройство | Телеметрия |
| `…/up/events/post` | `v1/me/e` | ↑ устройство | События |
| `…/up/attributes/report` | `v1/me/a` | ↑ устройство | Атрибуты (отчёт) |
| `…/up/attributes/request` | `v1/me/a/req` | ↑ устройство | Запрос атрибутов |
| `…/down/attributes/response` | `v1/me/a/res` | ↓ платформа | Ответ на запрос атрибутов |
| `…/down/attributes/update` | `v1/me/a/upd` | ↓ платформа | Обновление desired-атрибутов |
| `…/down/rpc/request` | `v1/me/r/req` | ↓ платформа | RPC-вызов на устройство |
| `…/up/rpc/response` | `v1/me/r/res` | ↑ устройство | Результат RPC |
| `…/up/rpc/request` | `v1/me/r/creq` | ↑ устройство | RPC-вызов от устройства |
| `…/down/rpc/response` | `v1/me/r/cres` | ↓ платформа | Ответ на RPC от устройства |
| `…/up/ntp/request` | `v1/me/n/req` | ↑ устройство | Запрос времени (NTP) |
| `…/down/ntp/response` | `v1/me/n/res` | ↓ платформа | Ответ времени |
| `…/up/ota/version` | `v1/me/o/ver` | ↑ устройство | Текущая версия прошивки |
| `…/up/ota/query` | `v1/me/o/q` | ↑ устройство | Запрос обновления |
| `…/down/ota/update` | `v1/me/o/upd` | ↓ платформа | Команда обновления |
| `…/down/ota/cancel` | `v1/me/o/can` | ↓ платформа | Отмена обновления |
| `…/up/ota/progress` | `v1/me/o/p` | ↑ устройство | Прогресс обновления |
| `…/up/logs/report` | `v1/me/l` | ↑ устройство | Логи устройства |

Полный топик: `iot/v1/{product_id}/{device_id}/{up|down}/…`. В колонке «Полный топик» показано окончание после `up/` или `down/`.

---

## Стили устройств (диалекты)

Платформа поддерживает четыре диалекта MQTT-протокола. Стиль (`topic_style`) задаётся устройству при создании или наследуется из поля **«Стиль топиков по умолчанию»** продукта; смотреть и менять его можно в карточке устройства.

| Диалект (`topic_style`) | Совместимость | Аутентификация | Топики |
|---|---|---|---|
| `short` (по умолчанию) | нативный | `username={product}.{device}`, `password=token` | `v1/me/…` + полный |
| `full` | нативный | `username={product}.{device}`, `password=token` | `iot/v1/…` + короткие |
| `alink` | Alibaba Cloud IoT (Alink) | HMAC-подпись deviceSecret | `/sys/…`, `/ext/ntp/…`, `/ota/device/…` |
| `thingsboard` | ThingsBoard MQTT Device API | `username=access token`, без пароля | `v1/devices/me/…` |

Правила выбора стиля:

- **Автоопределение** работает только между `short` и `full`: Will-топик, подписка или первый пакет с чужим префиксом переключает стиль устройства.
- **`alink` и `thingsboard` закреплены**: автоопределением не включаются и не выключаются, потому что стиль определяет модель аутентификации (секрет/токен). Их выбирают явно при создании устройства — или ставят дефолт продукта.
- Даунлинк (RPC, desired-атрибуты, OTA) платформа всегда отправляет в формате диалекта устройства; аплинк транслируется во внутренний формат, поэтому дашборд, автоматизация и плагины работают одинаково для всех диалектов.

В разделах ниже описан нативный протокол (`short` / `full`); специфика диалектов — в [alink](#диалект-alink-alibaba-cloud-iot) и [thingsboard](#диалект-thingsboard).

### Диалект Alink (Alibaba Cloud IoT)

Для прошивок, собранных под протокол Alink (Alicloud IoT embed-компонент). При создании/ротации платформа генерирует **deviceSecret** — HMAC-ключ, показывается один раз.

Подключение:

| Параметр | Значение |
|----------|----------|
| Username | `{deviceName}&{productKey}` (также принимается нативный `{product}.{device}`) |
| Password | HMAC-SHA256 от `clientId{username}deviceName{dn}productKey{pk}timestamp{ts}` с ключом deviceSecret, hex в верхнем регистре |
| Client ID | `{deviceName}&{productKey}\|timestamp=…,ext=…\|`; timestamp из KV-секции, по умолчанию `2524608000000` |

Топики:

| Топик | Напр. | Назначение |
|---|---|---|
| `/sys/{pk}/{dn}/thing/event/property/post` | ↑ | Телеметрия |
| `/sys/{pk}/{dn}/thing/event/{eventId}/post` | ↑ | Событие |
| `/sys/{pk}/{dn}/thing/property/desired/get` | ↑ | Запрос desired-атрибутов |
| `/sys/{pk}/{dn}/thing/property/desired/delete` | ↑ | Удаление desired-атрибутов |
| `/sys/{pk}/{dn}/thing/service/property/set_reply` | ↑ | Ответ на property/set |
| `/sys/{pk}/{dn}/thing/service/{method}/_reply` | ↑ | Ответ на RPC |
| `/sys/{pk}/{dn}/thing/ota/firmware/get` | ↑ | Запрос OTA (pull) |
| `/ota/device/inform/{pk}/{dn}` | ↑ | Версия прошивки |
| `/ota/device/progress/{pk}/{dn}` | ↑ | Прогресс OTA |
| `/ext/ntp/{pk}/{dn}/request` | ↑ | Запрос времени (NTP) |
| `/sys/{pk}/{dn}/thing/service/property/set` | ↓ | Обновление desired-атрибутов |
| `/sys/{pk}/{dn}/thing/property/desired/{get,delete}_reply` | ↓ | Ответ на запрос/удаление desired |
| `/sys/{pk}/{dn}/thing/service/{method}` | ↓ | RPC-вызов платформы |
| `/ota/device/upgrade/{pk}/{dn}` | ↓ | Команда OTA (и отмена) |
| `/ext/ntp/{pk}/{dn}/response` | ↓ | Ответ времени |

Примеры payload. Телеметрия — значения с опциональной меткой времени:
```json
{"id": "1", "version": "1.0", "params": {"temperature": {"value": 23.5, "time": 1710000000000}}}
```

RPC-вызов платформы (id — строка):
```json
{"id": "1", "version": "1.0", "method": "thing.service.reboot", "params": {"delay": 5}}
```
Устройство отвечает на `/sys/{pk}/{dn}/thing/service/reboot/_reply`; код `200` — успех:
```json
{"id": "1", "code": 200, "message": "success", "data": {}}
```

Обновление desired-атрибутов — все значения params передаются строками (прошивка читает только строки):
```json
{"id": "2", "version": "1.0", "method": "thing.service.property.set", "params": {"interval": "30"}}
```

OTA-команда (`code: "1000"` — обновление, `"-1"` — отмена):
```json
{"code": "1000", "data": {"url": "https://…/ota/fw/{token}.bin", "version": "1.3.0", "module": "main", "size": 524288}}
```

Недоступны в alink-стиле: статус LWT, отчёт reported-атрибутов, RPC от устройства к платформе, логи. Телеметрия, события, desired-атрибуты, NTP и OTA работают как в нативном стиле.

### Диалект ThingsBoard

Для прошивок, собранных под MQTT Device API ThingsBoard (JSON, v1; protobuf-формы v2 RPC и `v2/fw/…` не поддерживаются). Аутентификация: **MQTT username = access token устройства**, пароль не используется. Токен генерируется при создании/ротации и показывается один раз.

Топики:

| Топик | Напр. | Назначение |
|---|---|---|
| `v1/devices/me/telemetry` | ↑ | Телеметрия (те же форматы payload, что в нативном стиле) |
| `v1/devices/me/attributes` | ↑ | Отчёт об атрибутах |
| `v1/devices/me/attributes/request/{id}` | ↑ | Запрос атрибутов |
| `v1/devices/me/attributes/response/{id}` | ↓ | Ответ на запрос атрибутов |
| `v1/devices/me/rpc/request/{id}` | ↓ | RPC-вызов платформы |
| `v1/devices/me/rpc/response/{id}` | ↑ | Ответ на RPC |
| `v2/devices/me/rpc/response/{id}` | ↑ | Ответ на RPC (v2-форма топика) |

Устройство подписывается на `v1/devices/me/attributes` (desired-обновления приходят плоским объектом) и `v1/devices/me/rpc/request/+`.

Примеры payload. RPC-вызов платформы — id в топике, а не в теле:
```json
{"method": "reboot", "params": {"delay": 5}}
```
Устройство отвечает на `v1/devices/me/rpc/response/1` произвольным JSON:
```json
{"status": "ok"}
```

Запрос атрибутов: `sharedKeys` — desired (серверные), `clientKeys` — reported (клиентские); допустимы JSON-массив и строка `"a,b"`:
```json
{"sharedKeys": "interval", "clientKeys": "fw"}
```
Ответ придёт на `v1/devices/me/attributes/response/1`:
```json
{"client": {"fw": "1.2.3"}, "shared": {"interval": 30}}
```

Недоступны в thingsboard-стиле: статус LWT, события, логи, NTP, OTA-обмен с устройством, RPC от устройства к платформе. Телеметрия и атрибуты работают как в нативном стиле.

> Даунлинк-топики thingsboard (`v1/devices/me/…`) у всех устройств одинаковые — платформа доставляет каждое сообщение напрямую в сессию целевого устройства, как и в коротком нативном стиле.

---

## Что делает устройство

### Телеметрия

Публикация на `v1/me/t` (или `iot/v1/{p}/{d}/up/telemetry/data`):

```json
{"temperature": 23.5, "humidity": 60}
```

Платформа принимает три формата payload:

**Плоский объект** — ключи напрямую становятся сэмплами:
```json
{"temperature": 23.5, "humidity": 60, "voltage": 3.3}
```

**Объект с `values`** — ключи внутри `values`, опциональная метка времени:
```json
{"ts": 1710000000000, "values": {"temperature": 23.5, "humidity": 60}}
```

**Массив объектов** — batch-публикация нескольких наборов с разными метками времени:
```json
[
  {"ts": 1710000000000, "values": {"temperature": 23.5}},
  {"ts": 1710000001000, "values": {"temperature": 23.7}}
]
```

Каждый ключ сохраняется как отдельный сэмпл с типом: `value_num` (float64), `value_text` (string), `value_bool` (bool). Поле `ts` — миллисекунды; если отсутствует, платформа подставляет время получения.

### Статус (LWT)

При CONNECT устройство публикует на `v1/me/s` retained-сообщение:

```json
{"online": true}
```

В Last Will and Testament — `{"online": false, "reason": "lwt"}`. Платформа считает устройство офлайн при разрыве сессии или получении LWT.

### Атрибуты

- **Отчёт** (`v1/me/a`): устройство сообщает текущие атрибуты — `{"firmware": "1.2.3", "model": "A"}`
- **Запрос** (`v1/me/a/req`): устройство запрашивает desired-атрибуты у платформы
- **Обновление** (`v1/me/a/upd`, downlink): платформа отправляет желаемые атрибуты

**Отчёт атрибутов** — плоский JSON-объект, каждый ключ merges в таблицу reported-атрибутов:
```json
{"firmwareVersion": "1.2.3", "model": "A", "serial": "SN-001"}
```
Ключ `firmwareVersion` автоматически обновляет поле `fw_version` в таблице devices.

**Запрос атрибутов** — устройство запрашивает stored-атрибуты платформы:
```json
{"id": 1, "reported": ["*"], "desired": ["*"]}
```
`id` — идентификатор для корреляции ответа. `reported` / `desired` — массивы имён ключей; `["*"]` означает «все ключи». Платформа отвечает на `v1/me/a/res`:
```json
{"id": 1, "reported": {"firmwareVersion": "1.2.3"}, "desired": {"interval": 30}}
```

### RPC

Платформа вызывает метод на устройстве:

**Downlink** (`v1/me/r/req`):
```json
{"id": 1, "method": "reboot", "params": {"delay": 5}}
```

**Ответ устройства** (`v1/me/r/res`):
```json
{"id": 1, "code": 0, "data": {}}
```

`code: 0` — успех. Поле `id` (последовательное целое число) связывает запрос и ответ — это `request_id`. Устройство копирует `id` из запроса в ответ (стандартное поведение JSON-RPC).

**Механизм корреляции:**
- Платформа выдаёт следующий по порядку целочисленный `request_id` перед отправкой команды
- `request_id` помещается в поле `id` полезной нагрузки MQTT-сообщения
- Устройство обрабатывает команду и публикует ответ с тем же `id`
- Платформа сопоставляет ответ с ожидающим запросом по `request_id`

Устройство тоже может вызвать RPC на платформу через `v1/me/r/creq` (ответ придёт на `v1/me/r/cres`).

**RPC-вызов от устройства** (`v1/me/r/creq`):
```json
{"id": 1, "method": "issueUploadUrl", "params": {}}
```
Платформа отвечает на `v1/me/r/cres`:
```json
{"id": 1, "code": 0, "message": "ok", "data": {"url": "https://…/upload/{product}/{device}", "expires_in": 3600}}
```

Встроенный метод **`rpc-list`** (алиас `rpc_list`) возвращает каталог методов устройства. Каждый параметр: `{ type, required, default? }` (`int` / `bool` / `string` / `number`). Дашборд вызывает его автоматически при онлайн-устройстве.

### OTA (обновление прошивки)

1. Устройство публикует версию: `v1/me/o/ver` → `{"version": "1.2.3", "module": "main"}`
2. Или запрашивает обновление: `v1/me/o/q` → `{"version": "1.2.3", "module": "main"}`
3. Платформа отвечает командой: `v1/me/o/upd` → `{"url": "https://…/ota/fw/{token}.bin", "version": "1.3.0", "module": "main", "size": 524288, "sha256": "…", "signMethod": "sha256", "sign": "…", "force": false}`
4. Устройство загружает прошивку и публикует прогресс: `v1/me/o/p` → `{"step": 45, "module": "main", "desc": "downloading"}`
5. Отмена: `v1/me/o/can`

**Версия прошивки** (`v1/me/o/ver`):
```json
{"version": "1.2.3", "module": "main"}
```
`module` — опционален, по умолчанию `main`. Платформа обновляет `fw_version` устройства и проверяет авто-OTA.

**Запрос обновления** (`v1/me/o/q`):
```json
{"version": "1.2.3", "module": "main"}
```
Платформа ищет последнюю прошивку для этого product/module; если версия отличается от текущей — запускает OTA-задачу.

**Прогресс** (`v1/me/o/p`):
```json
{"step": 45, "module": "main", "desc": "downloading"}
```
| Значение `step` | Статус задачи |
|-----------------|---------------|
| 0–100 | `running` — обновление в процессе |
| < 0 | `failed` — ошибка |
| ≥ 101 | `done` — завершено |

### События и логи

**Событие** (`v1/me/e`) — дискретный инцидент:
```json
{
  "id": "evt-001",
  "ts": 1710000000000,
  "type": "alarm",
  "severity": "warning",
  "code": "OVERTEMP",
  "message": "Temperature exceeded threshold",
  "data": {"temperature": 85.3, "threshold": 80}
}
```
| Поле | Обязательное | Описание |
|------|-------------|----------|
| `id` | нет | Идентификатор события (уникальный на устройстве) |
| `ts` | нет | Миллисекунды; если 0 или отсутствует — время получения |
| `type` | нет | Тип события (`alarm`, `info`, …); по умолчанию `info` |
| `severity` | нет | `info`, `warning`, `error`, `critical`; по умолчанию `info` |
| `code` | нет | Код события (строка) |
| `message` | нет | Текстовое описание |
| `data` | нет | Произвольный объект с дополнительными данными |

**Логи** (`v1/me/l`) — одно сообщение или массив:
```json
[
  {"ts": 1710000000000, "level": "INFO", "module": "wifi", "message": "Connected to AP"},
  {"ts": 1710000001000, "level": "ERROR", "module": "sensor", "code": "E_TIMEOUT", "message": "Read timeout", "context": {"retries": 3}}
]
```
| Поле | Обязательное | Описание |
|------|-------------|----------|
| `ts` | нет | Миллисекунды; если 0 — время получения |
| `level` | нет | Уровень лога (`INFO`, `WARN`, `ERROR`, `DEBUG`) |
| `module` | нет | Модуль устройства (`wifi`, `sensor`, …) |
| `code` | нет | Код события (строка) |
| `message` | нет | Текст сообщения |
| `context` | нет | Произвольный объект с контекстом |

### NTP

Устройство запрашивает время: `v1/me/n/req` → платформа отвечает `v1/me/n/res` с текущим timestamp.

**Запрос от устройства:**
```json
{"id": 1, "deviceSendTime": 1710000000000}
```

**Ответ платформы** (`v1/me/n/res`):
```json
{"id": 1, "deviceSendTime": 1710000000000, "serverRecvTime": 1710000000012, "serverSendTime": 1710000000015}
```
Три метки времени позволяют устройству рассчитать смещение часов: `serverRecvTime` и `serverSendTime` — время приёма и отправки на сервере, `deviceSendTime` — время отправки на устройстве (эхо из запроса).

---

## Команды от внешних клиентов (Node-RED, плагины)

Внешние клиенты публикуют в топик `platform/v1/commands/{account_slug}/{product_id}/{device_id}/{kind}`:

| Kind | Назначение |
|------|------------|
| `rpc` | RPC-вызов на устройство |
| `attributes/desired` | Установить desired-атрибуты |
| `ota` | Запустить OTA-обновление |

События публикуются в `platform/v1/events/{account_slug}/{product_id}/{device_id}/{kind}`. Подписывайтесь в пределах своего аккаунта: `platform/v1/events/{account_slug}/#` (`platform/v1/events/#` запрещён ACL).

---

## Авторизация устройств

1. Дашборд создаёт устройство → генерирует access token → SHA-256 сохраняется в базе данных
2. MQTT CONNECT: `username={product}.{device}`, `password=token`
3. Ротация токена → новый хеш в базе; активная сессия отключается
4. Отключение / удаление устройства → сессия отключена, переподключение невозможно

Это нативная модель. Устройства стиля `alink` аутентифицируются HMAC-подписью deviceSecret, устройства `thingsboard` — access token в username без пароля (см. [Стили устройств (диалекты)](#стили-устройств-диалекты)).

## Мониторинг

Страница **MQTT** в дашборде (`GET /api/v1/mqtt`): активные сессии (client id, username, IP, подписки) и лента событий (авторизация, connect/disconnect, ACL deny, ingest, publish).

Логи в stdout (`slog`, `LOG_LEVEL`, по умолчанию Info): `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe`, `ingest`, `command`, downlink `publish`. Docker: `docker logs -f <platform-container>`.

WebSocket-событие `mqtt` транслируется при connect / disconnect / auth_fail / subscribe / ACL deny / kick.

## Внешние клиенты (Node-RED, плагины)

Внешние клиенты работают через шину правил — топики `platform/v1/events/{account_slug}/…` и `platform/v1/commands/{account_slug}/…`. Настройка описана в [06-automation.md](06-automation.md).
