# Внешние плагины — примеры (MQTT)

Примеры внешних плагинов с подпиской на MQTT rules bus.

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
  -d '{"login":"admin","password":"admin"}' | jq -r '.token')
```

### Два режима запуска

1. **Регистрация** — `--token <admin-jwt>` — плагин регистрируется, получает credentials
2. **Готовые credentials** — `--mqtt-user` / `--mqtt-pass` или env vars — без регистрации

#### Python

```bash
pip install requests paho-mqtt
cd examples/plugins/python

# Режим 1: Регистрация
python mqtt_telemetry_logger.py --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
python mqtt_telemetry_logger.py --mqtt-user extplg_logger --mqtt-pass <password>

# Режим 3: Environment variables
export MQTT_USER=extplg_logger
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
go run mqtt_telemetry_logger.go --mqtt-user extplg_logger --mqtt-pass <password>

# Режим 3: Environment variables
MQTT_USER=extplg_logger MQTT_PASS=<password> go run mqtt_telemetry_logger.go
```

#### Java

```bash
cd examples/plugins/java
mvn clean package

# Режим 1: Регистрация
java -jar target/mqtt-telemetry-logger-1.0.0.jar --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
java -jar target/mqtt-telemetry-logger-1.0.0.jar --mqtt-user extplg_logger --mqtt-pass <password>

# Режим 3: Environment variables
MQTT_USER=extplg_logger MQTT_PASS=<password> java -jar target/mqtt-telemetry-logger-1.0.0.jar
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
./build/mqtt_telemetry_logger --mqtt-user extplg_logger --mqtt-pass <password>

# Режим 3: Environment variables
MQTT_USER=extplg_logger MQTT_PASS=<password> ./build/mqtt_telemetry_logger
```

---

## Как это работает

1. Плагин регистрируется на платформе через `POST /api/v1/plugins/external` (нужен admin JWT)
2. Платформа возвращает MQTT credentials (username/password) + host/port
3. Плагин подключается к MQTT брокеру с этими credentials
4. Подписывается на `platform/v1/events/#` — все события всех устройств
5. Логирует telemetry, status, events, OTA в консоль

```
┌─────────────────────────────────────────────────────────────┐
│  MQTT Plugin                                                │
├─────────────────────────────────────────────────────────────┤
│  1. Registration (POST /api/v1/plugins/external)           │
│     → Получает: token, mqtt_user, mqtt_pass, host, port    │
│                                                             │
│  2. MQTT Connect (tcp://host:port)                         │
│     → Auth: mqtt_user / mqtt_pass                          │
│                                                             │
│  3. Subscribe (platform/v1/events/#)                       │
│     → Получает все события устройств                       │
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
3. Реализуйте логику обработки событий
4. Запустите и привяжите к устройству через UI

### Формат событий

**Telemetry:**
```json
{"metrics": {"temperature": 23.5, "humidity": 65}}
```

**Status:**
```json
{"online": true}
```

**Event:**
```json
{"type": "button_press", "value": 1}
```

### Преимущества MQTT

| | MQTT |
|---|------|
| Подключение | Плагин → Платформа (MQTT subscribe) |
| NAT traversal | Не нужен публичный IP |
| Привязка к устройству | Не нужна — видит все события |
| ACL | Rules bus (events read, commands write) |
| Зависимости | MQTT клиент |
