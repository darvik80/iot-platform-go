# Подключение Node-RED как External Plugin

Инструкция на русском. English: [automation.md](automation.md).

Раньше Node-RED подключался к MQTT-брокеру платформы под выделенным пользователем `nodered` (креды генерировались платформой и показывались в Settings). **Это удалено.** Теперь Node-RED — обычный внешний плагин: создаёте плагин в дашборде, получаете `MQTT User` (`extplg_…`) и `MQTT Password`, вставляете их в MQTT-ноды Node-RED.

## 1. Создать плагин в дашборде

1. Войти в дашборд под **admin**.
2. **Settings** → блок **External Plugins** (внизу страницы).
3. Заполни:
   - `Plugin ID` — короткое имя, напр. `nodered`
   - `Name` — отображаемое имя, напр. `Node-RED`
   - `Description` — по желанию
4. Нажми **Create plugin**.

Откроется модалка «Plugin created — save credentials!» с одноразовыми кредо:

| Поле | Пример | Назначение |
|------|--------|-----------|
| Plugin ID | `nodered` | идентификатор плагина |
| API Token | `…` | для REST (не нужен для MQTT) |
| MQTT User | `extplg_nodered_a1b2c3…` | username для MQTT |
| MQTT Password | `…` | пароль (показывается **один раз**) |
| MQTT Broker | `192.168.1.22:1883` | адрес брокера платформы |

Нажми **Copy all** и сохрани creds — пароль больше не показать. Если потерял — **Regenerate MQTT** в таблице плагинов выдаст новый.

## 2. Настроить MQTT-ноду в Node-RED

Открой редактор Node-RED (страница **Automation** в дашборде или `http://<host>:1880`).

1. Дважды кликни по **MQTT Connection** (broker config) любой MQTT-ноды → **Edit connection node**.
2. Вкладки `Server` / `Security`:
   - **Server**: IP или hostname платформы (в docker-compose — `platform`), **Port**: `1883`
   - TLS (по желанию): **Port**: `8883`, включи *Enable TLS* — см. [mqtt-tls.md](mqtt-tls.md)
3. Вкладка **Security**:
   - **User ID**: `MQTT User` из шага 1 (`extplg_…`)
   - **Password**: `MQTT Password` из шага 1
4. **Done** → **Deploy**.

## 3. Разрешённые топики (ACL)

Плагин-клиент может:

- **подписываться** на события платформы:
  ```
  platform/v1/events/#
  ```
- **публиковать** команды устройствам:
  ```
  platform/v1/commands/#
  ```

Примеры:

| Направление | Топик |
|-------------|-------|
| события (читать) | `platform/v1/events/{product}/{device}/telemetry` |
| события (читать) | `platform/v1/events/{product}/{device}/status` |
| команды (писать) | `platform/v1/commands/{product}/{device}/rpc` |
| команды (писать) | `platform/v1/commands/{product}/{device}/attributes/desired` |
| команды (писать) | `platform/v1/commands/{product}/{device}/ota` |

Доступ к топикам устройств (`iot/v1/…`, `v1/…`) у плагина **нет** — только шина правил.

## 4. Проверка

1. **Deploy** в Node-RED — на MQTT-ноде должен появиться зелёный статус `connect`.
2. Дашборд → **MQTT**: в таблице *Connected clients* видна строка с kind = `plugin` и username `extplg_…`.
3. Если статус красный / строки нет — смотри раздел «Устранение проблем» ниже.

## 5. Устранение проблем

| Симптом | Причина | Решение |
|---------|---------|---------|
| `not authorized` в логе Node-RED | неверный `MQTT User` / `MQTT Password` | возьми creds заново; если потерял — **Regenerate MQTT** |
| В MQTT-событих `auth_fail … external plugin bad password` | пароль не совпадает с хешем плагина | **Regenerate MQTT** и обнови ноду |
| `acl_deny` на публикацию/подписку | топик вне шины правил | используй только `platform/v1/events/#` (чтение) и `platform/v1/commands/#` (запись) |
| Подключение по `:1883` не проходит, порт занят | другой брокер (Mosquitto/RabbitMQ/WinNAT) | останови его или смени `MQTT_ADDR` |
| TLS `:8883` отклоняет сертификат | нет CA / самоподписанный cert | см. [mqtt-tls.md](mqtt-tls.md) |
| `EBUSY: resource busy or locked, rename flows.json` | flows.json смонтирован напрямую с хоста | обнови `docker-compose.yml` (см. раздел 6) или удали volume и перезапусти |

## 6. Docker-compose

### Структура томов

- `nodered_data:/data` — volume для flows, credentials, node_modules (сохраняется между перезапусками)
- `./nodered/settings.js:/data/settings.js:ro` — настройки Node-RED (read-only)
- `./nodered/flows.json:/templates/flows.json:ro` — шаблон flows (копируется при первом старте)
- `./nodered/entrypoint.sh:/entrypoint-custom.sh:ro` — entrypoint-скрипт

### Как это работает

При первом старте контейнера entrypoint-скрипт копирует `flows.json` из `/templates/` в `/data/` (если его там нет). После этого Node-RED работает с файлами в volume `nodered_data`, что позволяет сохранять изменения flows без ошибок `EBUSY`.

### Обновление flows.json

Если нужно обновить начальные flows:
1. Отредактируй `deploy/nodered/flows.json` на хосте
2. Удали volume: `docker compose down -v` (или `docker volume rm cogitor_nodered_data`)
3. Перезапусти: `docker compose up -d`

Либо обнови flows через веб-интерфейс Node-RED (Deploy) — они сохранятся в volume.

### Переменные `NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD`

Больше не читаются платформой. Node-RED подключается как external plugin через creds из дашборда (шаг 1).
