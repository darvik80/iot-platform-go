# Внешние плагины — примеры

Примеры внешних плагинов для IoT Platform.

## Типы плагинов

| Тип | Описание | Когда использовать |
|-----|----------|-------------------|
| **Webhook** | Платформа → Плагин (HTTP POST) | Нужен публичный IP, минимальная задержка |
| **Polling** | Плагин → Платформа (HTTP poll) | Плагин за NAT, опрашивает события |
| **MQTT** | Плагин → Брокер (MQTT subscribe) | Node-RED стиль, видит все события |
| **gRPC** | Платформа → Плагин (gRPC) | Высокая производительность, строгая типизация |

## Примеры плагинов

### Python

| Файл | Тип | Описание |
|------|-----|----------|
| `python/telemetry_logger.py` | Webhook | Простой webhook плагин с Flask |
| `python/mqtt_telemetry_logger.py` | MQTT | Подписка на rules bus через MQTT |

### Go

| Файл | Тип | Описание |
|------|-----|----------|
| `go/telemetry_logger.go` | Webhook | Webhook плагин (stdlib only) |
| `go/mqtt_telemetry_logger.go` | MQTT | Подписка на rules bus через MQTT |

### Java

| Файл | Тип | Описание |
|------|-----|----------|
| `java/TelemetryLogger.java` | Webhook | Webhook плагин (Gson + HttpServer) |
| `java/MqttTelemetryLogger.java` | MQTT | Подписка на rules bus через MQTT |

### C++

| Файл | Тип | Описание |
|------|-----|----------|
| `cpp/telemetry_logger.cpp` | Webhook | Webhook плагин (cpp-httplib) |
| `cpp/mqtt_telemetry_logger.cpp` | MQTT | Подписка на rules bus через MQTT |

### gRPC (Go)

| Файл | Тип | Описание |
|------|-----|----------|
| `grpc/telemetry_logger_grpc.go` | gRPC | gRPC сервер плагин |
| `grpc/plugin.proto` | Proto | Определение protobuf сервиса |

---

## Запуск примеров

### Получение Admin JWT

Все примеры требуют admin JWT для регистрации плагина:

```bash
# Получить токен через логин
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"login":"admin","password":"admin"}' | jq -r '.token')

echo "Token: $TOKEN"
```

### Webhook плагины

#### Python (Webhook)

```bash
# Установить зависимости
pip install requests flask

# Запустить плагин
cd examples/plugins/python
python telemetry_logger.py
```

#### Go (Webhook)

```bash
# Зависимости не нужны (stdlib)
cd examples/plugins/go
go run telemetry_logger.go --platform http://localhost:8080 --token $TOKEN --port 9000
```

#### Java (Webhook)

```bash
# Собрать проект
cd examples/plugins/java
mvn clean package

# Запустить плагин
java -cp target/mqtt-telemetry-logger-1.0.0.jar TelemetryLogger --platform http://localhost:8080 --token $TOKEN --port 9000
```

#### C++ (Webhook)

```bash
# Установить зависимости (Ubuntu/Debian)
sudo apt install nlohmann-json3-dev libcurl4-openssl-dev

# Собрать проект (cpp-httplib скачается автоматически)
cd examples/plugins/cpp
cmake -B build -S .
cmake --build build --target telemetry_logger

# Запустить плагин
./build/telemetry_logger --platform http://localhost:8080 --token $TOKEN --port 9000
```

### MQTT плагины

Все MQTT примеры поддерживают **два режима запуска**:

1. **Регистрация** — `--token <admin-jwt>` — плагин регистрируется, получает credentials
2. **Готовые credentials** — `--mqtt-user` / `--mqtt-pass` или env vars — без регистрации

#### Python (MQTT)

```bash
# Установить зависимости
pip install requests paho-mqtt
cd examples/plugins/python

# Режим 1: Регистрация (нужен admin JWT)
python mqtt_telemetry_logger.py --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
python mqtt_telemetry_logger.py --mqtt-user extplg_logger --mqtt-pass <password>

# Режим 3: Environment variables
export MQTT_USER=extplg_logger
export MQTT_PASS=<password>
python mqtt_telemetry_logger.py
```

#### Go (MQTT)

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

#### Java (MQTT)

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

#### C++ (MQTT)

```bash
# Установить зависимости (Ubuntu/Debian)
sudo apt install libpaho-mqttpp3-dev nlohmann-json3-dev libcurl4-openssl-dev

cd examples/plugins/cpp
cmake -B build -S .
cmake --build build --target mqtt_telemetry_logger

# Режим 1: Регистрация
./build/mqtt_telemetry_logger --platform http://localhost:8080 --token $TOKEN

# Режим 2: Готовые credentials
./build/mqtt_telemetry_logger --mqtt-user extplg_logger --mqtt-pass <password>

# Режим 3: Environment variables
MQTT_USER=extplg_logger MQTT_PASS=<password> ./build/mqtt_telemetry_logger
```

### gRPC плагины

#### Go (gRPC)

```bash
# Установить зависимости
cd examples/plugins/grpc
go mod tidy

# Запустить плагин (gRPC сервер на порту 50051)
go run telemetry_logger_grpc.go --platform http://localhost:8080 --token $TOKEN --port 50051
```

**Регистрация gRPC плагина:**
```bash
curl -X POST http://localhost:8080/api/v1/plugins/external \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id": "telemetry-logger-grpc",
    "name": "Telemetry Logger (gRPC)",
    "description": "Logs device telemetry via gRPC",
    "endpoint": "localhost:50051"
  }'
```

> **Примечание:** Для gRPC плагинов `endpoint` указывается без `http://` префикса.
> Платформа определяет транспорт автоматически: `http://` → webhook, иначе → gRPC.

---

## Как это работает

### Webhook плагины

1. Плагин регистрируется с указанием `endpoint` (URL для callbacks)
2. Платформа отправляет события через HTTP POST на endpoint
3. Плагин обрабатывает JSON-RPC методы:
   - `plugin.on_event` — событие от устройства (telemetry, status, event)
   - `plugin.apply_config` — привязка к устройству с конфигурацией
   - `plugin.detach` — отвязка от устройства
4. Плагин отправляет heartbeat каждые 30 секунд

### MQTT плагины

1. Плагин регистрируется на платформе через `POST /api/v1/plugins/external` (нужен admin JWT)
2. Платформа возвращает MQTT credentials (username/password) + host/port
3. Плагин подключается к MQTT брокеру с этими credentials
4. Подписывается на `platform/v1/events/#` — все события всех устройств
5. Логирует telemetry, status, events, OTA в консоль

### gRPC плагины

1. Плагин регистрируется с указанием `endpoint` в формате `host:port` (без `http://`)
2. Платформа подключается к плагину как gRPC клиент
3. Плагин реализует `PluginService` с методами:
   - `OnEvent` — событие от устройства
   - `ApplyConfig` — привязка к устройству с конфигурацией
   - `Detach` — отвязка от устройства
   - `Ping` — health check (вместо heartbeat)
4. Платформа автоматически проверяет здоровье через `Ping` каждые 30 секунд

---

## API платформы

| Метод | Описание |
|-------|----------|
| `POST /api/v1/plugins/external` | Регистрация плагина (admin JWT) |
| `GET /api/v1/plugins/external` | Список плагинов (auth) |
| `DELETE /api/v1/plugins/external/{id}` | Удаление плагина (admin) |
| `POST /api/v1/plugins/external/{id}/regenerate-mqtt` | Обновить MQTT пароль (admin) |
| `POST /api/plugins/external/rpc` | JSON-RPC endpoint (plugin token) |

---

## Создание своего плагина

1. Скопируйте пример на нужном языке
2. Измените `id`, `name`, `description` в коде
3. Реализуйте логику обработки событий
4. Запустите и привяжите к устройству через UI

### Структура Webhook плагина

```
┌─────────────────────────────────────────────────────────────┐
│  Webhook Plugin                                             │
├─────────────────────────────────────────────────────────────┤
│  1. Registration (POST /api/v1/plugins/external)           │
│     → Получает: token, endpoint                            │
│                                                             │
│  2. HTTP Server (порт 9000)                                │
│     → POST /rpc — JSON-RPC handler                         │
│                                                             │
│  3. Heartbeat (каждые 30 сек)                              │
│     → POST /api/plugins/external/rpc                       │
│                                                             │
│  4. Process Events                                         │
│     → plugin.on_event, plugin.apply_config, plugin.detach  │
└─────────────────────────────────────────────────────────────┘
```

### Структура MQTT плагина

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

### Структура gRPC плагина

```
┌─────────────────────────────────────────────────────────────┐
│  gRPC Plugin                                                │
├─────────────────────────────────────────────────────────────┤
│  1. Registration (POST /api/v1/plugins/external)           │
│     → Получает: token, endpoint (host:port)                │
│                                                             │
│  2. gRPC Server (порт 50051)                               │
│     → Реализует PluginService (OnEvent, ApplyConfig, ...)  │
│                                                             │
│  3. Health Check (Ping)                                    │
│     → Платформа вызывает Ping каждые 30 сек                │
│                                                             │
│  4. Process Events                                         │
│     → OnEvent, ApplyConfig, Detach                         │
└─────────────────────────────────────────────────────────────┘
```

### Формат событий

**Telemetry:**
```json
{
  "metrics": {
    "temperature": 23.5,
    "humidity": 65
  }
}
```

**Status:**
```json
{
  "online": true
}
```

**Event:**
```json
{
  "type": "button_press",
  "value": 1
}
```

### JSON-RPC методы (Webhook)

**plugin.on_event** — событие от устройства:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "plugin.on_event",
  "params": {
    "binding_id": 1,
    "device_pk": 7,
    "kind": "telemetry",
    "payload": {"temperature": 23.5}
  }
}
```

**plugin.apply_config** — привязка к устройству:
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "plugin.apply_config",
  "params": {
    "binding_id": 1,
    "device_pk": 7,
    "config": {}
  }
}
```

**plugin.detach** — отвязка от устройства:
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "plugin.detach",
  "params": {
    "binding_id": 1
  }
}
```

---

## Сравнение транспортов

| | Webhook | MQTT | gRPC |
|---|---------|------|------|
| Подключение | Платформа → Плагин (HTTP POST) | Плагин → Платформа (MQTT) | Платформа → Плагин (gRPC) |
| NAT traversal | Нужен публичный IP | Не нужен | Нужен публичный IP |
| Задержка | Минимальная | Минимальная | Минимальная |
| Привязка к устройству | Нужна через UI | Не нужна — видит все | Нужна через UI |
| ACL | Только свой binding | Rules bus (events read, commands write) | Только свой binding |
| Health check | Heartbeat (POST) | Не нужен | Ping (gRPC) |
| Зависимости | HTTP сервер | MQTT клиент | gRPC сервер |
| Типизация | JSON (динамическая) | JSON (динамическая) | Protobuf (строгая) |
| Производительность | Средняя | Средняя | Высокая |
