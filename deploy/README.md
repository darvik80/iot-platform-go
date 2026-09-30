# Cogitor IoT — deploy stack

See root [README](../README.md) for full platform docs.

```bash
cp .env.example .env
docker compose up -d
```

`docker-compose.yml` runs **only Node-RED** (`nodered/node-red:4.0`, `:1880`) with bind mounts:
`./nodered/data:/data`, `./nodered/settings.js`, `./nodered/flows.json` (template copied to `/data` on first start)
and `./nodered/entrypoint.sh`. Compose reads `NODERED_CREDENTIAL_SECRET`, `MQTT_BROKER_HOST` and `ACCOUNT_SLUG` (the account of the Node-RED external plugin, default `default`) from `.env`.

The platform itself (API `:8080` + embedded MQTT `:1883`) is **not** part of this compose file. Run it:

- bare-metal / systemd — `iot-platform.service` (`EnvironmentFile=/opt/iot-platform-go/etc/.env`), see [12-deploy-vds](../docs/en/12-deploy-vds.md);
- as a deb package — `make deb` (`deb/`);
- in its own container built from the root `Dockerfile`, see [13-deploy-docker](../docs/en/13-deploy-docker.md).

Node-RED has no dedicated MQTT credentials (`NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD` are gone).
Register it as an external plugin in the dashboard (**Settings → External Plugins**), then put the issued
`extplg_{account_slug}_{plugin_id}` user and one-time password into the flow's MQTT broker node, and subscribe to
`platform/v1/events/{account_slug}/…` — see [06-automation](../docs/en/06-automation.md).

Set `PUBLIC_BASE_URL` to a host ESP devices can reach (LAN IP) so hosted OTA downloads work.  
Set `TIMEZONE` (IANA, e.g. `Asia/Bangkok`) for schedule / quiet hours.
