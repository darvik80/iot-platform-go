# MQTT over TLS

MQTT TLS (mqtts) encrypts traffic between devices / clients and the embedded MQTT broker.
The platform supports both **server-side TLS** (standard) and **mutual TLS** (client certificate verification).

## Quick start

```bash
# 1. Generate certificates
make gen-mqtt-tls

# 2. Set environment variables
export MQTT_TLS_CERT=./certs/mqtt/cert.pem
export MQTT_TLS_KEY=./certs/mqtt/key.pem

# 3. Start the platform
./bin/platform
```

The TLS listener starts on `MQTT_TLS_ADDR` (default `:8883`).

## Configuration

| Variable | YAML key | Default | Description |
|----------|----------|---------|-------------|
| `MQTT_TLS_ADDR` | `mqtt.tls_addr` | `:8883` | Listen address for MQTT-over-TLS |
| `MQTT_TLS_CERT` | `mqtt.tls_cert` | — | Path to server certificate (PEM) |
| `MQTT_TLS_KEY` | `mqtt.tls_key` | — | Path to server private key (PEM) |
| `MQTT_TLS_CA` | `mqtt.tls_ca` | — | Path to CA certificate — enables **mutual TLS** |

TLS is enabled when **both** `MQTT_TLS_CERT` and `MQTT_TLS_KEY` are set.
Without them, only plain MQTT (`:1883`) and WebSocket (`:8083`) are available.

**Path resolution**: certificate paths may be absolute (`/etc/certs/cert.pem`) or relative.
Relative paths are resolved in order:
1. Current working directory
2. Source root (detected via `runtime.Caller` — works in `go run` / IDE)
3. Executable directory (Docker/systemd with certs next to binary)

### config.yml example

```yaml
mqtt:
  addr: ":1883"
  ws_addr: ":8083"
  tls_addr: ":8883"
  tls_cert: "/etc/iot-platform-go/certs/cert.pem"
  tls_key: "/etc/iot-platform-go/certs/key.pem"
  # tls_ca: "/etc/iot-platform-go/certs/ca.pem"   # uncomment for mutual TLS
  broker_host_public: "192.168.1.22"
```

## Certificate generation

### Using the script (recommended)

```bash
make gen-mqtt-tls              # generates to ./certs/mqtt/
make gen-mqtt-tls DIR=certs    # custom output directory
```

This creates:
- `ca.pem` / `ca-key.pem` — self-signed CA (valid 10 years)
- `cert.pem` / `key.pem` — server certificate signed by the CA (valid 1 year)
- `client.pem` / `client-key.pem` — client certificate for mutual TLS (valid 1 year)

### Using openssl directly

```bash
# CA
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout ca-key.pem -out ca.pem -days 3650 -nodes \
  -subj "/CN=Cogitor IoT CA"

# Server cert
openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout key.pem -out server.csr -nodes \
  -subj "/CN=iot.local"
openssl x509 -req -in server.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out cert.pem -days 365
rm server.csr ca.srl

# Client cert (for mutual TLS)
openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
  -keyout client-key.pem -out client.csr -nodes \
  -subj "/CN=device-client"
openssl x509 -req -in client.csr -CA ca.pem -CAkey ca-key.pem \
  -CAcreateserial -out client.pem -days 365
rm client.csr ca.srl
```

## Mutual TLS

When `MQTT_TLS_CA` is set, the platform requires every MQTT client to present a valid
certificate signed by that CA. This is useful for high-security deployments where
token-based auth alone is not sufficient.

```bash
export MQTT_TLS_CERT=./certs/mqtt/cert.pem
export MQTT_TLS_KEY=./certs/mqtt/key.pem
export MQTT_TLS_CA=./certs/mqtt/ca.pem    # enables mutual TLS
```

Devices must be provisioned with `client.pem` + `client-key.pem` embedded in firmware.

## Device provisioning

When TLS is enabled, the device credentials API (`GET /api/devices/{id}/credentials`)
automatically returns:

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

### ESP32 firmware

```cpp
#include <WiFiClientSecure.h>
#include <MQTTClient.h>

WiFiClientSecure net;
MQTTClient client(2048);

// Server CA verification (standard TLS)
net.setCACert(ca_cert_pem);

// For mutual TLS — also set client cert + key
// net.setCertificate(client_cert_pem);
// net.setPrivateKey(client_key_pem);

client.begin("192.168.1.22", 8883, net);
client.connect("product.device", "product.device", token);
```

The `ca_cert_pem` (and optionally `client_cert_pem`, `client_key_pem`) must be embedded
in the firmware as C strings or loaded from a partition.

## Docker deployment

```bash
# Place certs in deploy/certs/
cp cert.pem key.pem deploy/certs/

# Configure
cat >> deploy/.env <<EOF
MQTT_TLS_CERT=/etc/iot-platform-go/certs/cert.pem
MQTT_TLS_KEY=/etc/iot-platform-go/certs/key.pem
EOF

docker compose up -d --build
```

The `deploy/docker-compose.yml` mounts `./certs` read-only into the container at
`/etc/iot-platform-go/certs/`.

## Ports summary

| Port | Protocol | Encryption | Default |
|------|----------|------------|---------|
| 1883 | MQTT TCP | none | always on |
| 8083 | MQTT WebSocket | none | on by default |
| 8883 | MQTT TCP + TLS | TLS 1.2+ | on when cert+key configured |

Plain MQTT (`:1883`) remains available alongside TLS. Devices that do not support TLS
can continue connecting on the plain port.
