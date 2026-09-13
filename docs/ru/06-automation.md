# Автоматизация (Node-RED)

Node-RED — отдельный процесс (compose / Synology). Платформа предоставляет шину правил для MQTT-автоматизации и интеграций с внешними системами, такими как Node-RED.

## Руководство по интеграции

Страница **Settings** в дашборде включает раздел **Automation & Integrations** (только для администраторов), который предоставляет:

- Краткое описание шины правил
- Шаблоны MQTT-топиков для вашего аккаунта
- Кнопка быстрого копирования шаблонов топиков
- Ссылку на эту документацию

Чтобы получить шаблон MQTT-топика для вашего аккаунта:

1. Перейдите в **Settings** в дашборде
2. Прокрутите до раздела **Automation & Integrations**
3. Нажмите **Copy MQTT topic template**, чтобы получить шаблон топика для вашего аккаунта

Шаблон топика использует slug вашего аккаунта: `platform/v1/events/{account_slug}/...`

## Мультиаккаунтные топики

С мультиаккаунтным тенанси все топики привязаны к аккаунту. Топики шины правил следуют этому паттерну:

```
platform/v1/events/{account_slug}/{product_id}/{device_id}/status
platform/v1/events/{account_slug}/{product_id}/{device_id}/telemetry
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/reported
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/events/{account_slug}/{product_id}/{device_id}/event
platform/v1/events/{account_slug}/{product_id}/{device_id}/rpc/result
platform/v1/events/{account_slug}/{product_id}/{device_id}/ota
platform/v1/events/{account_slug}/{product_id}/{device_id}/log
```

Команды:

```
platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc
platform/v1/commands/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{account_slug}/{product_id}/{device_id}/ota
```

Это обеспечивает изоляцию событий и команд каждого аккаунта. Внешние плагины (такие как Node-RED) могут получать доступ только к топикам того аккаунта, в котором они зарегистрированы.

## Шина правил

### События (платформа → Node-RED)

Пример конверта телеметрии:

```json
{
  "ts": 1451649600512,
  "product_id": "esp32-cam",
  "device_id": "cam-001",
  "keys": { "temperature": 22.5, "humidity": 61 }
}
```

### Команды (Node-RED → платформа)

```
platform/v1/commands/{product_id}/{device_id}/rpc
platform/v1/commands/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{product_id}/{device_id}/ota
```

RPC-команда (платформа выделяет device-facing `id`):

```json
{
  "method": "reboot",
  "params": { "delayMs": 1000 },
  "timeout_ms": 30000,
  "correlation_id": "nr-flow-1"
}
```

Альтернативно — REST `POST /api/v1/devices/{id}/rpc` с Bearer-токеном.

Для **поведения каждого устройства** (LED-гирлянда, расписание и т.п.) используйте [плагины устройств](07-internal-plugins.md) вместо Node-RED-потока.

## MQTT-креды Node-RED

Подключайте Node-RED к MQTT-порту **платформы** (`MQTT_ADDR`, по умолчанию `:1883`), не к отдельному брокеру.

Создайте **внешний плагин** (Дашборд → Settings → External Plugins) с именем напр. `nodered`, затем используйте сгенерированные `MQTT User` (`extplg_…`) и `MQTT Password` в MQTT-нодах Node-RED. Такой клиент может подписываться на `platform/v1/events/#` и публиковать на `platform/v1/commands/#`.

**Никогда** не помещайте токены устройств в потоки Node-RED.

### Создание плагина в дашборде

1. Войти в дашборд под **admin**.
2. **Settings** → блок **External Plugins** (внизу страницы).
3. Заполнить:
   - `Plugin ID` — короткое имя, напр. `nodered`
   - `Name` — отображаемое имя, напр. `Node-RED`
   - `Description` — по желанию
4. Нажать **Create plugin**.

Откроется модалка «Plugin created — save credentials!» с одноразовыми кредами:

| Поле | Пример | Назначение |
|------|--------|-----------|
| Plugin ID | `nodered` | идентификатор плагина |
| API Token | `…` | для REST (не нужен для MQTT) |
| MQTT User | `extplg_nodered_a1b2c3…` | username для MQTT |
| MQTT Password | `…` | пароль (показывается **один раз**) |
| MQTT Broker | `192.168.1.22:1883` | адрес брокера платформы |

Нажать **Copy all** и сохранить креды — пароль больше не показать. Если потерял — **Regenerate MQTT** в таблице плагинов выдаст новый.

### Настройка MQTT-ноды в Node-RED

1. Дважды кликнуть по **MQTT Connection** → **Edit connection node**.
2. Вкладки `Server` / `Security`:
   - **Server**: IP или hostname платформы (в docker-compose — `platform`), **Port**: `1883`
   - TLS (опционально): **Port**: `8883`, включить *Enable TLS* — см. [05-mqtt-tls.md](05-mqtt-tls.md)
3. Вкладка **Security**:
   - **User ID**: `MQTT User` из шага создания (`extplg_…`)
   - **Password**: `MQTT Password` из шага создания
4. **Done** → **Deploy**.

### Разрешённые топики (ACL)

Плагин-клиент может:

- **подписываться** на события платформы: `platform/v1/events/#`
- **публиковать** команды устройствам: `platform/v1/commands/#`

| Направление | Топик |
|-------------|-------|
| события (читать) | `platform/v1/events/{product}/{device}/telemetry` |
| события (читать) | `platform/v1/events/{product}/{device}/status` |
| команды (писать) | `platform/v1/commands/{product}/{device}/rpc` |
| команды (писать) | `platform/v1/commands/{product}/{device}/attributes/desired` |
| команды (писать) | `platform/v1/commands/{product}/{device}/ota` |

Доступ к топикам устройств (`iot/v1/…`, `v1/…`) у плагина **нет** — только шина правил.

### Проверка

1. **Deploy** в Node-RED — на MQTT-ноде должен появиться зелёный статус `connect`.
2. Дашборд → **MQTT**: в таблице *Connected clients* видна строка с kind = `plugin` и username `extplg_…`.

### Устранение проблем

| Симптом | Причина | Решение |
|---------|---------|---------|
| `not authorized` в логе Node-RED | неверный `MQTT User` / `MQTT Password` | взять креды заново; если потерял — **Regenerate MQTT** |
| `auth_fail … external plugin bad password` | пароль не совпадает с хешем | **Regenerate MQTT** и обновить ноду |
| `acl_deny` на публикацию/подписку | топик вне шины правил | использовать только `platform/v1/events/#` и `platform/v1/commands/#` |
| `:1883` не проходит, порт занят | другой брокер (Mosquitto/RabbitMQ/WinNAT) | остановить или сменить `MQTT_ADDR` |

## Пример потока

`deploy/nodered/flows.json` содержит пример: порог температуры → desired-attribute и offline debug-нода. В compose хост MQTT-брокера — `platform`.

Чтобы получить доступ к Node-RED, откройте его напрямую в браузере, используя `NODERED_URL`, настроенный в настройках вашей платформы. URL должен быть доступен **из браузера**, не из Docker-хоста `nodered`.

## Docker-compose

### Структура томов

- `nodered_data:/data` — volume для flows, credentials, node_modules
- `./nodered/settings.js:/data/settings.js:ro` — настройки Node-RED (read-only)
- `./nodered/flows.json:/templates/flows.json:ro` — шаблон flows (копируется при первом старте)
- `./nodered/entrypoint.sh:/entrypoint-custom.sh:ro` — entrypoint-скрипт

При первом старте entrypoint-скрипт копирует `flows.json` из `/templates/` в `/data/` (если его там нет). После этого Node-RED работает с файлами в volume `nodered_data`.

### Обновление flows.json

1. Отредактировать `deploy/nodered/flows.json` на хосте
2. Удалить volume: `docker compose down -v`
3. Перезапустить: `docker compose up -d`

Либо обновить flows через веб-интерфейс Node-RED (Deploy) — они сохранятся в volume.

### Переменные `NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD`

Больше не читаются платформой. Node-RED подключается как внешний плагин через креды из дашборда.

---

## HA / Cluster mode (RabbitMQ)

> **Только для cluster mode (v2.x).** Если платформа работает в single mode — этот раздел не применяется, см. секцию «MQTT-креды Node-RED» выше.

В cluster mode платформа **не предоставляет MQTT-доступ для external plugins** — только AMQP через RabbitMQ (порт 5672). Это значит, что в Node-RED нужно заменить **MQTT-ноды** на **AMQP-ноды**. Логика flow остаётся той же — меняется только transport.

### Отличия от single mode

| Аспект | Single mode | Cluster mode |
|--------|-------------|--------------|
| Брокер | Встроенный `mochi-mqtt` платформы | RabbitMQ (обязательный middleware) |
| Node-RED ноды | `mqtt in` / `mqtt out` | `amqp in` / `amqp out` |
| Палетт | `node-red-contrib-mqtt-broker` (встроен) | `node-red-contrib-amqp` (установить) |
| Подключение | `mqtt://platform:1883` | `amqp://rabbitmq:5672` |
| Subscribe events | Topic: `platform/v1/events/{slug}/{product}/{device}/#` | Exchange `iot.external` (topic) + routing key `events.{slug}.#` |
| Publish commands | Topic: `platform/v1/commands/{slug}/{product}/{device}/rpc` | Exchange `iot.commands` (topic) + routing key `commands.{slug}.{product}.{device}.{kind}` |
| Credentials | `extplg_{slug}_{id}` (username) + пароль | `extplg_{slug}_{id}` (username) + **тот же** пароль |

**Credentials (`extplg_*`) не меняются** — те же, что использовались для MQTT. Не нужно создавать новый plugin в дашборде. Пароль общий между MQTT (single) и AMQP (cluster) — это одна учётка плагина, разный только transport.

### Что установить

В Node-RED:

1. **Меню → Manage palette → Install** → `node-red-contrib-amqp` (или `@iiot-rabbitmq/amqp`).
2. Перезапустить Node-RED, чтобы палетт зарегистрировался (Deploy → перезапуск контейнера `docker compose restart nodered`).

### Пример flow: подписка на события

**Было (single mode, MQTT):**

```json
{
  "type": "mqtt in",
  "name": "Subscribe events",
  "topic": "platform/v1/events/{account_slug}/+/+/+",
  "qos": "0",
  "broker": "platform-broker"
}
```

**Стало (cluster mode, AMQP):**

```json
{
  "type": "amqp in",
  "name": "Subscribe events",
  "broker": "rabbitmq-broker",
  "exchange": "iot.external",
  "exchangeType": "topic",
  "routingKey": "events.{account_slug}.#"
}
```

`{account_slug}` подставляется реальный slug (например, `acme`, `factory-1`).

### Пример flow: публикация RPC-команды

**Было (single mode, MQTT):**

```json
{
  "type": "mqtt out",
  "name": "Publish RPC",
  "topic": "platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc",
  "qos": "0",
  "retain": "false",
  "broker": "platform-broker"
}
```

**Стало (cluster mode, AMQP):**

```json
{
  "type": "amqp out",
  "name": "Publish RPC",
  "broker": "rabbitmq-broker",
  "exchange": "iot.commands",
  "exchangeType": "topic",
  "routingKey": "commands.{account_slug}.{product_id}.{device_id}.rpc"
}
```

### Конфигурация AMQP broker ноды

В Node-RED создать config-ноду типа `amqp-broker`:

| Поле | Значение |
|------|----------|
| **Name** | `rabbitmq-broker` (любое, как в нодах выше) |
| **Host** | `rabbitmq` (в docker-compose) или `<rabbitmq-host>` (bare-metal) |
| **Port** | `5672` |
| **Username** | `extplg_<account_slug>_<plugin_id>` (из дашборда, поле «MQTT User») |
| **Password** | сохранённый пароль из дашборда (тот же, что для MQTT) |
| **VHost** | `/` (по умолчанию) |
| **Use TLS** | включить, если RabbitMQ за TLS (порт 5671) |

### Mapping топиков (single → cluster)

| Single mode MQTT topic | Cluster mode AMQP exchange + routing key |
|------------------------|-------------------------------------------|
| `platform/v1/events/{slug}/{product}/{device}/telemetry` | exchange `iot.external`, routing key `events.{slug}.{product}.{device}.telemetry` |
| `platform/v1/events/{slug}/{product}/{device}/status` | exchange `iot.external`, routing key `events.{slug}.{product}.{device}.status` |
| `platform/v1/events/{slug}/{product}/{device}/rpc/result` | exchange `iot.external`, routing key `events.{slug}.{product}.{device}.rpc.result` |
| `platform/v1/events/{slug}/{product}/{device}/ota` | exchange `iot.external`, routing key `events.{slug}.{product}.{device}.ota` |
| `platform/v1/events/#` (subscribe all) | exchange `iot.external`, routing key `events.{slug}.#` |
| `platform/v1/commands/{slug}/{product}/{device}/rpc` | exchange `iot.commands`, routing key `commands.{slug}.{product}.{device}.rpc` |
| `platform/v1/commands/{slug}/{product}/{device}/attributes/desired` | exchange `iot.commands`, routing key `commands.{slug}.{product}.{device}.attributes.desired` |
| `platform/v1/commands/{slug}/{product}/{device}/ota` | exchange `iot.commands`, routing key `commands.{slug}.{product}.{device}.ota` |

**Важно про точки vs слэши:** AMQP routing key использует **точку** как разделитель, не слэш. `events.acme.esp32-cam.cam-001.telemetry`, не `events/acme/esp32-cam/cam-001/telemetry`.

### Пошаговый migration guide (single → cluster)

1. **Остановить Node-RED** (или работать параллельно с двумя flow'ами во время миграции).
2. **Установить палетт** `node-red-contrib-amqp` через Manage palette.
3. **Экспортировать текущий flow** (Ctrl+A → Ctrl+E → сохранить JSON в файл).
4. **Заменить ноды**:
   - `mqtt in` → `amqp in`
   - `mqtt out` → `amqp out`
   - `mqtt-broker` (config) → `amqp-broker` (config)
5. **Обновить топики** по mapping-таблице выше (MQTT topic → AMQP exchange + routing key).
6. **Создать `amqp-broker` config-ноду** с кредами `extplg_*` (теми же, что для MQTT).
7. **Import flow** в Node-RED (Ctrl+I → вставить JSON), **Deploy**.
8. **Проверить**:
   - На `amqp in` ноде — статус `connect` (зелёный).
   - Вкладка `Debug` показывает входящие события.
   - В дашборде Settings → External Plugins статус плагина `online`.

### Развёртывание Node-RED в cluster mode

В `docker-compose.yml` для cluster mode сервис `nodered` должен:
- иметь сетевой доступ к RabbitMQ (link на `rabbitmq` или `networks: [cluster_net]`)
- **НЕ** зависеть от `platform:1883` для MQTT (в cluster mode этот порт не работает для external plugins)
- `NODERED_URL` остаётся как раньше — это URL для iframe в дашборде (UI)

```yaml
services:
  nodered:
    image: nodered/node-red:latest
    networks:
      - cluster_net   # общая сеть с rabbitmq
    environment:
      - NODERED_URL=http://nodered:1880
    # ... volumes, entrypoint — те же

  rabbitmq:
    image: rabbitmq:3-management
    # ... с включённым MQTT plugin
```

### Устранение проблем

| Симптом | Причина | Решение |
|---------|---------|---------|
| `access refused` / `NOT_ALLOWED` в логе Node-RED | неверный `extplg_*` username или пароль | взять креды заново из дашборда; если потерял — **Regenerate MQTT** (пароль общий между MQTT и AMQP) |
| `connection refused` на `5672` | Node-RED не видит RabbitMQ | проверить `networks` в docker-compose, доступность `rabbitmq:5672` |
| Routing key не доходит до flow | exchange или routing key не совпадают | проверить `iot.external` / `iot.commands` (topic exchange), маску `events.{slug}.#` |
| События приходят с `events.{slug}.#`, но не с `events.{slug}.{product}.#` | routing key не совпадает | проверить формат routing key (точка, не слэш) |
| Node-RED получает события, но не публикует команды | exchange `iot.commands` не настроен как topic | RabbitMQ по умолчанию `iot.commands` уже topic; см. `deploy/rabbitmq/definitions.json` |
| Payload приходит как `Buffer`, а не `Object` | AMQP нода не парсит JSON автоматически | в `amqp in` ноде: отметить `msg.payload` как parsed, либо добавить `JSON.parse` ноду перед обработкой |

### Связанные документы

- [17-ha-architecture.md → §2.5.1 Node-RED: переход с MQTT на AMQP](17-ha-architecture.md#251-node-red-переход-с-mqtt-на-amqp) — краткая сводка
- [17-ha-architecture.md → §2.5 External Plugins (только AMQP)](17-ha-architecture.md#25-external-plugins-только-amqp) — общая модель
- [17-ha-architecture.md → §4 External Plugin SDK](17-ha-architecture.md#4-external-plugin-sdk) — программный SDK (не для Node-RED, но для понимания модели)
