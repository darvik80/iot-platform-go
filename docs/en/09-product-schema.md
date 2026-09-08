# Product schema

A product defines **what a device can do** — which RPC methods it supports, which telemetry metrics it sends, which attributes it stores. All devices of the same product share one schema.

The schema is stored as JSON within the product and used by the dashboard to render the device page UI.

---

## RPC methods

Define commands that can be invoked on the device. Displayed as buttons on the device page.

```json
[
  {
    "name": "reboot",
    "label": "Reboot",
    "description": "Restart the device",
    "params": [
      { "name": "delayMs", "label": "Delay (ms)", "type": "number", "default": 500 }
    ]
  }
]
```

### Built-in `rpc-list`

Every device automatically provides the **`rpc-list`** method (alias `rpc_list`). The platform injects it into every product schema. When the device is online, the RPC console calls `rpc-list` and builds forms from the **live firmware catalog**:

```json
[
  { "method": "rpc-list", "params": {}, "required": [] },
  {
    "method": "echo",
    "params": { "msg": { "type": "string", "required": true } },
    "required": ["msg"]
  },
  {
    "method": "set_led",
    "description": "Set LED range color",
    "params": {
      "gpio": { "type": "int", "required": true },
      "offset": { "type": "int", "required": true },
      "length": { "type": "int", "required": true },
      "r": { "type": "int", "required": false, "default": 255 },
      "g": { "type": "int", "required": false, "default": 255 },
      "b": { "type": "int", "required": false, "default": 255 },
      "on": { "type": "bool", "required": false, "default": true }
    },
    "required": ["gpio", "offset", "length"]
  }
]
```

`rpc-list` cannot be registered manually or added as a custom method.

---

## Telemetry metrics

Define how telemetry keys are displayed in the dashboard — labels, units, grouping.

```json
[
  {
    "key": "freeHeap",
    "label": "Free heap",
    "group": "Memory",
    "unit": "bytes",
    "format": "bytes",
    "description": "Currently free internal heap"
  }
]
```

| Field | Description |
|-------|-------------|
| `key` | Key name in telemetry JSON |
| `label` | Display name |
| `group` | Group for visual grouping |
| `unit` | Measurement unit |
| `format` | Display format: `bytes` \| `percent` \| `duration` \| `bool` \| `rssi` \| `number` \| `text` |
| `description` | Description (optional) |

New products receive **ESP presets** automatically (`rpc-list`, `reboot` + system metrics: DRAM, heap, PSRAM, storage). **Reset to ESP presets** button on the product page restores these values.

---

## Attributes

Define fields for the attributes tab on the device page.

```json
[
  {
    "key": "targetTemperature",
    "label": "Target temperature",
    "scope": "desired",
    "type": "number",
    "unit": "°C",
    "writable": true
  }
]
```

| Field | Description |
|-------|-------------|
| `key` | Attribute name |
| `scope` | `reported` (device reports) \| `desired` (platform sends) \| `both` |
| `type` | Type: `number` \| `string` \| `bool` |
| `writable` | Can be changed via UI |

---

## API

| Method | Path | Description |
|--------|------|-------------|
| PUT | `/api/v1/products/{id}` | Update product; body may include `methods`, `metrics`, `attributes` |
| POST | `/api/v1/products/{id}/esp-defaults` | Restore ESP presets |
| DELETE | `/api/v1/products/{id}` | Delete product (error if devices exist) |
| GET | `/api/v1/devices/{id}` | Includes `product` with schemas |
