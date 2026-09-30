# Плагины устройств

Автоматизации для **каждого устройства**, работающие в процессе платформы. Не путать с Node-RED — он для **меж-устройственных** правил на шине правил MQTT.

Плагин — это **одно устройство + один тип из каталога**. Получает события устройства (статус, телеметрия, тики), вызывает RPC обратно, имеет UI-панель на странице устройства.

Встроенные плагины: `led-garland` (WS2812-ленты), `schedule` (RPC по времени), `prometheus-exporter` (endpoint метрик Prometheus) и `metrics-dashboard` (панели Metrics+ на странице устройства). Свой плагин — один Go-файл + `init()`.

---

## Архитектура

> **Диаграмма**: [Архитектура внутренних плагинов](14-diagrams.md#архитектура-внутренних-плагинов)

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

| Компонент | Где | Роль |
|-----------|-----|------|
| **Каталог** | `internal/plugins` `Register()` в `init()` | Встроенные типы (`led-garland`, `schedule`, `prometheus-exporter`, `metrics-dashboard`). `GET /plugins`. |
| **Политика** | `/plugins/policies` (admin) | Включение/выключение (и настройки) каждого плагина каталога для аккаунта. |
| **Binding** | Таблица БД `device_plugin_bindings` (SQLite или PostgreSQL) | `UNIQUE(device_pk, plugin_id)`. Один плагин на многих устройствах = независимые инстансы/конфиги. |
| **Hub** | `services.PluginHub` | Один runner на каждый включённый binding. Запускается из `main` через `StartPlugins`. |
| **Instance** | `plugins.Instance` | `ApplyConfig` → `OnEvent` → `IntervalMS` / `Auto`. |
| **Runtime** | hub → `Platform.InvokeRPC` | Fire-and-forget RPC (без JWT). Тот же downlink, что и консоль дашборда. |

Hub **не говорит** в MQTT. Ingest уже распарсил конверт; `publishRules` рассылает в плагины **и** (если включено) в Node-RED.

### Жизненный цикл

> **Диаграмма**: [Жизненный цикл внутреннего плагина](14-diagrams.md#жизненный-цикл-внутреннего-плагина)

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

| `OnEvent` тип | Когда | Действие плагина |
|---------------|-------|-----------------|
| `status` | MQTT online/offline (после runner resync) | Подключить оборудование, сбросить состояние. `ev.Online` — авторитетный. |
| `apply` | Сохранение конфига, bind, или `POST …/apply` | Повторно применить конфиг к устройству (должно быть онлайн). |
| `tick` | Hub ticker пока **онлайн** и `Auto()` | Кадры анимации, опрос расписания. |
| `telemetry` / `event` / `rpc/result` | Те же конверты, что и шина правил | Опциональная реакция. |

`Auto()` + `IntervalMS()` управляют тикером. Offline всегда приостанавливает. Статические цвета LED не тикают, если не включены временные окна.

Плагины **не должны**: открывать MQTT, использовать токены устройств, заменять Node-RED для мульти-устройственных правил, блокировать надолго внутри `OnEvent`.

---

## Встроенный каталог

| id | Описание |
|----|----------|
| `led-garland` | WS2812-ленты через firmware `set_led`, тихие часы / ночной свет |
| `schedule` | Вызов RPC устройства по локальному времени (перезагрузка в 04:00, свет выключить в 23:00, …) |
| `prometheus-exporter` | Отдаёт метрики платформы (устройства, телеметрия, RPC) в текстовом формате Prometheus на собственном HTTP-порту — см. [Prometheus exporter](#prometheus-exporter) |
| `metrics-dashboard` | Пассивный: хранит раскладку панелей вкладки **Metrics+** устройства — см. [Metrics dashboard](#metrics-dashboard) |

Каждый плагин каталога админ может включить или выключить для аккаунта: `GET /plugins/policies` возвращает каталог с действующей политикой (по умолчанию включено), `PATCH /plugins/policies/{pluginId}` с `{ "enabled"?: bool, "settings"?: {} }` меняет её и сразу пересобирает runner-ы аккаунта.

Binding хранится в БД платформы (SQLite или PostgreSQL, таблица `device_plugin_bindings`). Тикер работает пока устройство **онлайн** и плагину нужны кадры (`mode: auto`, анимированный эффект, тихие/активные окна, или `schedule`). Offline приостанавливает. Статические цвета (`off` / `white` / …) не тикают, если не включены временные окна.

**`TIMEZONE`** платформы (IANA, напр. `Asia/Bangkok`) — по умолчанию для всех временных окон. Настраивается в `config.yml` / `.env`. Binding может переопределить через `config.tz`. Пустой `tz` → зона платформы. Неизвестная зона → ошибка сохранения.

---

## LED-гирлянда

Firmware привязывает WS2812-ленты в runtime. Плагин поэтому имеет **обязательный шаг attach**:

1. **Save / Apply now** (`apply`) или **device online** (`status`) → `led_attach` на каждую ленту `{ gpio, count, brightness }`
2. Затем `set_led` кадры `{ gpio, offset, length, r, g, b, on }`
3. Offline очищает attach; следующий online снова прикрепляет. Тики не пере-прикрепляют, если attach не потерян.

На странице устройства: **Add strip** / **Delete strip** (сохраняется сразу). GPIO + количество LED на ленту.

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

`interval_ms` — период кадра для анимированных эффектов **и** для `mode: auto`.
`mode: auto` дополнительно переключает эффект каждые `auto_ms`. Статические цвета не используют тикер, если не включены **тихие часы** или **только между** (тогда хаб опрашивает ~15с для входа/выхода из окна).

- **Тихие часы** — внутри `[from, to)` (переходит через полночь если `from > to`) принудительно `quiet.effect` (обычно `off` / `warm`). Strobe/chase останавливаются ночью.
- **Только между** (ночной свет) — вне окна лента выключена. Комбинируется с тихими: вне active → off; внутри active ∩ quiet → quiet effect; иначе нормально.

RPC:

| Метод | Параметры | Когда |
|-------|-----------|-------|
| `led_attach` | `gpio`, `count` (= длина ленты), `brightness` (1–255, по умолчанию 32) | save / apply / device online |
| `set_led` | `gpio`, `offset`, `length`, `r`, `g`, `b`, `on` | после attach, каждый кадр |

Повторный `led_attach` на тот же GPIO пересоздаёт ленту (firmware).

---

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

---

## Prometheus exporter

`prometheus-exporter` запускает отдельный HTTP-сервер, который отдаёт внутренние метрики платформы (онлайн-гейджи устройств, сэмплы телеметрии, RPC-вызовы, публикации в шину правил) в текстовом формате Prometheus. RPC не отправляет и не тикает.

Конфиг по умолчанию:

```json
{
  "port": 9100,
  "bind": "0.0.0.0",
  "path": "/metrics",
  "bearer_token": "",
  "update_interval_ms": 5000
}
```

| Поле | Значение |
|------|----------|
| `port` | Порт (некорректный → `9100`) |
| `bind` | Адрес (пусто → `0.0.0.0`) |
| `path` | Путь метрик (пусто → `/metrics`) |
| `bearer_token` | Если задан, запрос должен содержать `Authorization: Bearer <token>`, иначе `401` |
| `update_interval_ms` | Принимается, но сейчас не используется (периодического цикла обновления нет) |

HTTP-сервер запускается по событию `apply` (сохранение binding / bind / **Apply now**; привязанное устройство должно быть онлайн). В v1.0.x хаб его никогда не останавливает: выключение или удаление binding оставляет listener работающим, а изменённые `port` / `bind` / `path` не заменяют сервер, уже занявший порт, — для применения таких изменений перезапустите платформу. Метрики берутся из общего стора платформы (`cogitor_device_online`, `cogitor_devices_total`, `cogitor_device_telemetry_samples_total`, `cogitor_device_rpc_calls_total`, `cogitor_event_bus_published_total`, `cogitor_mqtt_clients`, `cogitor_external_plugin_online`, `cogitor_device_ota_jobs_total`), а не только по привязанному устройству.

---

## Metrics dashboard

`metrics-dashboard` пассивный: не выполняет автоматизацию, не шлёт RPC и не запускает сервер. Его конфиг — список панелей, который читает `GET /devices/{id}/dashboard` для вкладки **Metrics+**; без binding используются панели по умолчанию.

Конфиг по умолчанию:

```json
{
  "panels": [
    { "id": "p1", "type": "telemetry_chart", "title": "WiFi RSSI", "keys": ["wifi_rssi"], "window": "1h" },
    { "id": "p2", "type": "telemetry_gauge", "title": "Free Heap", "key": "free_heap", "window": "1h" },
    { "id": "p3", "type": "online_timeline", "title": "Online Timeline", "window": "24h" }
  ]
}
```

Поля панели: `id`, `type`, `title`, `keys` (`telemetry_chart`), `key` (`telemetry_gauge`), `window` (`30m`, `1h`, `24h`, `7d`, …; по умолчанию `1h`), `limit` (`rpc_table`, по умолчанию 20), `severity` (`events_count`). Типы панелей: `telemetry_chart`, `telemetry_gauge`, `rpc_table`, `online_timeline`, `events_count`.

---

## API

| Метод | Путь |
|-------|------|
| GET | `/plugins` |
| GET | `/devices/{id}/plugins` |
| POST | `/devices/{id}/plugins` | `{ "plugin_id": "led-garland", "config"?: {}, "enabled"?: true }` |
| PUT | `/devices/{id}/plugins/{bindingId}` | `{ "enabled"?: bool, "config"?: {} }` |
| DELETE | `/devices/{id}/plugins/{bindingId}` |
| POST | `/devices/{id}/plugins/{bindingId}/apply` | один кадр сейчас (устройство должно быть онлайн) |
| GET | `/plugins/policies` | каталог + политика аккаунта (admin) |
| PATCH | `/plugins/policies/{pluginId}` | `{ "enabled"?: bool, "settings"?: {} }` (admin) |

UI: страница устройства → **Plugins**. Форма рендерится из **`Descriptor.Form`** (GET `/plugins` → `form`). Save / Apply / Remove — в shell. Нет `form` → JSON textarea.

---

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
		return nil
	}
	return nil
}
```

Контракт:

| Метод | Роль |
|-------|------|
| `ApplyConfig` | парсинг/валидация JSON конфига; вызывается при bind/update |
| `OnEvent` | `tick` / `apply` / `status` / `telemetry` / `event` / `rpc/result`. Использовать `status`+`Online` для attach-on-connect. |
| `IntervalMS` | период тика когда `Auto()` |
| `Auto` | `true` → хаб тикает пока устройство онлайн |

`Runtime.InvokeRPC` — fire-and-forget (без JWT). Тот же путь, что и дашборд RPC.

### 2. Регистрация

`init()` + `Register(...)` достаточно. Тот же пакет `plugins` импортируется хабом, новый файл линкуется автоматически. **Не забыть** `init()` или плагин не появится в `GET /plugins`.

Опциональный `Effects []string` питает `options_from: "effects"` на select-полях.

### 3. UI (не трогать webui)

Установить `Descriptor.Form` (`[]FormField`). Типы: `number` `string` `select` `checkbox` `time` `timezone` `json` `weekdays` `object` `array` `break`. `break` — layout-only (опциональный `label` + `<hr>`). Дашборд рендерит; Save/Apply/Remove — в shell. Пустой `Form` → JSON textarea.

```go
Form: []FormField{
  {Key: "interval_ms", Type: "number", Label: "Interval ms", Min: num(200)},
  {Key: "rpc", Type: "string", Label: "RPC"},
},
```

### 4. Запуск

```bash
go test ./internal/plugins/ ./internal/services/
# перезапустить платформу
```

Страница устройства → **Add plugin** → выбрать новое имя → редактировать конфиг → Save. `enabled` + `Auto()` запускает тикер после подключения устройства. Если firmware нужен одноразовый bind (как `led_attach`), делать на `apply` **и** `status` online.

### 5. Тесты

Предпочитать чистые хелперы (как `LEDFrame`) без MQTT. Binding CRUD: `internal/services/plugin_hub_test.go`.

### Не делать

- Не использовать MQTT / токены устройств из плагина — только `InvokeRPC` + события.
- Не заменять Node-RED для меж-устройственных правил.
- Не блокировать надолго внутри `OnEvent` (тики идут последовательно).

---

## Внешние плагины

Внешние плагины работают как **отдельные процессы** (любой язык). Единственный реализованный транспорт — **MQTT**: плагин подключается к встроенному брокеру с кредами `extplg_{account_slug}_{plugin_id}`, подписывается на события шины правил своего аккаунта и публикует команды. Транспорты Webhook, Polling и gRPC запланированы, но не реализованы. Внешние плагины не используют binding-и устройств и не получают события `tick` / `apply`.

Спецификация (что реализовано, что запланировано) — [08-external-plugins.md](08-external-plugins.md), MQTT-примеры — [examples/plugins/](../../examples/plugins/) (Python, Go, Java, C++).
