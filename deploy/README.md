# Cogitor IoT — deploy files

See root [README](../README.md) for full platform docs.

- `nodered/` — Node-RED assets for the `nodered` service of the root [`docker-compose.yml`](../docker-compose.yml):
  `settings.js`, `flows.json` (template copied to `nodered/data/` on first start), `entrypoint.sh`.
- `iot-platform.service`, `remote-deploy.sh` — bare-metal / systemd deploy on a VPS, see [12-deploy-vds](../docs/en/12-deploy-vds.md).

Docker Compose runs from the repository root:

```bash
cp .env.example .env                    # host ports, host folders, image
cp platform.env.example platform.env    # optional: pinned platform settings
docker compose up -d                    # platform (Docker Hub image; --build for a local build)
docker compose --profile nodered up -d  # + Node-RED
```

Details (ports, folders, Gitea CI, Docker Hub) — [13-deploy-docker](../docs/en/13-deploy-docker.md).

Node-RED has no dedicated MQTT credentials (`NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD` are gone).
Register it as an external plugin in the dashboard (**Settings → External Plugins**), then put the issued
`extplg_{account_slug}_{plugin_id}` user and one-time password into the flow's MQTT broker node, and subscribe to
`platform/v1/events/{account_slug}/…` — see [06-automation](../docs/en/06-automation.md).

Set `PUBLIC_BASE_URL` (in the setup wizard or `platform.env`) to a host ESP devices can reach (LAN IP) so hosted OTA downloads work.
Set `TIMEZONE` (IANA, e.g. `Asia/Bangkok`) for schedule / quiet hours.
