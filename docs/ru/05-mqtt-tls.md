# MQTT over TLS

MQTT TLS (mqtts) шифрует трафик между устройствами / клиентами и встроенным MQTT-брокером.
Платформа поддерживает **серверный TLS** (стандартный) и **взаимный TLS** (проверка клиентского сертификата).

## Быстрый старт

```bash
# 1. Сгенерировать сертификаты
make gen-mqtt-tls

# 2. Установить переменные окружения
export MQTT_TLS_CERT=./certs/mqtt/cert.pem
export MQTT_TLS_KEY=./certs/mqtt/key.pem

# 3. Запустить платформу
./bin/platform
```

TLS-слушатель запускается на `MQTT_TLS_ADDR` (по умолчанию `:8883`).

## Конфигурация

| Переменная | YAML-ключ | По умолчанию | Описание |
|-----------|-----------|-------------|----------|
| `MQTT_TLS_ADDR` | `mqtt.tls_addr` | `:8883` | Адрес слушателя MQTT-over-TLS |
| `MQTT_TLS_CERT` | `mqtt.tls_cert` | — | Путь к серверному сертификату (PEM) |
| `MQTT_TLS_KEY` | `mqtt.tls_key` | — | Путь к приватному ключу (PEM) |
| `MQTT_TLS_CA` | `mqtt.tls_ca` | — | Путь к CA-сертификату — включает **взаимный TLS** |

TLS включён, когда установлены **оба** `MQTT_TLS_CERT` и `MQTT_TLS_KEY`.
Без них доступны только обычный MQTT (`:1883`) и WebSocket (`:8083`).

**Разрешение путей**: пути сертификатов могут быть абсолютными (`/etc/certs/cert.pem`) или относительными.
Относительные пути определяются в порядке:
1. Текущий рабочий каталог
2. Исходный каталог (через `runtime.Caller` — работает в `go run` / IDE)
3. Каталог исполняемого файла (Docker/systemd)

### Пример config.yml

```yaml
mqtt:
  addr: ":1883"
  ws_addr: ":8083"
  tls_addr: ":8883"
  tls_cert: "/etc/iot-platform-go/certs/cert.pem"
  tls_key: "/etc/iot-platform-go/certs/key.pem"
  # tls_ca: "/etc/iot-platform-go/certs/ca.pem"   # раскомментировать для взаимного TLS
  broker_host_public: "192.168.1.22"
```

## Генерация сертификатов

### Скриптом (рекомендуется)

```bash
make gen-mqtt-tls              # генерация в ./certs/mqtt/
make gen-mqtt-tls DIR=certs    # другой каталог
```

Создаёт:
- `ca.pem` / `ca-key.pem` — самоподписанный CA (действителен 10 лет)
- `cert.pem` / `key.pem` — серверный сертификат, подписанный CA (действителен 1 год)
- `client.pem` / `client-key.pem` — клиентский сертификат для взаимного TLS (действителен 1 год)

### Через openssl

```bash
# CA
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout ca-key.pem -out ca.pem -days 3650 -nodes \
  -subj "/CN=Cogitor IoT CA"

# Серверный сертификат
openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout key.pem -out server.csr -nodes \
  -subj "/CN=iot.local"
openssl x509 -req -in server.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out cert.pem -days 365
rm server.csr ca.srl

# Клиентский сертификат (для взаимного TLS)
openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout client-key.pem -out client.csr -nodes \
  -subj "/CN=device-client"
openssl x509 -req -in client.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out client.pem -days 365
rm client.csr ca.srl
```

## Взаимный TLS

Когда `MQTT_TLS_CA` установлен, платформа требует от каждого MQTT-клиента предъявить действительный
сертификат, подписанный этим CA. Полезно для deployments с высокими требованиями безопасности.

```bash
export MQTT_TLS_CERT=./certs/mqtt/cert.pem
export MQTT_TLS_KEY=./certs/mqtt/key.pem
export MQTT_TLS_CA=./certs/mqtt/ca.pem    # включает взаимный TLS
```

Устройства должны быть обеспечены `client.pem` + `client-key.pem`, встроенными в прошивку.

## Сводка портов

| Порт | Протокол | Шифрование | По умолчанию |
|------|----------|-----------|-------------|
| 1883 | MQTT TCP | нет | всегда включён |
| 8083 | MQTT WebSocket | нет | включён по умолчанию |
| 8883 | MQTT TCP + TLS | TLS 1.2+ | включён при настройке cert+key |

Обычный MQTT (`:1883`) остаётся доступным вместе с TLS. Устройства без поддержки TLS
могут продолжать подключаться на обычный порт.
