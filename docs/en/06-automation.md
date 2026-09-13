# Automation (Node-RED)

Node-RED is a separate process (compose / Synology). The platform provides a rules bus for MQTT-based automation and integrations with external systems like Node-RED.

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

With multi-account tenancy, all topics are scoped to the account. The rules bus topics follow this pattern:

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

Commands:

```
platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc
platform/v1/commands/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{account_slug}/{product_id}/{device_id}/ota
```

This ensures that each account's events and commands are isolated. External plugins (like Node-RED) can only access topics for the account they are registered under.

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

```
platform/v1/commands/{product_id}/{device_id}/rpc
platform/v1/commands/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{product_id}/{device_id}/ota
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

Create an **external plugin** (Dashboard → Settings → External Plugins) named e.g. `nodered`, then use its generated `MQTT User` (`extplg_…`) and `MQTT Password` in Node-RED MQTT nodes. Such a client may subscribe to `platform/v1/events/#` and publish to `platform/v1/commands/#`.

**Never** put device access tokens into Node-RED flows.

### Creating a plugin in the dashboard

1. Log in to the dashboard as **admin**.
2. **Settings** → **External Plugins** block (bottom of the page).
3. Fill in:
   - `Plugin ID` — short name, e.g. `nodered`
   - `Name` — display name, e.g. `Node-RED`
   - `Description` — optional
4. Click **Create plugin**.

A modal "Plugin created — save credentials!" will appear with one-time credentials:

| Field | Example | Purpose |
|-------|---------|---------|
| Plugin ID | `nodered` | plugin identifier |
| API Token | `…` | for REST (not needed for MQTT) |
| MQTT User | `extplg_nodered_a1b2c3…` | username for MQTT |
| MQTT Password | `…` | password (shown **once**) |
| MQTT Broker | `192.168.1.22:1883` | platform broker address |

Click **Copy all** and save the credentials — the password won't be shown again. If lost — **Regenerate MQTT** in the plugins table will issue new ones.

### Configuring the MQTT node in Node-RED

1. Double-click **MQTT Connection** → **Edit connection node**.
2. Tabs `Server` / `Security`:
   - **Server**: IP or hostname of the platform (in docker-compose — `platform`), **Port**: `1883`
   - TLS (optional): **Port**: `8883`, enable *Enable TLS* — see [05-mqtt-tls.md](05-mqtt-tls.md)
3. Tab **Security**:
   - **User ID**: `MQTT User` from the creation step (`extplg_…`)
   - **Password**: `MQTT Password` from the creation step
4. **Done** → **Deploy**.

### Allowed topics (ACL)

The plugin client can:

- **subscribe** to platform events: `platform/v1/events/#`
- **publish** commands to devices: `platform/v1/commands/#`

| Direction | Topic |
|-----------|-------|
| events (read) | `platform/v1/events/{product}/{device}/telemetry` |
| events (read) | `platform/v1/events/{product}/{device}/status` |
| commands (write) | `platform/v1/commands/{product}/{device}/rpc` |
| commands (write) | `platform/v1/commands/{product}/{device}/attributes/desired` |
| commands (write) | `platform/v1/commands/{product}/{device}/ota` |

The plugin has **no access** to device topics (`iot/v1/…`, `v1/…`) — only the rules bus.

### Verification

1. **Deploy** in Node-RED — the MQTT node should show a green `connect` status.
2. Dashboard → **MQTT**: the *Connected clients* table should show a row with kind = `plugin` and username `extplg_…`.

### Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `not authorized` in Node-RED log | wrong `MQTT User` / `MQTT Password` | re-copy credentials; if lost — **Regenerate MQTT** |
| `auth_fail … external plugin bad password` | password doesn't match the hash | **Regenerate MQTT** and update the node |
| `acl_deny` on publish/subscribe | topic outside the rules bus | use only `platform/v1/events/#` and `platform/v1/commands/#` |
| `:1883` won't connect, port busy | another broker (Mosquitto/RabbitMQ/WinNAT) | stop it or change `MQTT_ADDR` |

## Example flow

`deploy/nodered/flows.json` ships a temperature-threshold → desired-attribute example and an offline status debug node. In compose the MQTT broker host is `platform`.

To access Node-RED, open it directly in your browser using the `NODERED_URL` configured in your platform settings. The URL must be reachable **from the browser**, not the Docker hostname `nodered`.

## Docker-compose

### Volume structure

- `nodered_data:/data` — volume for flows, credentials, node_modules
- `./nodered/settings.js:/data/settings.js:ro` — Node-RED settings (read-only)
- `./nodered/flows.json:/templates/flows.json:ro` — flows template (copied on first start)
- `./nodered/entrypoint.sh:/entrypoint-custom.sh:ro` — entrypoint script

On first start the entrypoint script copies `flows.json` from `/templates/` to `/data/` (if not already there). After that Node-RED works with files in the `nodered_data` volume.

### Updating flows.json

1. Edit `deploy/nodered/flows.json` on the host
2. Remove the volume: `docker compose down -v`
3. Restart: `docker compose up -d`

Or update flows through the Node-RED web interface (Deploy) — they will be saved in the volume.

### `NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD` variables

No longer read by the platform. Node-RED connects as an external plugin via dashboard credentials.
