# Cogitor IoT — deploy stack

See root [README](../README.md) for full platform docs.

```bash
cp .env.example .env                    # host ports, host folders, image
cp platform.env.example platform.env    # optional: pinned platform settings
docker compose up -d --build            # platform
docker compose --profile nodered up -d  # + Node-RED
```

`docker-compose.yml` services:

- **`platform`** — built from the root `Dockerfile` (or `IOT_IMAGE`, e.g. `darvik80/iot-platform:latest`).
  Host ports `IOT_HTTP_PORT` / `IOT_MQTT_PORT` / `IOT_MQTT_WS_PORT` / `IOT_MQTT_TLS_PORT` → container `8080` / `1883` / `8083` / `8883`;
  the MQTT host ports are also announced to devices (`MQTT_*_PUBLIC_PORT`).
  Host folders `IOT_DATA_DIR` → `/opt/iot-platform/lib` (SQLite + firmware) and `IOT_ETC_DIR` → `/opt/iot-platform/etc`
  (`config.yml` written by the setup wizard + certs). First start: open `http://<host>:<IOT_HTTP_PORT>/setup`.
- **`nodered`** (profile `nodered`) — `nodered/node-red:4.0` on `NODERED_PORT` (default `1880`) with bind mounts
  `./nodered/data:/data`, `./nodered/settings.js`, `./nodered/flows.json` (template copied to `/data` on first start)
  and `./nodered/entrypoint.sh`. Reads `NODERED_CREDENTIAL_SECRET`, `MQTT_BROKER_HOST` and `ACCOUNT_SLUG` from `.env`.

Other ways to run the platform:

- bare-metal / systemd — `iot-platform.service` (optional `EnvironmentFile=/opt/iot-platform/etc/.env`), see [12-deploy-vds](../docs/en/12-deploy-vds.md);
- as a deb package — `make deb` (`deb/`);
- `docker run` or Gitea CI — see [13-deploy-docker](../docs/en/13-deploy-docker.md).

Node-RED has no dedicated MQTT credentials (`NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD` are gone).
Register it as an external plugin in the dashboard (**Settings → External Plugins**), then put the issued
`extplg_{account_slug}_{plugin_id}` user and one-time password into the flow's MQTT broker node, and subscribe to
`platform/v1/events/{account_slug}/…` — see [06-automation](../docs/en/06-automation.md).

Set `PUBLIC_BASE_URL` (in the setup wizard or `platform.env`) to a host ESP devices can reach (LAN IP) so hosted OTA downloads work.
Set `TIMEZONE` (IANA, e.g. `Asia/Bangkok`) for schedule / quiet hours.
