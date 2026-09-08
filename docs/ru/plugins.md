# Плагины устройств

Автоматизации для каждого устройства, работающие **в процессе платформы** (не Node-RED). Node-RED — для **меж-устройственных** правил на шине правил MQTT. Плагины — **одно устройство + один тип из каталога**: получают те же события, вызывают RPC обратно, UI-панель на странице устройства.

## Архитектура

```mermaid
flowchart LR
  Dev[Device MQTT] -->|status / telemetry / rpc result| Ing[ingest]
  UI[Dashboard Plugins] -->|Save / Apply now| API["REST /devices/id/plugins"]
  Ing --> PR[publishRules]
  PR --> Hub[PluginHub]
  PR -.->|optional| NR[Node-RED]
  API --> SQ[(device_plugin_bindings)]
  API --> Hub
  SQ --> Hub
  Cat[Catalog Register init] --> Hub
  Hub --> R[runner per binding]
  R --> AC[ApplyConfig]
  AC --> OE[OnEvent]
  OE --> RPC[InvokeRPC]
  RPC --> Dev
```

| Компонент | Где | Роль |
|-----------|-----|------|
| **Каталог** | `internal/plugins` `Register()` в `init()` | Встроенные типы (`led-garland`, `schedule`, …). `GET /plugins`. |
| **Binding** | SQLite `device_plugin_bindings` | `UNIQUE(device_pk, plugin_id)`. Один плагин на многих устройствах = независимые инстансы/конфиги. |
| **Hub** | `services.PluginHub` | Один runner на каждый включённый binding. Запускается из `main` через `StartPlugins`. |
| **Instance** | `plugins.Instance` | `ApplyConfig` → `OnEvent` → `IntervalMS` / `Auto`. |
| **Runtime** | hub → `Platform.InvokeRPC` | Fire-and-forget RPC (без JWT). Тот же downlink, что и консоль дашборда. |

### Жизненный цикл

| `OnEvent` тип | Когда | Действие плагина |
|---------------|-------|-----------------|
| `status` | MQTT online/offline | Подключить оборудование, сбросить состояние. |
| `apply` | Сохранение конфига, bind, или `POST …/apply` | Повторно применить конфиг к устройству. |
| `tick` | Hub ticker пока **онлайн** и `Auto()` | Кадры анимации, опрос расписания. |
| `telemetry` / `event` / `rpc/result` | Те же конверты, что и шина правил | Опциональная реакция. |

## Встроенный каталог

| id | Описание |
|----|----------|
| `led-garland` | WS2812-ленты через firmware `set_led`, тихие часы / ночной свет |
| `schedule` | Вызов RPC устройства по локальному времени (перезагрузка в 04:00, свет выключить в 23:00, …) |

**`TIMEZONE`** платформы (IANA, напр. `Asia/Bangkok`) — по умолчанию для всех временных окон. Binding может переопределить через `config.tz`.

## LED-гирлянда

Конфиг по умолчанию:

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

Эффекты: `off`, `white`, `warm`, `red`, `green`, `blue`, `blink`, `strobe`, `breathe`, `chase`, `comet`, `scanner`, `fill`, `theater`, `sparkle`, `fire`, `rainbow`, `police`, `christmas`.

- **Тихие часы** — внутри `[from, to)` (переходит через полночь если `from > to`) принудительно `quiet.effect`.
- **Только между** (ночной свет) — вне окна лента выключена.

## Расписание

Задачи выполняются раз в подходящую минуту, пока устройство онлайн (опрос 10с). `days` — `time.Weekday` (`0` воскресенье … `6` суббота); пусто / все семь = каждый день.

```json
{
  "tz": "",
  "jobs": [
    { "name": "night off", "at": "23:00", "days": [], "rpc": "set_led", "params": { "gpio": 16, "offset": 0, "length": 8, "on": false } },
    { "name": "weekday reboot", "at": "04:00", "days": [1, 2, 3, 4, 5], "rpc": "reboot", "params": {} }
  ]
}
```

## API

| Метод | Путь |
|-------|------|
| GET | `/plugins` |
| GET | `/devices/{id}/plugins` |
| POST | `/devices/{id}/plugins` | `{ "plugin_id": "led-garland", "config"?: {}, "enabled"?: true }` |
| PUT | `/devices/{id}/plugins/{bindingId}` | `{ "enabled"?: bool, "config"?: {} }` |
| DELETE | `/devices/{id}/plugins/{bindingId}` |
| POST | `/devices/{id}/plugins/{bindingId}/apply` | один кадр сейчас (устройство должно быть онлайн) |

## Как добавить новый плагин

V1 плагины — **Go builtins** (без zip/npm). Один файл + `init()` регистрация. Пересобрать бинарник платформы.

### 1. Реализовать `plugins.Instance`

Создать `internal/plugins/<name>.go` в пакете `plugins`:

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

func (p *Ping) ApplyConfig(raw json.RawMessage) error { /* ... */ return nil }
func (p *Ping) IntervalMS() int { return p.interval }
func (p *Ping) Auto() bool      { return p.auto }
func (p *Ping) OnEvent(rt Runtime, ev Event) error {
	switch ev.Kind {
	case "tick", "apply":
		return rt.InvokeRPC(p.rpc, map[string]any{"msg": p.msg})
	}
	return nil
}
```

### 2. Регистрация

`init()` + `Register(...)` достаточно. **Не забыть** `init()` или плагин не появится в `GET /plugins`.

### 3. UI

Установить `Descriptor.Form` (`[]FormField`). Типы: `number` `string` `select` `checkbox` `time` `timezone` `json` `weekdays` `object` `array` `break`. Пустой `Form` → JSON textarea.

### 4. Запуск

```bash
go test ./internal/plugins/ ./internal/services/
# перезапустить платформу
```

### Не делать

- Не использовать MQTT / токены устройств из плагина — только `InvokeRPC` + события.
- Не заменять Node-RED для меж-устройственных правил.
- Не блокировать надолго внутри `OnEvent`.
