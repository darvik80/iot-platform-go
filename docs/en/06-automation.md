# Automation (Node-RED)

Node-RED is a separate process (`deploy/docker-compose.yml` / Synology). The platform provides a rules bus for MQTT-based automation and integrations with external systems like Node-RED. Node-RED has no dedicated MQTT credentials: it connects to the embedded broker as an **external plugin** (see [Node-RED MQTT credentials](#node-red-mqtt-credentials)).

## Integration guide

The dashboard **Settings** page includes an **Automation & Integrations** section (admin-only) that provides:

- Brief description of the rules bus
- MQTT topic templates for your account
- Quick copy button for topic patterns
- Link to this documentation

To access the MQTT topic template for your account:

1. Navigate to **Settings** in the dashboard
2. Scroll to **Automation & Integrations** section
3. Click **Copy MQTT topic template** to get the topic pattern for your account

The topic template uses your account slug: `platform/v1/events/{account_slug}/...`

## Multi-account topics

With multi-account tenancy, rules bus events are scoped to the account. Event topics follow this pattern (`{kind}` is one of `status`, `telemetry`, `event`, `attributes/reported`, `attributes/desired`, `rpc/result`, `ota`, `log`):

```
platform/v1/events/{account_slug}/{product_id}/{device_id}/status
platform/v1/events/{account_slug}/{product_id}/{device_id}/telemetry
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/reported
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/events/{account_slug}/{product_id}/{device_id}/event
platform/v1/events/{account_slug}/{product_id}/{device_id}/rpc/result
platform/v1/events/{account_slug}/{product_id}/{device_id}/ota
platform/v1/events/{account_slug}/{product_id}/{device_id}/log
```

Commands (contract):

```
platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc
platform/v1/commands/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{account_slug}/{product_id}/{device_id}/ota
```

The broker ACL lets an external plugin (like Node-RED) **subscribe** only to its own account's events: `platform/v1/events/{account_slug}/#`. A bare `platform/v1/events/#` (or any filter outside the account's subtree) is rejected with `acl_deny`.

For **publishing**, the ACL allows only `platform/v1/commands/{account_slug}/#` of the plugin's own account, and the platform resolves `{product_id}/{device_id}` inside that account. Commands for other accounts are rejected with `acl_deny`; the legacy slug-less form `platform/v1/commands/{product_id}/{device_id}/…` is no longer accepted.

## Rules bus

### Events (platform → Node-RED)

Example telemetry envelope:

```json
{
  "ts": 1451649600512,
  "product_id": "esp32-cam",
  "device_id": "cam-001",
  "keys": { "temperature": 22.5, "humidity": 61 }
}
```

### Commands (Node-RED → platform)

Account-scoped topics (the slug must be the plugin's own account):

```
platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc
platform/v1/commands/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{account_slug}/{product_id}/{device_id}/ota
```

RPC command (platform allocates device-facing `id`):

```json
{
  "method": "reboot",
  "params": { "delayMs": 1000 },
  "timeout_ms": 30000,
  "correlation_id": "nr-flow-1"
}
```

Alternatively call REST `POST /api/v1/devices/{id}/rpc` with a Bearer token.

For **per-device** behavior (LED garland, schedule, etc.) use [device plugins](07-internal-plugins.md) instead of a Node-RED flow.

## Node-RED MQTT credentials

Connect Node-RED to the **platform** MQTT port (`MQTT_ADDR`, default `:1883`), not a separate broker.

Node-RED has no dedicated MQTT user: the old `nodered_mqtt_user` / `nodered_mqtt_pass` settings were removed (migration `010_drop_nodered_creds.sql`). Instead, register an **external plugin** (Dashboard → Settings → External Plugins, or `POST /api/v1/plugins/external`) named e.g. `nodered`, then use its generated `MQTT User` (`extplg_{account_slug}_{plugin_id}`) and `MQTT Password` in Node-RED MQTT nodes. Such a client may subscribe to `platform/v1/events/{account_slug}/#` and publish to `platform/v1/commands/...`.

**Never** put device access tokens into Node-RED flows.

### Creating a plugin in the dashboard

1. Log in to the dashboard as **admin**.
2. **Settings** → **External Plugins** block (bottom of the page).
3. Fill in:
   - `Plugin ID` — short name, e.g. `nodered`
   - `Name` — display name, e.g. `Node-RED`
   - `Description` — optional
4. Click **Create plugin**.

A modal "Plugin created — save credentials!" will appear with one-time credentials (no API token is issued):

| Field | Example | Purpose |
|-------|---------|---------|
| Plugin ID | `nodered` | plugin identifier |
| MQTT User | `extplg_acme_nodered` | username for MQTT (`extplg_{account_slug}_{plugin_id}`) |
| MQTT Password | `ext_plg_…` | password (shown **once**) |
| MQTT Broker | `192.168.1.22:1883` | platform broker address |
| Rules Bus | `platform/v1` | topic prefix; the modal also shows the subscribe/publish templates for your account (the publish template includes the slug — in v1.0.x drop it, see the warning above) |

Click **Copy all** and save the credentials — the password won't be shown again. If lost — **Regenerate MQTT** in the plugins table (`POST /api/v1/plugins/external/{id}/regenerate-mqtt`) issues a new password; the username does not change.

### Configuring the MQTT node in Node-RED

1. Double-click **MQTT Connection** → **Edit connection node**.
2. Tabs `Server` / `Security`:
   - **Server**: IP or hostname of the platform host (with `deploy/docker-compose.yml` the platform runs on the host, so use `host.docker.internal` / the LAN IP — the shipped flow uses `MQTT_BROKER_HOST`), **Port**: `1883`
   - TLS (optional): **Port**: `8883`, enable *Enable TLS* — see [05-mqtt-tls.md](05-mqtt-tls.md)
3. Tab **Security**:
   - **User ID**: `MQTT User` from the creation step (`extplg_…`)
   - **Password**: `MQTT Password` from the creation step
4. **Done** → **Deploy**.

### Allowed topics (ACL)

The plugin client can:

- **subscribe** to its own account's events: `platform/v1/events/{account_slug}/#` (the slug is the middle part of the MQTT user `extplg_{account_slug}_{plugin_id}`)
- **publish** commands to its own account's devices: `platform/v1/commands/{account_slug}/…`

| Direction | Topic |
|-----------|-------|
| events (read) | `platform/v1/events/{account_slug}/{product}/{device}/telemetry` |
| events (read) | `platform/v1/events/{account_slug}/{product}/{device}/status` |
| events (read) | `platform/v1/events/{account_slug}/+/+/{kind}` or `platform/v1/events/{account_slug}/#` |
| commands (write) | `platform/v1/commands/{account_slug}/{product}/{device}/rpc` |
| commands (write) | `platform/v1/commands/{account_slug}/{product}/{device}/attributes/desired` |
| commands (write) | `platform/v1/commands/{account_slug}/{product}/{device}/ota` |

Denied: `platform/v1/events/#`, `#`, `+`, other accounts' `platform/v1/events/{other_slug}/…` and `platform/v1/commands/{other_slug}/…`, slug-less `platform/v1/commands/{product}/{device}/…`. The plugin has **no access** to device topics (`iot/v1/…`, `v1/…`) — only the rules bus.

### Verification

1. **Deploy** in Node-RED — the MQTT node should show a green `connect` status.
2. Dashboard → **MQTT**: the *Connected clients* table should show a row with kind = `plugin` and username `extplg_{account_slug}_{plugin_id}`; the plugin status in Settings → External Plugins becomes `online`.

### Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `not authorized` in Node-RED log | wrong `MQTT User` / `MQTT Password` | re-copy credentials; if lost — **Regenerate MQTT** |
| `auth_fail … external plugin bad password` | password doesn't match the hash | **Regenerate MQTT** and update the node |
| `acl_deny` on subscribe | filter is `platform/v1/events/#`, `#`, `+` or another account's slug | subscribe to `platform/v1/events/{account_slug}/#` (or narrower) |
| `acl_deny` on publish | topic outside `platform/v1/commands/{account_slug}/` (no slug or another account's slug) | publish under `platform/v1/commands/{account_slug}/{product}/{device}/…` |
| `command … unknown device` in MQTT log | no such `{product}/{device}` in the plugin's account | check product/device IDs in the dashboard of that account |
| `:1883` won't connect, port busy | another broker (Mosquitto/RabbitMQ/WinNAT) | stop it or change `MQTT_ADDR` |

## Example flow

`deploy/nodered/flows.json` ships a minimal example: an `mqtt in` node on device status events wired to a debug node, plus an `mqtt-broker` config node. On first start `entrypoint.sh` replaces `__MQTT_BROKER__` with `MQTT_BROKER_HOST` (default `host.docker.internal`, i.e. the platform running on the Docker host) and `__ACCOUNT_SLUG__` with `ACCOUNT_SLUG` (default `default`; must match the plugin's MQTT user `extplg_{account_slug}_{id}`).

After import, open the broker node and set the external-plugin `MQTT User` / `MQTT Password`. The shipped `mqtt in` topic `platform/v1/events/+/+/status` does not include the account slug and is rejected by the ACL — change it to `platform/v1/events/{account_slug}/+/+/status`.

To access Node-RED, open it directly in your browser using the `NODERED_URL` configured in your platform settings. The URL must be reachable **from the browser**, not the Docker hostname `nodered`.

## Docker-compose

`deploy/docker-compose.yml` runs **only** the `nodered` service (`nodered/node-red:4.0`, port `1880`). The platform itself runs outside this compose file (bare-metal/systemd, deb package, or its own container built from the root `Dockerfile`).

### Volume structure

- `./nodered/data:/data` — bind mount for flows, credentials, node_modules
- `./nodered/settings.js:/data/settings.js:ro` — Node-RED settings (read-only)
- `./nodered/flows.json:/templates/flows.json:ro` — flows template (copied on first start)
- `./nodered/entrypoint.sh:/entrypoint-custom.sh:ro` — entrypoint script

Environment: `NODERED_CREDENTIAL_SECRET` (encrypts Node-RED credentials) and `MQTT_BROKER_HOST` (broker host substituted into the flow template).

On first start the entrypoint script copies `flows.json` from `/templates/` to `/data/` (if not already there). After that Node-RED works with files in `deploy/nodered/data/`.

### Updating flows.json

1. Edit `deploy/nodered/flows.json` on the host
2. Delete `deploy/nodered/data/flows.json` (a bind mount, so `docker compose down -v` does not remove it)
3. Restart: `docker compose up -d`

Or update flows through the Node-RED web interface (Deploy) — they will be saved in `deploy/nodered/data/`.

### `NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD` variables

Removed. The platform no longer reads them and the `nodered_mqtt_user` / `nodered_mqtt_pass` settings were dropped by migration `010_drop_nodered_creds.sql`. Node-RED connects as an external plugin via dashboard credentials.
