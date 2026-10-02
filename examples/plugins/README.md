# Внешние плагины — примеры (MQTT)

Примеры внешних плагинов с подпиской на MQTT rules bus. MQTT — единственный реализованный
транспорт внешних плагинов (см. [docs/ru/08-external-plugins.md](../../docs/ru/08-external-plugins.md)).

Внешний плагин может читать только события своего аккаунта —
`{rules_bus}/events/{account_slug}/#`; подписку на `{rules_bus}/events/#` ACL брокера отклоняет.
`account_slug` в ответе регистрации нет — примеры берут его из `mqtt_user`: это часть между
`extplg_` и следующим `_` (`extplg_acme_mqtt-telemetry-logger` → `acme`).

## Примеры

| Язык | Файл | Описание |
|------|------|----------|
| Python | `python/mqtt_telemetry_logger.py` | Подписка на rules bus через MQTT |
| Go | `go/mqtt_telemetry_logger.go` | Подписка на rules bus через MQTT |
| Java | `java/MqttTelemetryLogger.java` | Подписка на rules bus через MQTT |
| C++ | `cpp/mqtt_telemetry_logger.cpp` | Подписка на rules bus через MQTT |

---

## Запуск

### Получение Admin JWT

```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"login":"<admin-login>","password":"<admin-password>"}' | jq -r '.token')
```

Логин и пароль — те, что заданы при первом запуске в setup wizard. Плагин регистрируется в
**активном аккаунте** этого пользователя.

### Два режима запуска

1. **Регистрация** — `--token <admin-jwt>` — плагин регистрируется и получает MQTT credentials
   (`mqtt_user`, `mqtt_pass`, host/port, `rules_bus`; API-токена в ответе нет)
2. **Готовые credentials** — `--mqtt-user` / `--mqtt-pass` или env vars — без регистрации
   (креды из Settings → External Plugins; username всегда вида `extplg_{account_slug}_{plugin_id}`)

#### Python

```bash
pip install requests paho-mqtt
cd examples/plugins/python

# Режим 1: Регистрация
python mqtt_telemetry_logger.py --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
python mqtt_telemetry_logger.py --mqtt-user extplg_acme_mqtt-telemetry-logger --mqtt-pass <password>

# Режим 3: Environment variables
export MQTT_USER=extplg_acme_mqtt-telemetry-logger
export MQTT_PASS=<password>
python mqtt_telemetry_logger.py
```

#### Go

```bash
cd examples/plugins/go
go mod tidy

# Режим 1: Регистрация
go run mqtt_telemetry_logger.go --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
go run mqtt_telemetry_logger.go --mqtt-user extplg_acme_mqtt-telemetry-logger --mqtt-pass <password>

# Режим 3: Environment variables
MQTT_USER=extplg_acme_mqtt-telemetry-logger MQTT_PASS=<password> go run mqtt_telemetry_logger.go
```

#### Java

```bash
cd examples/plugins/java
mvn clean package

# Режим 1: Регистрация
java -jar target/mqtt-telemetry-logger-1.0.0.jar --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
java -jar target/mqtt-telemetry-logger-1.0.0.jar --mqtt-user extplg_acme_mqtt-telemetry-logger --mqtt-pass <password>

# Режим 3: Environment variables
MQTT_USER=extplg_acme_mqtt-telemetry-logger MQTT_PASS=<password> java -jar target/mqtt-telemetry-logger-1.0.0.jar
```

#### C++

```bash
# Установить зависимости (Ubuntu/Debian)
sudo apt install libpaho-mqttpp3-dev nlohmann-json3-dev libcurl4-openssl-dev

cd examples/plugins/cpp
cmake -B build -S .
cmake --build build

# Режим 1: Регистрация
./build/mqtt_telemetry_logger --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
./build/mqtt_telemetry_logger --mqtt-user extplg_acme_mqtt-telemetry-logger --mqtt-pass <password>

# Режим 3: Environment variables
MQTT_USER=extplg_acme_mqtt-telemetry-logger MQTT_PASS=<password> ./build/mqtt_telemetry_logger
```

---

## Как это работает

1. Плагин регистрируется на платформе через `POST /api/v1/plugins/external` (нужен admin JWT)
2. Платформа возвращает `ok`, `id`, `mqtt_user` (`extplg_{account_slug}_{plugin_id}`), `mqtt_pass`,
   `mqtt_host`, `mqtt_port`, `mqtt_ws`, `rules_bus` — **без** API-токена
3. Плагин подключается к MQTT брокеру с этими credentials
4. Подписывается на `{rules_bus}/events/{account_slug}/#` — все события устройств своего аккаунта
5. Логирует telemetry, status, events, OTA в консоль

```
┌─────────────────────────────────────────────────────────────┐
│  MQTT Plugin                                                │
├─────────────────────────────────────────────────────────────┤
│  1. Registration (POST /api/v1/plugins/external)           │
│     → Получает: mqtt_user, mqtt_pass, host, port, rules_bus│
│                                                             │
│  2. MQTT Connect (tcp://host:port)                         │
│     → Auth: mqtt_user / mqtt_pass                          │
│                                                             │
│  3. Subscribe ({rules_bus}/events/{account_slug}/#)        │
│     → Получает события устройств своего аккаунта           │
│                                                             │
│  4. Process Events                                         │
│     → telemetry, status, event, log, ota                   │
└─────────────────────────────────────────────────────────────┘
```

---

## API платформы

| Метод | Описание |
|-------|----------|
| `POST /api/v1/plugins/external` | Регистрация плагина (admin JWT) |
| `GET /api/v1/plugins/external` | Список плагинов (auth) |
| `DELETE /api/v1/plugins/external/{id}` | Удаление плагина (admin) |
| `POST /api/v1/plugins/external/{id}/regenerate-mqtt` | Обновить MQTT пароль (admin) |

---

## Создание своего плагина

1. Скопируйте пример на нужном языке
2. Измените `id`, `name`, `description` в коде
3. Подпишитесь на `{rules_bus}/events/{account_slug}/#` (или уже, например `{rules_bus}/events/{account_slug}/+/+/telemetry`)
4. Реализуйте логику обработки событий
5. Запустите — привязка к устройству не нужна (binding-и для внешних плагинов не реализованы)

Команды устройствам публикуются в
`{rules_bus}/commands/{account_slug}/{product_id}/{device_id}/{rpc|attributes/desired|ota}` — только со slug
своего аккаунта; устройство ищется внутри этого аккаунта, форма без slug не принимается
(см. [docs/ru/06-automation.md](../../docs/ru/06-automation.md#мультиаккаунтные-топики)).

### Формат событий

Топик: `{rules_bus}/events/{account_slug}/{product_id}/{device_id}/{kind}`, где `kind` —
`status`, `telemetry`, `event`, `attributes/reported`, `attributes/desired`, `rpc/result`, `ota`, `log`.

**Telemetry:**
```json
{"ts": 1451649600512, "product_id": "esp32-cam", "device_id": "cam-001", "keys": {"temperature": 23.5, "humidity": 65}}
```

**Status:**
```json
{"ts": 1451649600512, "product_id": "esp32-cam", "device_id": "cam-001", "online": true}
```

**Event:**
```json
{"ts": 1451649600512, "product_id": "esp32-cam", "device_id": "cam-001", "type": "button_press", "severity": "info", "code": "", "message": "", "data": {"value": 1}}
```

### Преимущества MQTT

| | MQTT |
|---|------|
| Подключение | Плагин → Платформа (MQTT subscribe) |
| NAT traversal | Не нужен публичный IP |
| Привязка к устройству | Не нужна — видит все события своего аккаунта |
| ACL | Rules bus: чтение `events/{account_slug}/…`, запись `commands/…` |
| Зависимости | MQTT клиент |
