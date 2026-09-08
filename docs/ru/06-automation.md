# Автоматизация (Node-RED)

Node-RED — отдельный процесс (compose / Synology). Страница **Automation** в дашборде встраивает редактор в iframe (`NODERED_URL`, напр. `http://192.168.1.100:1880`). Same-origin reverse proxy опционален; iframe достаточно для лабы, если Node-RED разрешает `frame-ancestors` (см. `deploy/nodered/settings.js`).

## Шина правил

### События (платформа → Node-RED)

```
platform/v1/events/{product_id}/{device_id}/status
platform/v1/events/{product_id}/{device_id}/telemetry
platform/v1/events/{product_id}/{device_id}/attributes/reported
platform/v1/events/{product_id}/{device_id}/attributes/desired
platform/v1/events/{product_id}/{device_id}/event
platform/v1/events/{product_id}/{device_id}/rpc/result
platform/v1/events/{product_id}/{device_id}/ota
platform/v1/events/{product_id}/{device_id}/log
```

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

Дашборд → **Automation** показывает редактор inline (и «Open in new tab»). `NODERED_URL` должен быть доступен **из браузера**, не из Docker-хоста `nodered`.

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
