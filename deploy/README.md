# Crearts IoT — deploy stack

See root [README](../README.md) for full platform docs.

```bash
cp .env.example .env
docker compose up -d
```

Services: platform API + embedded MQTT (`:8080`, `:1883`), Node-RED (`:1880`).

Set `PUBLIC_BASE_URL` to a host ESP devices can reach (LAN IP) so hosted OTA downloads work.  
Set `TIMEZONE` (IANA, e.g. `Asia/Bangkok`) for schedule / quiet hours.
