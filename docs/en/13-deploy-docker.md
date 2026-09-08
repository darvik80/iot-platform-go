# Deploy Docker (Synology / compose)

Containerized run: Synology Gitea runner or `deploy/docker-compose.yml`.  
Production on Cogitor VPS uses systemd instead — see [12-deploy-vds.md](12-deploy-vds.md).

## Gitea workflow

`.gitea/workflows/deploy-docker.yml` — **workflow_dispatch** (run manually on the NAS runner):

1. Build `localhost/iot-platform-go:latest`
2. Recreate container on HTTP **9083** and MQTT **1883**
3. Mount:
   - `/volume1/docker/iot-platform-go/data` → `/data`
   - `/volume1/docker/iot-platform-go/etc` → `/etc/iot-platform-go`

Stop any other MQTT broker on `:1883`, or change `TARGET_MQTT_PORT` / `MQTT_ADDR`.

## One-time NAS setup

```bash
mkdir -p /volume1/docker/iot-platform-go/{data,etc}
cp .env.example /volume1/docker/iot-platform-go/etc/.env
# BROKER_HOST_PUBLIC, PUBLIC_BASE_URL, JWT_SECRET, …
```

Dashboard: `http://<nas>:9083` · MQTT: `<nas>:1883`

## Local compose

```bash
cd deploy
cp .env.example .env
docker compose up -d --build
```
