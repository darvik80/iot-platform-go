# Product schema (methods / metrics / attributes)

Each product stores JSON schemas used by the device UI.

## Methods (RPC)

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

Shown as buttons on the device page; click fills method/params and invokes RPC.

Firmware always exposes built-in **`rpc-list`** (alias **`rpc_list`**). It cannot be registered in the SDK (`iot->rpc().add`) or added as a custom product method. The platform injects it into every product schema. When the device is online, the RPC console calls `rpc-list` and builds forms from the live catalog:

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

## Metrics (telemetry)

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

`format`: `bytes` | `percent` | `duration` | `bool` | `rssi` | `number` | `text`

New products get ESP defaults automatically (`rpc-list`, `reboot` + system metrics including DRAM/heap/PSRAM/storage). Firmware methods (LED, echo, …) come from live `rpc-list`, or can be added as custom product methods. Use **Reset to ESP presets** on the product page to restore predefined entries while keeping your custom methods/metrics.

Methods support multiple parameters via the UI parameter rows (`+ Add parameter`).

## Attributes

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

`scope`: `reported` | `desired` | `both`

## API

| Method | Path |
|--------|------|
| PUT | `/api/v1/products/{id}` — body may include `methods`, `metrics`, `attributes` |
| POST | `/api/v1/products/{id}/esp-defaults` |
| DELETE | `/api/v1/products/{id}` — fails if devices still exist |
| GET | `/api/v1/devices/{id}` — includes `product` with schemas |
