# Device plugins

Per-device automations that live **in the platform process**. Not to be confused with Node-RED — that handles **cross-device** rules on the MQTT rules bus.

A plugin is **one device + one catalog type**. It receives device events (status, telemetry, ticks), calls RPC back, and has a UI panel on the device page.

Built-in plugins: `led-garland` (WS2812 strips), `schedule` (RPC by clock time) and `metrics-dashboard` (Metrics+ panels on the device page). Adding your own — one Go file + `init()`.

Prometheus metrics are **not** a catalog plugin: the platform binary serves them itself on `METRICS_ADDR` — see [Prometheus metrics](#prometheus-metrics).

Related: [External Plugins](08-external-plugins.md), [Automation](06-automation.md), [Product Schema](09-product-schema.md).

## Architecture

> **Diagram**: [Internal Plugin Architecture](14-diagrams.md#internal-plugin-architecture)

```mermaid
flowchart TB
  subgraph Ingest["MQTT Ingest"]
    Dev[Device MQTT] -->|status / telemetry / rpc result| Ing[ingest]
  end

  subgraph Core["Plugin Processing"]
    Ing --> PR[publishRules]
    PR --> Hub[PluginHub]
    PR -.->|optional| NR[Node-RED]
    Cat[Catalog<br/>Register init] --> Hub
    Hub --> R[runner per binding]
    R --> AC[ApplyConfig]
    AC --> OE[OnEvent]
    OE --> RPC[InvokeRPC]
  end

  subgraph Management["Dashboard"]
    UI[Dashboard Plugins] -->|Save / Apply now| API["REST /devices/id/plugins"]
    API --> SQ[(device_plugin_bindings)]
    API --> Hub
    SQ --> Hub
  end

  %% Core → Device
  RPC --> Dev

  style Ingest fill:#0f3460,stroke:#16213e,color:#eee
  style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
  style Management fill:#16213e,stroke:#1a1a2e,color:#eee
```

| Piece | Where | Role |
|-------|--------|------|
| **Catalog** | `internal/plugins` `Register()` in `init()` | Builtin types (`led-garland`, `schedule`, `metrics-dashboard`). `GET /plugins`. |
| **Policy** | `/plugins/policies` (admin) | Per-account enable/disable (and settings) of each catalog plugin. |
| **Binding** | DB table `device_plugin_bindings` (SQLite or PostgreSQL) | `UNIQUE(device_pk, plugin_id)`. Same catalog plugin on many devices = independent instances/configs. One device cannot bind the same `plugin_id` twice. |
| **Hub** | `services.PluginHub` | One runner per enabled binding. Started from `main` via `StartPlugins`. |
| **Instance** | `plugins.Instance` | `ApplyConfig` → `OnEvent` → `IntervalMS` / `Auto`. |
| **Runtime** | hub → `Platform.InvokeRPC` | Fire-and-forget RPC (no JWT). Same downlink as the dashboard console. |

Hub does **not** speak MQTT. Ingest already parsed the envelope; `publishRules` fans out to plugins **and** (if enabled) to Node-RED.

### Lifecycle

> **Diagram**: [Internal Plugin Lifecycle](14-diagrams.md#internal-plugin-lifecycle)

```mermaid
sequenceDiagram
  participant Dev as Device MQTT
  participant Ing as ingest
  participant Hub as PluginHub
  participant P as Instance
  participant RPC as InvokeRPC

  Note over Hub: StartPlugins / bind / save → ApplyConfig + syncRunner
  Dev->>Ing: CONNECT / status online
  Ing->>Hub: Dispatch(status, online)
  Hub->>Hub: syncRunner (ticker if Auto)
  Hub->>P: OnEvent(status, online=true)
  P->>RPC: led_attach / …
  loop while online and Auto()
    Hub->>P: OnEvent(tick)
    P->>RPC: set_led / schedule jobs
  end
  Dev->>Ing: disconnect
  Ing->>Hub: Dispatch(status, online=false)
  Hub->>Hub: stop ticker
  Hub->>P: OnEvent(status, online=false)

  Note over Hub: UI Save or Apply now
  Hub->>P: OnEvent(apply)
  P->>RPC: led_attach then paint / run jobs now
```

| `OnEvent` kind | When | Typical plugin action |
|----------------|------|------------------------|
| `status` | MQTT online/offline (after runner resync) | Attach hardware, reset state. `ev.Online` is authoritative. |
| `apply` | Binding save, bind, or `POST …/apply` | Re-apply config to the device (must be online). |
| `tick` | Hub ticker while **online** and `Auto()` | Animation frames, schedule poll. |
| `telemetry` / `event` / `rpc/result` | Same envelopes as the rules bus | Optional react. |

`Auto()` + `IntervalMS()` control the ticker. Offline always pauses it. Static LED colors do not tick unless a time window is enabled.

Plugins must not: open MQTT, use device tokens, replace Node-RED for multi-device flows, or block long inside `OnEvent`.

## Builtin catalog

| id | What |
|----|------|
| `led-garland` | WS2812-style strips via firmware `set_led`, optional quiet hours / night-light window |
| `schedule` | Fire device RPC at local clock times (reboot at 04:00, lights off at 23:00, …) |
| `metrics-dashboard` | Passive: holds the panel layout for the device **Metrics+** tab — see [Metrics dashboard](#metrics-dashboard) |

Each catalog plugin can be enabled or disabled per account by an admin: `GET /plugins/policies` lists the catalog with the effective policy (default: enabled), `PATCH /plugins/policies/{pluginId}` with `{ "enabled"?: bool, "settings"?: {} }` changes it and rebuilds the account's runners on the fly.

Binding is stored in the platform database (SQLite or PostgreSQL, table `device_plugin_bindings`). A ticker runs while the device is **online** and the plugin needs frames (`mode: auto`, animated effect, quiet/active windows, or `schedule`). Offline pauses it. Static colors (`off` / `white` / …) do not tick unless a time window is enabled.

Platform **`TIMEZONE`** (IANA, e.g. `Asia/Bangkok`) is the default for all time windows. Set it in `config.yml` / `.env`. A binding may override with `config.tz`. Empty `tz` → platform zone. Unknown zone → save is rejected.

## LED garland

Firmware binds WS2812 strips at runtime. The plugin therefore has a **required attach step**:

1. **Save / Apply now** (`apply`) or **device online** (`status`) → `led_attach` per strip `{ gpio, count, brightness }`
2. Then `set_led` frames `{ gpio, offset, length, r, g, b, on }`
3. Offline clears attach state; next online attaches again. Ticks do not re-attach unless attach was lost.

On the device page: **Add strip** / **Delete strip** (saved immediately). GPIO + LED count per strip.

Default config:

```json
{
  "strips": [
    { "gpio": 16, "length": 8 },
    { "gpio": 17, "length": 8 }
  ],
  "mode": "effect",
  "effect": "off",
  "interval_ms": 200,
  "auto_ms": 8000,
  "rpc": "set_led",
  "attach_rpc": "led_attach",
  "brightness": 32,
  "tz": "",
  "quiet": { "enabled": false, "from": "23:00", "to": "07:00", "effect": "off" },
  "active": { "enabled": false, "from": "18:00", "to": "23:00" }
}
```

Effects: `off`, `white`, `warm`, `red`, `green`, `blue`, `blink`, `strobe`, `breathe`, `chase`, `comet`, `scanner`, `fill`, `theater`, `sparkle`, `fire`, `rainbow`, `police`, `christmas`.

`interval_ms` is the frame period for animated effects **and** for `mode: auto`.  
`mode: auto` additionally switches the effect every `auto_ms`. Static solids do not use the ticker unless **quiet hours** or **only between** is on (then the hub polls ~15s to enter/leave the window).

- **Quiet hours** — inside `[from, to)` (wraps midnight if `from > to`) force `quiet.effect` (usually `off` / `warm`). Strobe/chase stop at night.
- **Only between** (night light) — outside the window the strip is off. Combine with quiet: outside active → off; inside active ∩ quiet → quiet effect; else normal.

RPC:

| Method | Params | When |
|--------|--------|------|
| `led_attach` | `gpio`, `count` (= strip length), `brightness` (1–255, default 32) | save / apply / device online |
| `set_led` | `gpio`, `offset`, `length`, `r`, `g`, `b`, `on` | after attach, each frame |

Same GPIO `led_attach` again recreates the strip (firmware).

## Schedule

Jobs run once per matching minute while the device is online (10s poll). `days` is `time.Weekday` (`0` Sunday … `6` Saturday); empty / all seven = every day.

```json
{
  "tz": "",
  "jobs": [
    { "name": "night off", "at": "23:00", "days": [], "rpc": "set_led", "params": { "gpio": 16, "offset": 0, "length": 8, "on": false } },
    { "name": "weekday reboot", "at": "04:00", "days": [1, 2, 3, 4, 5], "rpc": "reboot", "params": {} }
  ]
}
```

## Prometheus metrics

Metrics are served by the platform core, not by a plugin: `internal/metrics` keeps a private registry that is exposed on `METRICS_ADDR` (default `:9102`, deliberately not `:9100` so it cannot clash with node_exporter). The listener is always on, independent of catalog bindings and of `HTTP_ADDR`. Platform admins can read the same registry at `GET /api/v1/metrics`.

| Metric | Type | Labels |
|--------|------|--------|
| `platform_http_requests_total` | counter | `route`, `method`, `status` — `route` is the chi pattern (`/api/v1/devices/{id}`), which keeps cardinality bounded |
| `platform_http_request_duration_seconds` | histogram | `route`, `method` |
| `platform_http_in_flight_requests` | gauge | — |
| `platform_telemetry_samples_total` | counter | `account`, `product` |
| `platform_telemetry_errors_total` | counter | `account`, `product` |
| `platform_rpc_calls_total` | counter | `account`, `product`, `method`, `status` |
| `platform_event_bus_published_total` | counter | `account`, `kind` |
| `platform_devices` | gauge | `account`, `status` — written by the platform, read from the DB on scrape |
| `platform_ota_jobs` | gauge | `account`, `status` |
| `platform_external_plugins` | gauge | `account`, `status` |
| `platform_mqtt_clients` | gauge | `kind` |
| `platform_build_info` | gauge | `version`, `commit` |

Standard collectors are included: `go_*` and `process_*` (client_golang), plus DB pool stats as `go_sql_*{db_name="platform"}`.

Use `METRICS_ADDR=127.0.0.1:9102` when Prometheus scrapes from the same host; keep `:9102` when it scrapes over the network and firewall the port.

## Metrics dashboard

`metrics-dashboard` is passive: it runs no automation, sends no RPC and starts no server. Its config is the panel list that `GET /devices/{id}/dashboard` reads to build the **Metrics+** tab; without a binding the default panels are used.

Default config:

```json
{
  "panels": [
    { "id": "p1", "type": "telemetry_chart", "title": "WiFi RSSI", "keys": ["wifi_rssi"], "window": "1h" },
    { "id": "p2", "type": "telemetry_gauge", "title": "Free Heap", "key": "free_heap", "window": "1h" },
    { "id": "p3", "type": "online_timeline", "title": "Online Timeline", "window": "24h" }
  ]
}
```

Panel fields: `id`, `type`, `title`, `keys` (`telemetry_chart`), `key` (`telemetry_gauge`), `window` (`30m`, `1h`, `24h`, `7d`, …; default `1h`), `limit` (`rpc_table`, default 20), `severity` (`events_count`). Panel types: `telemetry_chart`, `telemetry_gauge`, `rpc_table`, `online_timeline`, `events_count`.

## API

| Method | Path |
|--------|------|
| GET | `/plugins` |
| GET | `/devices/{id}/plugins` |
| POST | `/devices/{id}/plugins` | `{ "plugin_id": "led-garland", "config"?: {}, "enabled"?: true }` |
| PUT | `/devices/{id}/plugins/{bindingId}` | `{ "enabled"?: bool, "config"?: {} }` |
| DELETE | `/devices/{id}/plugins/{bindingId}` |
| POST | `/devices/{id}/plugins/{bindingId}/apply` | one frame now (device must be online) |
| GET | `/plugins/policies` | catalog + per-account policy (admin) |
| PATCH | `/plugins/policies/{pluginId}` | `{ "enabled"?: bool, "settings"?: {} }` (admin) |

UI: device page → **Plugins**. The form is **`Descriptor.Form`** from the plugin (GET `/plugins` → `form`). Save / Apply / Remove stay in the shell. No `form` → JSON textarea.

## How to add a new plugin

V1 plugins are **Go builtins** (no zip/npm). One file + `init()` register. Rebuild the platform binary.

### 1. Implement `plugins.Instance`

Create `internal/plugins/<name>.go` in package `plugins`:

```go
package plugins

import "encoding/json"

const PluginPing = "ping"

func init() {
	Register(Descriptor{
		ID:            PluginPing,
		Name:          "Ping",
		Description:   "RPC ping while device is online",
		DefaultConfig: json.RawMessage(`{"interval_ms":5000,"rpc":"echo","msg":"ping"}`),
		Form: []FormField{
			{Key: "interval_ms", Type: "number", Label: "Interval ms", Min: num(200)},
			{Key: "rpc", Type: "string", Label: "RPC"},
			{Key: "msg", Type: "string", Label: "Message"},
		},
	}, func() Instance { return &Ping{} })
}

type Ping struct {
	interval int
	rpc, msg string
	auto     bool
}

func (p *Ping) ApplyConfig(raw json.RawMessage) error {
	var cfg struct {
		IntervalMS int    `json:"interval_ms"`
		RPC        string `json:"rpc"`
		Msg        string `json:"msg"`
		Mode       string `json:"mode"` // auto | off
	}
	cfg.IntervalMS, cfg.RPC, cfg.Msg = 5000, "echo", "ping"
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	p.interval, p.rpc, p.msg = cfg.IntervalMS, cfg.RPC, cfg.Msg
	p.auto = cfg.Mode == "auto"
	if p.interval < 200 {
		p.interval = 200
	}
	return nil
}

func (p *Ping) IntervalMS() int { return p.interval }
func (p *Ping) Auto() bool      { return p.auto }

func (p *Ping) OnEvent(rt Runtime, ev Event) error {
	switch ev.Kind {
	case "tick", "apply":
		return rt.InvokeRPC(p.rpc, map[string]any{"msg": p.msg})
	case "status", "telemetry", "event":
		// optional: react to device data
		return nil
	}
	return nil
}
```

Contract:

| Method | Role |
|--------|------|
| `ApplyConfig` | parse/validate binding JSON; called on bind/update |
| `OnEvent` | `tick` / `apply` / `status` / `telemetry` / `event` / `rpc/result`. Use `status`+`Online` for attach-on-connect. |
| `IntervalMS` | ticker period when `Auto()` |
| `Auto` | `true` → hub ticks while device is online |

`Runtime.InvokeRPC` is fire-and-forget (no JWT). Same path as dashboard RPC.

### 2. Register

`init()` + `Register(...)` is enough. Same package `plugins` is imported by the hub, so the new file is linked automatically. **Do not** forget `init()` or the plugin will not appear in `GET /plugins`.

Optional `Effects []string` feeds `options_from: "effects"` on select fields.

### 3. UI (do not touch webui)

Set `Descriptor.Form` (`[]FormField`). Types: `number` `string` `select` `checkbox` `time` `timezone` `json` `weekdays` `object` `array` `break`. `break` is layout-only (optional `label` + `<hr>`). Dashboard renders it; Save/Apply/Remove stay in the shell. Empty `Form` → JSON textarea.

```go
Form: []FormField{
  {Key: "interval_ms", Type: "number", Label: "Interval ms", Min: num(200)},
  {Key: "rpc", Type: "string", Label: "RPC"},
},
```

### 4. Run

```bash
go test ./internal/plugins/ ./internal/services/
# restart platform
```

Device page → **Add plugin** → pick the new name → edit config → Save. `enabled` + `Auto()` starts the ticker after the device is online. If the firmware needs a one-time bind (like `led_attach`), do it on `apply` **and** `status` online.

### 5. Tests

Prefer pure helpers (like `LEDFrame`) without MQTT. Binding CRUD: `internal/services/plugin_hub_test.go`.

### Do not

- Talk MQTT / device tokens from the plugin — only `InvokeRPC` + events.
- Replace Node-RED for multi-device rules.
- Block for long inside `OnEvent` (ticks run sequentially).

---

## External Plugins

External plugins run as **separate processes** (any language). The only implemented transport is **MQTT**: the plugin connects to the embedded broker with its `extplg_{account_slug}_{plugin_id}` credentials, subscribes to its account's rules bus events and publishes commands. Webhook, polling and gRPC transports are planned, not implemented. External plugins do not use device bindings and do not receive `tick` / `apply` events.

See [08-external-plugins.md](08-external-plugins.md) for the specification (implemented vs planned) and [examples/plugins/](../../examples/plugins/) for MQTT examples in Python, Go, Java and C++.
