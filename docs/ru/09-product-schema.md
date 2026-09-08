# Схема продукта

Продукт определяет **что умеет устройство** — какие RPC-методы поддерживать, какие метрики телеметрии отправлять, какие атрибуты хранить. Все устройства одного продукта разделяют одну схему.

Схема хранится как JSON внутри продукта и используется дашбордом для рендеринга UI страницы устройства.

---

## RPC-методы

Определяют команды, которые можно вызвать на устройстве. Отображаются как кнопки на странице устройства.

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

### Встроенный `rpc-list`

Каждое устройство автоматически предоставляет метод **`rpc-list`** (алиас `rpc_list`). Платформа инжектит его в каждую схему продукта. При онлайн-устройстве RPC-консоль вызывает `rpc-list` и строит формы из **живого каталога** прошивки:

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

`rpc-list` нельзя зарегистрировать вручную или добавить как пользовательский метод.

---

## Метрики телеметрии

Определяют как отображать ключи телеметрии в дашборде — подписи, единицы, группировка.

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

| Поле | Описание |
|------|----------|
| `key` | Имя ключа в JSON телеметрии |
| `label` | Отображаемое имя |
| `group` | Группа для визуальной группировки |
| `unit` | Единица измерения |
| `format` | Формат отображения: `bytes` \| `percent` \| `duration` \| `bool` \| `rssi` \| `number` \| `text` |
| `description` | Описание (опционально) |

Новые продукты получают **ESP-пресеты** автоматически (`rpc-list`, `reboot` + системные метрики: DRAM, heap, PSRAM, storage). Кнопка **Reset to ESP presets** на странице продукта восстанавливает эти значения.

---

## Атрибуты

Определяют поля для вкладки атрибутов на странице устройства.

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

| Поле | Описание |
|------|----------|
| `key` | Имя атрибута |
| `scope` | `reported` (устройство сообщает) \| `desired` (платформа отправляет) \| `both` |
| `type` | Тип: `number` \| `string` \| `bool` |
| `writable` | Можно ли менять через UI |

---

## API

| Метод | Путь | Описание |
|-------|------|----------|
| PUT | `/api/v1/products/{id}` | Обновить продукт; тело может включать `methods`, `metrics`, `attributes` |
| POST | `/api/v1/products/{id}/esp-defaults` | Восстановить ESP-пресеты |
| DELETE | `/api/v1/products/{id}` | Удалить продукт (ошибка если есть устройства) |
| GET | `/api/v1/devices/{id}` | Включает `product` со схемами |
