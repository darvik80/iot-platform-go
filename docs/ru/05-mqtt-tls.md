# MQTT over TLS

По умолчанию MQTT-трафик между устройствами и платформой идёт **без шифрования** (порт 1883). Для production-развёртываний, особенно через публичные сети, необходимо включить TLS.

Платформа поддерживает два режима:
- **Серверный TLS** (стандартный) — шифрование трафика, устройство проверяет сертификат сервера
- **Взаимный TLS** (mTLS) — дополнительно сервер проверяет сертификат каждого устройства

---

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

Обычный MQTT (`:1883`) остаётся работать — устройства без TLS продолжают подключаться как раньше.

---

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

---

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

---

## Подключение устройств

Когда TLS включён, API учётных данных устройства (`GET /api/devices/{id}/credentials`) автоматически возвращает TLS-порт:

```json
{
  "broker_port": 8883,
  "provision": {
    "cogitor": {
      "host": "192.168.1.22",
      "port": 8883,
      "tls": true
    }
  }
}
```

### ESP32 (стандартный TLS)

```cpp
#include <WiFiClientSecure.h>
#include <MQTTClient.h>

WiFiClientSecure net;
MQTTClient client(2048);

// Проверка серверного сертификата
net.setCACert(ca_cert_pem);

client.begin("192.168.1.22", 8883, net);
client.connect("product.device", "product.device", token);
```

### ESP32 (взаимный TLS)

```cpp
// Дополнительно — клиентский сертификат + ключ
net.setCACert(ca_cert_pem);
net.setCertificate(client_cert_pem);
net.setPrivateKey(client_key_pem);

client.begin("192.168.1.22", 8883, net);
client.connect("product.device", "product.device", token);
```

`ca_cert_pem` (и `client_cert_pem`, `client_key_pem` для mTLS) должны быть встроены в прошивку как C-строки или загружены из partition.

---

## Взаимный TLS (mTLS)

Когда `MQTT_TLS_CA` установлен, платформа требует от каждого MQTT-клиента предъявить действительный сертификат, подписанный этим CA. Полезно для deployments, где токенов недостаточно.

```bash
export MQTT_TLS_CERT=./certs/mqtt/cert.pem
export MQTT_TLS_KEY=./certs/mqtt/key.pem
export MQTT_TLS_CA=./certs/mqtt/ca.pem    # включает взаимный TLS
```

Устройства должны быть обеспечены `client.pem` + `client-key.pem`, встроенными в прошивку.

---

## Docker-развёртывание

```bash
# Положить сертификаты в deploy/certs/
cp cert.pem key.pem deploy/certs/

# Добавить в deploy/.env
cat >> deploy/.env <<EOF
MQTT_TLS_CERT=/etc/iot-platform-go/certs/cert.pem
MQTT_TLS_KEY=/etc/iot-platform-go/certs/key.pem
EOF

docker compose up -d --build
```

`deploy/docker-compose.yml` монтирует `./certs` read-only в контейнер: `/etc/iot-platform-go/certs/`.

Не забудьте открыть порт 8883 наружу, если устройства подключаются через TLS.

---

## Сводка портов

| Порт | Протокол | Шифрование | Когда включён |
|------|----------|-----------|---------------|
| 1883 | MQTT TCP | нет | Всегда |
| 8083 | MQTT WebSocket | нет | По умолчанию |
| 8883 | MQTT TCP + TLS | TLS 1.2+ | При настройке cert+key |

Обычный MQTT (`:1883`) остаётся доступным вместе с TLS. Устройства без поддержки TLS могут продолжать подключаться на обычный порт.

См. также: [04-mqtt.md](04-mqtt.md) — MQTT-протокол, [12-deploy-vds.md](12-deploy-vds.md) — TLS через nginx.
