# MQTT protocol (platform view)

Canonical device contract: **protocol v1** (see embed-framework `docs/iot-platform-mqtt-spec.md`).  
Go names: [`internal/protocol`](../internal/protocol/topics.go) (`UpTelemetry` / `ShortTelemetry`, downlinks, rules bus).

The platform process **is** the MQTT broker ([mochi-mqtt](https://github.com/mochi-mqtt/server)). Devices and Node-RED connect to `MQTT_ADDR` (default `:1883`). Optional MQTT-over-WebSocket on `MQTT_WS_ADDR` (default `:8083`, URL `ws://{BROKER_HOST_PUBLIC}:8083/`, subprotocol `mqtt`). Set `MQTT_WS_ADDR=off` to disable. There is no external RabbitMQ.

## Topic styles

Style is **detected**, not configured. Platform infers it from Will / subscribe / publish prefix (session identity, same idea as ThingsBoard `/me/`):

| Prefix | Style |
|--------|--------|
| `iot/v1/{product}/{device}/…` | **full** |
| `v1/me/…` (alias `v1/dev/me/…`) | **short** |
| `v1/t`, `v1/r/req`, … | **short** |

Downlinks use the last detected style. Until the first MQTT packet, downlink defaults to short.

**Full**

```
iot/v1/{product_id}/{device_id}/{up|down}/{capability}/{operation}
```

**Short** (identity from session: `client_id=username={product}.{device}`, `password=access_token`)

| Short | Direction | Full |
|-------|-----------|------|
| `v1/s` | up | `…/up/status` (LWT only) |
| `v1/t` | up | `…/up/telemetry/data` |
| `v1/e` | up | `…/up/events/post` |
| `v1/a` | up | `…/up/attributes/report` |
| `v1/a/req` | up | `…/up/attributes/request` |
| `v1/a/res` | down | `…/down/attributes/response` |
| `v1/a/upd` | down | `…/down/attributes/update` |
| `v1/r/req` | down | `…/down/rpc/request` |
| `v1/r/res` | up | `…/up/rpc/response` |
| `v1/r/creq` | up | `…/up/rpc/request` |
| `v1/r/cres` | down | `…/down/rpc/response` |
| `v1/n/req` | up | `…/up/ntp/request` |
| `v1/n/res` | down | `…/down/ntp/response` |
| `v1/o/ver` | up | `…/up/ota/version` |
| `v1/o/q` | up | `…/up/ota/query` |
| `v1/o/upd` | down | `…/down/ota/update` (`url` is usually `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`) |
| `v1/o/can` | down | `…/down/ota/cancel` |
| `v1/o/p` | up | `…/up/ota/progress` |
| `v1/l` | up | `…/up/logs/report` |

Correlation: JSON field `"id"` (uint32). RPC success: `"code": 0`.

Built-in firmware RPC **`rpc-list`** (alias `rpc_list`) returns the method catalog. Each param is `{ type, required, default? }` (`int` / `bool` / `string` / `number`). The dashboard calls it when the device is online.

## Presence

- **Online**: MQTT session established (and first accepted uplink).
- **Offline**: disconnect, or LWT on status topic (`online: false`, `reason: "lwt"`).

## Platform ingest

PUBLISH from non-inline clients is ingested in-process:

- `iot/v1/{product}/{device}/up/#`
- `v1/#`
- `platform/v1/commands/#` (Node-RED → RPC / desired / OTA)

Short-topic messages are attributed using MQTT username (`{product}.{device}`). Platform republishes on the rules bus via the inline client and those publishes are **not** re-ingested.

## Device auth

1. Dashboard creates device → generates access token → SHA-256 stored.
2. MQTT CONNECT: `username={product}.{device}`, `password=token`. ACL is scoped to that device (`up`/`down` + `v1/#`).
3. Rotate → new hash in SQLite; live session for that username is dropped.
4. Disable / delete → session dropped; auth fails on reconnect.

## Monitoring

Dashboard **MQTT** page (`GET /api/v1/mqtt`): live sessions (client id, username, remote IP, subscriptions) and an in-memory event trail (auth, connect/disconnect, ACL deny, ingest, publish).

Stdout (`slog`, `LOG_LEVEL`, default Info): `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe`, `ingest` (non-telemetry), `command`, downlink `publish`. Telemetry ingest is Debug. Docker: `docker compose logs -f platform`.

WebSocket event `mqtt` is broadcast on connect / disconnect / auth_fail / subscribe / ACL deny / kick.

## Node-RED user

`NODERED_MQTT_USER` / `NODERED_MQTT_PASSWORD` (defaults `nodered` / `nodered`):

- subscribe `platform/v1/events/#`
- publish `platform/v1/commands/#`
