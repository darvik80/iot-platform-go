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

New products get ESP defaults automatically (`reboot` + system metrics including `freeHeap`, `freeDram`, PSRAM/storage). Use **Reset to ESP presets** on the product page to restore predefined entries while keeping your custom methods/metrics.

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
