# Развёртывание Docker (Synology / compose)

Контейнерный запуск: Gitea runner на Synology или `deploy/docker-compose.yml`.
Production на Cogitor VPS использует systemd — см. [12-deploy-vds.md](12-deploy-vds.md).

## Gitea workflow

`.gitea/workflows/deploy-docker.yml` — **workflow_dispatch** (запуск вручную на NAS runner):

1. Собрать `localhost/iot-platform-go:latest`
2. Пересоздать контейнер на HTTP **9083** и MQTT **1883**
3. Монтирование:
   - `/volume1/docker/iot-platform-go/data` → `/data`
   - `/volume1/docker/iot-platform-go/etc` → `/etc/iot-platform-go`

Остановить другой MQTT-брокер на `:1883` или сменить `TARGET_MQTT_PORT` / `MQTT_ADDR`.

## Первоначальная настройка NAS

```bash
mkdir -p /volume1/docker/iot-platform-go/{data,etc}
cp .env.example /volume1/docker/iot-platform-go/etc/.env
# BROKER_HOST_PUBLIC, PUBLIC_BASE_URL, JWT_SECRET, …
```

Дашборд: `http://<nas>:9083` · MQTT: `<nas>:1883`

## Локальный compose

```bash
cd deploy
cp .env.example .env
docker compose up -d --build
```
