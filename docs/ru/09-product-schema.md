# Схема продукта (методы / метрики / атрибуты)

Каждый продукт хранит JSON-схемы, используемые UI устройства.

## Методы (RPC)

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

Отображаются как кнопки на странице устройства; клик заполняет метод/параметры и вызывает RPC.

Firmware всегда предоставляет встроенный **`rpc-list`** (алиас **`rpc_list`**). Его нельзя зарегистрировать в SDK (`iot->rpc().add`) или добавить как пользовательский метод продукта. Платформа инжектит его в каждую схему продукта. Когда устройство онлайн, RPC-консоль вызывает `rpc-list` и строит формы из живого каталога:

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

## Метрики (телеметрия)

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

Новые продукты получают ESP-значения по умолчанию автоматически (`rpc-list`, `reboot` + системные метрики включая DRAM/heap/PSRAM/storage). Использовать **Reset to ESP presets** на странице продукта для восстановления предустановок.

## Атрибуты

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

| Метод | Путь |
|-------|------|
| PUT | `/api/v1/products/{id}` — тело может включать `methods`, `metrics`, `attributes` |
| POST | `/api/v1/products/{id}/esp-defaults` |
| DELETE | `/api/v1/products/{id}` — ошибка если есть устройства |
| GET | `/api/v1/devices/{id}` — включает `product` со схемами |
