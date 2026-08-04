# MQTT protocol (platform view)

Canonical device contract: **protocol v1** (see embed-framework `docs/iot-platform-mqtt-spec.md`).

## Topic styles

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
| `v1/o/upd` | down | `…/down/ota/update` |
| `v1/o/can` | down | `…/down/ota/cancel` |
| `v1/o/p` | up | `…/up/ota/progress` |
| `v1/l` | up | `…/up/logs/report` |

Correlation: JSON field `"id"` (uint32). RPC success: `"code": 0`.

## Presence

- **Online**: first accepted uplink (or future connection tracking).
- **Offline**: LWT on status topic (`online: false`, `reason: "lwt"`).

## Platform ingest

The service account binds an AMQP queue to:

- `iot.v1.*.*.up.#`
- `v1.#`

Short-topic messages are attributed using RabbitMQ delivery `user_id` (MQTT username = `{product}.{device}`).

## Device auth

1. Dashboard creates device → generates access token → SHA-256 stored.
2. RabbitMQ user `username={product}.{device}` / `password=token` with topic ACL for that device.
3. Rotate → updates password on the same RabbitMQ user; old hash revoked in DB.
4. Delete device → removes the RabbitMQ user.
