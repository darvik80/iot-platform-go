# Synology / Gitea deploy notes

CI workflows live in `.gitea/workflows/` (same pattern as note-keeper).

## Deploy (`deploy.yml`)

On push to `master`/`main`, the Gitea runner on the NAS:

1. Builds `localhost/crearts-iot-platform:latest`
2. Recreates container on port **9082**
3. Mounts:
   - `/volume1/docker/crearts-iot-platform/data` → `/data` (SQLite)
   - env from `/volume1/docker/crearts-iot-platform/etc/.env`

## One-time NAS setup

```bash
mkdir -p /volume1/docker/crearts-iot-platform/{data,etc}
cp .env.example /volume1/docker/crearts-iot-platform/etc/.env
# edit .env — set MQTT/RabbitMQ to NAS LAN IP, PUBLIC_BASE_URL, JWT_SECRET, etc.
```

Example `.env` on NAS (RabbitMQ already on the same host):

```env
MQTT_BROKER=tcp://192.168.1.100:1883
RABBITMQ_MGMT_URL=http://192.168.1.100:15672
BROKER_HOST_PUBLIC=192.168.1.100
PUBLIC_BASE_URL=http://192.168.1.100:9082
JWT_SECRET=change-me
ADMIN_LOGIN=admin
ADMIN_PASSWORD=...
```

Dashboard: `http://<nas>:9082`

## Build check (`build.yml`)

Runs `go build` + `go vet` on push/PR.
