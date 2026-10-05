# MQTT over TLS

By default, MQTT traffic between devices and the platform is **unencrypted** (port 1883). For production deployments — especially over public networks — TLS should be enabled.

The platform supports two modes:
- **Server-side TLS** (standard) — encrypts traffic, device verifies the server certificate
- **Mutual TLS** (mTLS) — server additionally verifies each device's certificate (TLS listener `:8883` only)

---

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

Plain MQTT (`:1883`) remains available — devices without TLS continue connecting as before.

---

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
  tls_cert: "/opt/iot-platform/etc/certs/mqtt/cert.pem"
  tls_key: "/opt/iot-platform/etc/certs/mqtt/key.pem"
  # tls_ca: "/opt/iot-platform/etc/certs/mqtt/ca.pem"   # uncomment for mutual TLS
  broker_host_public: "192.168.1.22"
```

---

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

### Using the setup wizard

On first start the setup wizard can generate certificates via `POST /api/v1/setup/generate-tls` (`{ "host"?, "dir"? }`). Default directory: `/opt/iot-platform/etc/certs/mqtt`. It creates only `ca.pem`, `cert.pem` (server, CN/SAN = `host`) and `key.pem`; it does **not** create a client certificate — use `make gen-mqtt-tls` or openssl for mTLS.

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

---

## Device provisioning

Device credentials are returned only by `POST /api/v1/devices` (create) and
`POST /api/v1/devices/{id}/token/rotate`. When TLS is enabled, they automatically contain the TLS port:

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

### CA certificate download

`GET /api/v1/mqtt/ca.pem` (public, no auth) returns the file configured in `MQTT_TLS_CA`.
If `MQTT_TLS_CA` is not set, it returns `404`; in that case distribute `ca.pem` to devices yourself.

### ESP32 (standard TLS)

```cpp
#include <WiFiClientSecure.h>
#include <MQTTClient.h>

WiFiClientSecure net;
MQTTClient client(2048);

// Server CA verification
net.setCACert(ca_cert_pem);

client.begin("192.168.1.22", 8883, net);
client.connect("product.device", "product.device", token);
```

### ESP32 (mutual TLS)

```cpp
// Additionally — client cert + key
net.setCACert(ca_cert_pem);
net.setCertificate(client_cert_pem);
net.setPrivateKey(client_key_pem);

client.begin("192.168.1.22", 8883, net);
client.connect("product.device", "product.device", token);
```

The `ca_cert_pem` (and `client_cert_pem`, `client_key_pem` for mTLS) must be embedded
in the firmware as C strings or loaded from a partition.

---

## Mutual TLS

When `MQTT_TLS_CA` is set, the platform requires every MQTT client connecting to the TLS listener
(`:8883`) to present a valid certificate signed by that CA. Plain MQTT (`:1883`) and WebSocket (`:8083`)
do not check client certificates. Useful for deployments where token-based auth alone is not sufficient.

```bash
export MQTT_TLS_CERT=./certs/mqtt/cert.pem
export MQTT_TLS_KEY=./certs/mqtt/key.pem
export MQTT_TLS_CA=./certs/mqtt/ca.pem    # enables mutual TLS
```

Devices must be provisioned with `client.pem` + `client-key.pem` embedded in firmware.

---

## Docker deployment

`deploy/docker-compose.yml` contains only Node-RED; the platform container is built from the root
`Dockerfile` and run separately. Mount the certificates and pass the paths via environment:

```bash
docker build -t cogitor-iot-platform .

docker run -d --name iot-platform \
  -p 8080:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v iot-platform-data:/data \
  -v "$PWD/certs/mqtt:/opt/iot-platform/etc/certs/mqtt:ro" \
  -e MQTT_TLS_CERT=/opt/iot-platform/etc/certs/mqtt/cert.pem \
  -e MQTT_TLS_KEY=/opt/iot-platform/etc/certs/mqtt/key.pem \
  cogitor-iot-platform
```

The process runs as user `platform` (uid 1000), so the mounted `key.pem` must be readable by that uid.

Bare-metal / systemd: put the files in `/opt/iot-platform/etc/certs/mqtt/` and set `mqtt.tls_cert` / `mqtt.tls_key`
in `config.yml` (or the `MQTT_TLS_*` environment variables).

Don't forget to expose port 8883 if devices connect over TLS.

---

## Ports summary

| Port | Protocol | Encryption | When enabled |
|------|----------|------------|--------------|
| 1883 | MQTT TCP | none | Always |
| 8083 | MQTT WebSocket | none | On by default |
| 8883 | MQTT TCP + TLS | TLS 1.2+ | When cert+key configured |

Plain MQTT (`:1883`) remains available alongside TLS. Devices that do not support TLS
can continue connecting on the plain port.

See also: [04-mqtt.md](04-mqtt.md) — MQTT protocol, [12-deploy-vds.md](12-deploy-vds.md) — TLS via nginx.
