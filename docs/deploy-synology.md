# Synology / Gitea deploy notes

CI workflows live in `.gitea/workflows/` (same pattern as note-keeper).

## Deploy (`deploy.yml`)

On push to `master`/`main`, the Gitea runner on the NAS:

1. Builds `localhost/iot-platform-go:latest`
2. Recreates container on HTTP **9083** and MQTT **1883**
3. Mounts:
   - `/volume1/docker/iot-platform-go/data` → `/data` (SQLite)
   - env from `/volume1/docker/iot-platform-go/etc/.env`

Stop any other MQTT broker on `:1883` (e.g. RabbitMQ MQTT plugin) before deploy, or change `TARGET_MQTT_PORT` / `MQTT_ADDR`.

## One-time NAS setup

```bash
mkdir -p /volume1/docker/iot-platform-go/{data,etc}
cp .env.example /volume1/docker/iot-platform-go/etc/.env
# edit .env — set BROKER_HOST_PUBLIC to NAS LAN IP, PUBLIC_BASE_URL, JWT_SECRET, etc.
```

Example `.env` on NAS:

```env
MQTT_ADDR=:1883
BROKER_HOST_PUBLIC=192.168.1.100
PUBLIC_BASE_URL=http://192.168.1.100:9083
TIMEZONE=Asia/Bangkok
# ESP OTA: devices GET {PUBLIC_BASE_URL}/ota/fw/{token}.bin (no JWT)
JWT_SECRET=change-me
ADMIN_LOGIN=admin
ADMIN_PASSWORD=...
NODERED_MQTT_PASSWORD=...
```

Dashboard: `http://<nas>:9083`  
Devices / Node-RED MQTT: `<nas>:1883`

## Build check (`build.yml_`)

Runs `go build` + `go vet` on push/PR.
