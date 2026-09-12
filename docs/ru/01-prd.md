# Cogitor IoT Platform — Требования к продукту

> **Версия**: 1.0
> **Статус**: Активен
> **Обновлено**: Август 2026

Этот документ определяет что делает платформа, для кого и зачем. Источник истины по скоупу функций и критериям приёмки.

Связанные доки: [API](02-api.md), [Авторизация](03-auth.md), [MQTT](04-mqtt.md), [Автоматизация](06-automation.md), [Эксплуатация](11-operations.md).

---

## 1. Видение

Cogitor IoT Platform — **self-hosted, одно-бинарный IoT-бэкенд и операционный дашборд** для управления ESP32-устройствами по MQTT. Предоставляет всё необходимое для небольшого/среднего развёртывания — подключение устройств, приём телеметрии, RPC, OTA-обновления, автоматизацию и интеграцию с LLM — без облачных сервисов.

Главный принцип: **один процесс, один бинарник, без внешнего SaaS**.

---

## 2. Проблемы

| Боль | Альтернативы | Подход Cogitor |
|------|-------------|---------------|
| Облачные IoT требуют интернет, стоят за устройство, привязывают данные | AWS IoT, ThingsBoard Cloud, Blynk | Self-hosted; данные на NAS / сервере оператора |
| Многокомпонентные стеки сложны | ThingsBoard CE + Mosquitto + PostgreSQL + Grafana | Один Go-бинарник с MQTT, SQLite, React |
| OTA ручная | Скрипты, ручная прошивка | Встроенная OTA: загрузка `.bin`, URL, авто-отправка |
| Автоматизация требует отдельный сервер | Node-RED, Home Assistant | Встроенные плагины (Go) для каждого устройства + опциональный Node-RED для кросс-устройственных правил |
| Нет AI-интеграции | — | MCP-эндпоинт для AI-клиентов |

---

## 3. Целевые пользователи

### 3.1 Основной — разработчик устройств / оператор лаборатории
- ESP32-устройства (LED-контроллеры, датчики, актуаторы)
- Дашборд для статуса, прошивки, команд
- Запуск на NAS или Raspberry Pi
- Минимальные операционные затраты

### 3.2 Вторичный — малый бизнес IoT
- 10–200 устройств на одном объекте
- Ролевой доступ (admin / operator / viewer)
- Правила автоматизации (расписания, тихие часы, пороговые алерты)
- Аудит (события, логи, история OTA)

### 3.3 Третичный — AI/LLM энтузиаст
- Управление устройствами через естественный язык
- MCP-совместимый клиент (Cursor, Claude)
- Стандартный протокол интеграции (MCP over HTTP)

---

## 4. Цели и метрики успеха

| Цель | Метрика | Значение |
|------|---------|----------|
| Минимальный footprint | Docker-контейнеров / процессов | 1 (платформа) + 1 опциональный (Node-RED) |
| Быстрое подключение | Время от «Создать устройство» до первой телеметрии | < 5 минут (включая прошивку) |
| Надёжное присутствие | Точность трекинга MQTT-сессий | 100% подключённых сессий в дашборде за 2 с |
| Zero-touch OTA | Устройства на последней прошивке после включения Auto-OTA | Все онлайн-устройства за один цикл переподключения |
| Оперативная видимость | Загрузка страниц дашборда | < 1 с для 200 устройств |
| AI-управляемость | MCP-инструменты доступны | Все основные операции доступны через MCP |

---

## 5. Функциональные требования

### 5.1 Жизненный цикл устройств

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-101 | Создание **продуктов** как архетипов устройств с общей схемой (RPC, телеметрия, атрибуты) | P0 | ✅ |
| F-102 | Регистрация **устройств** под продуктом; одноразовый токен показывается один раз | P0 | ✅ |
| F-103 | Ротация токена; старая MQTT-сессия немедленно отключается | P0 | ✅ |
| F-104 | Включение / отключение / удаление устройств; отключённые не могут авторизоваться | P0 | ✅ |
| F-105 | Автоопределение стиля MQTT-топиков (full `iot/v1/…` vs short `v1/me/…`) | P0 | ✅ |
| F-106 | Провижининг устройства: Wi-Fi + MQTT креды как JSON для прошивки | P1 | ✅ |

### 5.2 Телеметрия и атрибуты

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-201 | Приём телеметрии из MQTT; хранение в SQLite с временными метками | P0 | ✅ |
| F-202 | REST-запросы последней и исторической телеметрии | P0 | ✅ |
| F-203 | **Reported** и **desired** атрибуты; desired публикуются на устройство | P0 | ✅ |
| F-204 | Хранение телеметрии: автоочистка старше N дней (по умолчанию 30) | P1 | ✅ |
| F-205 | Схема метрик продукта: ключ, метка, группа, единица, формат | P1 | ✅ |

### 5.3 RPC

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-301 | RPC-запросы через MQTT downlink; ожидание ответа с настраиваемым таймаутом | P0 | ✅ |
| F-302 | RPC-консоль: автозаполнение из схемы + live `rpc-list` | P0 | ✅ |
| F-303 | История RPC для каждого устройства | P1 | ✅ |
| F-304 | Корреляция по JSON `id`; успех = `code: 0` | P0 | ✅ |

### 5.4 OTA-обновления

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-401 | Загрузка `.bin` (multipart, макс. 32 МБ); публичный URL (без auth для ESP HTTP GET) | P0 | ✅ |
| F-402 | Регистрация внешних URL прошивок (HTTPS) | P1 | ✅ |
| F-403 | Уведомление устройства или всего продукта через MQTT | P0 | ✅ |
| F-404 | **Auto-OTA**: автоотправка при подключении или отчёте версии | P0 | ✅ |
| F-405 | Отслеживание статуса: pending → running → success / failed / cancelled | P0 | ✅ |
| F-406 | Модули: один продукт, разные модули (main, factory) | P1 | ✅ |
| F-407 | Тот же продукт+модуль+версия заменяет образ | P1 | ✅ |

### 5.5 Аутентификация

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-501 | Вход по логину или email + пароль | P0 | ✅ |
| F-502 | Саморегистрация с подтверждением email (SMTP или log) | P1 | ✅ |
| F-503 | Google OAuth с привязкой аккаунтов | P1 | ✅ |
| F-504 | Роли: **admin** (полный), **operator** (чтение + запись), **viewer** (только чтение) | P0 | ✅ |
| F-505 | Начальный админ из переменных окружения | P0 | ✅ |
| F-506 | API-токены для MCP и скриптов (создаются в Settings, показываются один раз) | P0 | ✅ |
| F-507 | Страница Users: список, смена роли, отключение, удаление | P1 | ✅ |

### 5.6 Автоматизация

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-601 | **Шина правил**: JSON-события на `platform/v1/events/{product}/{device}/…` | P0 | ✅ |
| F-602 | **Команды**: Node-RED публикует на `platform/v1/commands/{product}/{device}/…` | P0 | ✅ |
| F-603 | **Плагины устройств**: внутрипроцессные автоматизации с RPC-обратным вызовом | P0 | ✅ |
| F-604 | LED-гирлянда: WS2812, эффекты, тихие часы, ночной свет | P1 | ✅ |
| F-605 | Расписание: RPC по локальному времени и дням недели | P1 | ✅ |
| F-606 | Node-RED в iframe дашборда | P1 | ✅ |
| F-607 | Расширяемость: Go `Register()` + `init()` | P2 | ✅ |

### 5.7 MQTT-брокер

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-701 | Встроенный MQTT-брокер (mochi-mqtt) — внешний не нужен | P0 | ✅ |
| F-702 | ACL устройств по `{product}.{device}` (только up/down топики) | P0 | ✅ |
| F-703 | Опциональный MQTT-over-WebSocket | P2 | ✅ |
| F-704 | LWT-присутствие (онлайн/офлайн) | P0 | ✅ |
| F-705 | Страница мониторинга MQTT: сессии, события брокера | P0 | ✅ |
| F-706 | NTP-синхронизация: `serverRecvTime` / `serverSendTime` | P1 | ✅ |

### 5.8 Дашборд

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-801 | React SPA (Bootstrap 5, Vite) встроен в Go | P0 | ✅ |
| F-802 | Страницы: Overview, MQTT, Products, Devices, Device Detail, Events, Logs, OTA, Automation, Settings, Users | P0 | ✅ |
| F-803 | Обновления через WebSocket (статус, телеметрия, MQTT, OTA) | P0 | ✅ |
| F-804 | Overview: счётчики online/offline/total, MQTT, события, сессии | P0 | ✅ |
| F-805 | Device Detail: подключение, метрики, desired-атрибуты, OTA, плагины, RPC, события | P0 | ✅ |
| F-806 | Мобильная адаптация с off-canvas навигацией | P1 | ✅ |

### 5.9 MCP (LLM)

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-901 | Streamable HTTP MCP на `/mcp` (один процесс, JWT/API token) | P0 | ✅ |
| F-902 | Инструменты: обзор, устройства, продукты, RPC, телеметрия, атрибуты | P0 | ✅ |
| F-903 | Инструменты записи требуют operator/admin | P0 | ✅ |
| F-904 | Совместимость с Cursor, Claude | P1 | ✅ |

### 5.10 Наблюдаемость

| ID | Требование | Приоритет | Статус |
|----|-----------|-----------|--------|
| F-1001 | Структурированные логи в stdout (`slog`): connect, disconnect, auth_fail, ACL, RPC, OTA | P0 | ✅ |
| F-1002 | Настраиваемый уровень: debug / info / warn / error | P0 | ✅ |
| F-1003 | Страница Events: фильтр по устройству, severity, времени | P0 | ✅ |
| F-1004 | Страница Logs: сообщения устройств с фильтром по уровню и времени | P1 | ✅ |
| F-1005 | Health-эндпоинты: `/healthz` (liveness), `/readyz` (DB + MQTT ready) | P0 | ✅ |

---

## 6. Нефункциональные требования

| ID | Категория | Требование |
|----|-----------|-----------|
| NF-01 | Развёртывание | Один Docker-образ (multi-stage: Node.js → Go → Alpine). Non-root. |
| NF-02 | Развёртывание | Альтернатива: bare-metal через `make build` → один бинарник + SQLite |
| NF-03 | Портативность | Windows (dev), Linux (prod), ARM64 (pure-Go SQLite драйвер) |
| NF-04 | Безопасность | JWT; SHA-256 хеши device tokens; ACL MQTT |
| NF-05 | Безопасность | Без device tokens в Node-RED; шина правил — отдельные креды |
| NF-06 | Безопасность | OTA download без auth (URL по токену) |
| NF-07 | Производительность | SQLite WAL (параллельное чтение при записи) |
| NF-08 | Производительность | WebSocket fan-out (без polling) |
| NF-09 | Надёжность | LWT; pending OTA на переподключение |
| NF-10 | Конфигурация | Каскад: env → `.env` → `./config.yml` → `/etc/...` |
| NF-11 | Обслуживание | Миграции в `internal/db/migrations/`; автоприменение |
| NF-12 | Расширяемость | Каталог плагинов с `init()`; form-driven UI |

---

## 7. Архитектура

> **Диаграмма**: [Архитектура платформы](14-diagrams.md#архитектура-платформы)

```mermaid
graph TB
    subgraph Devices["ESP32 Устройства"]
        ESP1["Датчик / Актуатор"]
        ESP2["LED Контроллер"]
        ESP3["Камера / Другое"]
    end

    subgraph Platform["Cogitor IoT Platform (один Go бинарник)"]
        subgraph Transport["MQTT Транспорт"]
            Broker["MQTT Broker<br/>(mochi-mqtt, встроенный)"]
        end

        subgraph Core["Основная обработка"]
            ACL["Device ACL<br/>(product.device идентичность)"]
            Ingest["Приём телеметрии"]
            PluginHub["Plugin Hub<br/>(в процессе)"]
            OTA["OTA Manager<br/>(Auto-OTA)"]
        end

        subgraph Services["Сервисы и хранилище"]
            Rules["Rules Bus<br/>publisher"]
            WS["WebSocket Hub<br/>(real-time)"]
            DB["SQLite<br/>(WAL mode)"]
            API["REST API<br/>(chi router)"]
            Auth["JWT / OAuth<br/>Auth"]
            MCP["MCP Endpoint<br/>/mcp"]
        end
    end

    subgraph External["Внешние клиенты"]
        UI["React Dashboard<br/>(embedded SPA)"]
        NR["Node-RED<br/>(опционально)"]
        LLM["LLM / AI Client<br/>(Cursor, Claude)"]
    end

    %% Устройство → Транспорт
    ESP1 & ESP2 & ESP3 -->|"MQTT TCP :1883<br/>MQTT WS :8083"| Broker

    %% Транспорт → Ядро
    Broker --> ACL
    ACL --> Ingest

    %% Ядро внутреннее
    Ingest --> PluginHub
    Ingest --> OTA

    %% Ядро → Сервисы
    Ingest --> DB
    Ingest --> WS
    Ingest --> Rules
    PluginHub -->|"InvokeRPC"| Broker
    OTA -->|"MQTT downlink<br/>v1/o/upd"| Broker

    %% Сервисы внутреннее
    API --> Auth
    API --> DB
    API --> WS
    API --> PluginHub
    MCP --> API

    %% Сервисы ↔ Внешние
    Rules -->|"platform/v1/events/..."| NR
    NR -->|"platform/v1/commands/..."| Broker
    UI -->|"REST + WS<br/>JWT / API token"| API
    LLM -->|"Streamable HTTP<br/>JWT / API token"| MCP

    style Platform fill:#1a1a2e,stroke:#16213e,color:#eee
    style Transport fill:#0f3460,stroke:#16213e,color:#eee
    style Core fill:#1a1a2e,stroke:#0f3460,color:#eee
    style Services fill:#16213e,stroke:#1a1a2e,color:#eee
    style Devices fill:#0f3460,stroke:#16213e,color:#eee
    style External fill:#16213e,stroke:#1a1a2e,color:#eee
```

### Поток данных

1. **Устройство → Платформа**: MQTT CONNECT с `{product}.{device}` / токен. PUBLISH телеметрии на `v1/t` или `iot/v1/.../up/telemetry/data`. Платформа принимает, хранит в SQLite, транслирует через WebSocket, публикует в шину правил.
2. **Платформа → Устройство**: RPC, desired-атрибуты, OTA — на downlink-топиках. Стиль топиков автоопределяется из uplink-префикса устройства.
3. **Платформа ↔ Node-RED**: События на `platform/v1/events/...`; команды на `platform/v1/commands/...`. Node-RED использует отдельный MQTT-пользователь с ACL.
4. **Дашборд → Платформа**: REST API + WebSocket для real-time обновлений. JWT или API-токен.
5. **LLM → Платформа**: MCP-эндпоинт на `/mcp` — инструменты для управления устройствами, телеметрии, RPC, OTA.

---

## 8. MQTT-протокол (Cogitor v1)

### Стили топиков

| Стиль | Префикс | Источник идентичности |
|-------|---------|----------------------|
| Full | `iot/v1/{product}/{device}/{up|down}/{capability}/{operation}` | Путь топика |
| Short | `v1/t`, `v1/r/req`, `v1/a/upd`, … | MQTT username (`{product}.{device}`) |

### Основные топики

| Направление | Короткий | Назначение |
|-------------|----------|-----------|
| ↑ | `v1/s` | Статус (LWT) |
| ↑ | `v1/t` | Телеметрия |
| ↑ | `v1/e` | События |
| ↑ | `v1/a` | Атрибуты (отчёт) |
| ↓ | `v1/a/upd` | Desired-атрибуты |
| ↓ | `v1/r/req` | RPC-запрос |
| ↑ | `v1/r/res` | RPC-ответ |
| ↓ | `v1/o/upd` | OTA-обновление (URL) |
| ↑ | `v1/o/p` | OTA-прогресс |
| ↑ | `v1/n/req` | NTP-запрос |
| ↓ | `v1/n/res` | NTP-ответ |

---

## 9. Конфигурация

### Переменные окружения (основные)

| Переменная | По умолчанию | Назначение |
|-----------|-------------|-----------|
| `HTTP_ADDR` | `:8080` | HTTP-адрес (дашборд + API) |
| `MQTT_ADDR` | `:1883` | MQTT TCP-слушатель |
| `SQLITE_PATH` | `./data/platform.db` | Файл базы данных SQLite |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Публичный URL (OTA, email, OAuth) |
| `JWT_SECRET` | dev-значение | Ключ подписи JWT — **сменить в production** |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@cogitor.local` / `admin` | Начальный администратор |
| `TIMEZONE` | `UTC` | Часовая зона для расписаний и тихих часов |
| `TELEMETRY_RETENTION_DAYS` | `30` | Автоочистка старых данных |
| `RULES_BUS_ENABLED` | `true` | Шина событий для Node-RED |
| `BROKER_HOST_PUBLIC` | `localhost` | MQTT-хост в креды устройства |

### Приоритет конфигурации

1. Переменные окружения (OS env)
2. Файл `.env`
3. `./config.yml`
4. `/etc/iot-platform-go/config.yml`

---

## 10. Развёртывание

### Docker Compose (рекомендуется)

```yaml
services:
  platform:
    build: .
    ports:
      - "8080:8080"   # HTTP + Dashboard
      - "1883:1883"   # MQTT
    volumes:
      - platform_data:/data
    environment:
      - PUBLIC_BASE_URL=http://<host-ip>:8080
      - JWT_SECRET=<change-me>

  nodered:              # опционально
    image: nodered/node-red:4.0
    ports:
      - "1880:1880"
```

### Bare Metal (разработка)

```bash
make build        # web UI + Go binary → bin/platform.exe
./bin/platform.exe
```

### Synology NAS

- Gitea CI собирает образ на push в `master`
- Монтирование `/volume1/docker/iot-platform-go/data` для SQLite
- `.env` из `/volume1/docker/iot-platform-go/etc/.env`

---

## 11. REST API (сводка)

| Область | Эндпоинты |
|---------|----------|
| Auth | `POST /auth/login`, `POST /auth/register`, `GET /auth/config`, Google OAuth |
| Overview | `GET /overview`, `GET /healthz`, `GET /readyz`, `GET /settings` |
| Products | CRUD `/products`, `POST /products/{id}/esp-defaults` |
| Devices | CRUD `/devices`, `POST /devices/{id}/token/rotate`, `POST /devices/{id}/enable`, `POST /devices/{id}/disable` |
| Telemetry | `GET /devices/{id}/telemetry`, `GET /devices/{id}/telemetry/latest` |
| Attributes | `GET /devices/{id}/attributes`, `PATCH /devices/{id}/attributes/desired` |
| RPC | `POST /devices/{id}/rpc`, `GET /devices/{id}/rpc` (история) |
| OTA | `POST /ota/firmwares/upload`, `POST /ota/firmwares/{id}/notify`, `POST /devices/{id}/ota` |
| Events/Logs | `GET /events`, `GET /logs` |
| Plugins | `GET /plugins`, CRUD `/devices/{id}/plugins`, `POST …/apply` |
| MQTT | `GET /mqtt` (сессии + события) |
| MCP | `/mcp` (streamable HTTP, JWT/token) |
| WebSocket | `GET /api/v1/ws?token=<jwt>` |

---

## 12. Технологический стек

| Слой | Технология |
|------|-----------|
| Backend | Go 1.25+, chi router |
| Database | SQLite (modernc.org/sqlite — pure Go, без CGO) |
| MQTT Broker | mochi-mqtt (embedded) |
| Frontend | React 18 + Bootstrap 5 + Vite |
| Auth | JWT (golang-jwt), Google OAuth |
| Real-time | gorilla/websocket |
| Automation | Node-RED (опционально, через шину правил MQTT) |
| AI Integration | MCP (modelcontextprotocol/go-sdk) |
| Container | Multi-stage Docker (Node.js 22 → Go 1.25 → Alpine 3.20) |

---

## 13. Вне скобки (v1)

- ~~Multi-tenancy / рабочие пространства~~ — реализовано, см. [16-multi-account.md](16-multi-account.md)
- Облачный relay / hybrid cloud-edge
- Нативные мобильные приложения
- Видеостриминг с ESP32-CAM
- Mesh-сети / device-to-device
- Маркетплейс плагинов
- Интернационализация UI

---

## 14. Будущие улучшения (v2+)

- **Multi-account / IAM**: см. [16-multi-account.md](16-multi-account.md) — реализованный релиз, тенанты, роли per-account, account-scope правил, встроенный мониторинг
- **MQTT 5**: shared subscriptions, message expiry
- **TimescaleDB / InfluxDB**: замена SQLite для высоконагруженной телеметрии
- **Маркетплейс плагинов**: пользовательские плагины с веб-установкой
- **Fleet management**: группы, теги, массовые операции
- **Alerting**: email/webhook при превышении порога (без Node-RED)
- **mDNS**: автообнаружение платформы из прошивки
- **TLS для MQTT**: нативный TLS-слушатель
- **Grafana**: экспорт телеметрии

---

## 15. Критерии приёмки

- [ ] Устройство подключается с токеном и появляется **online**
- [ ] Телеметрия видна и обновляется через WebSocket
- [ ] Desired-атрибут достигает устройства через MQTT downlink
- [ ] RPC round-trip с совпадающим `id` и `code: 0`
- [ ] NTP-запрос получает `serverRecvTime` / `serverSendTime`
- [ ] Ротация токена отключает старую сессию
- [ ] Node-RED получает события на `platform/v1/events/...`
- [ ] Auto-OTA отправляет прошивку при переподключении
- [ ] MCP-клиент может перечислять устройства и вызывать RPC через `/mcp`

---

## Связанная документация

| Док | Что |
|-----|------|
| [02-api.md](02-api.md) | REST API |
| [03-auth.md](03-auth.md) | Авторизация и роли |
| [04-mqtt.md](04-mqtt.md) | MQTT-протокол и топики |
| [05-mqtt-tls.md](05-mqtt-tls.md) | MQTT over TLS |
| [06-automation.md](06-automation.md) | Node-RED и шина правил |
| [07-internal-plugins.md](07-internal-plugins.md) | Встроенные плагины |
| [08-external-plugins.md](08-external-plugins.md) | Внешние плагины |
| [09-product-schema.md](09-product-schema.md) | Схема продукта (RPC, телеметрия, атрибуты) |
| [10-mcp.md](10-mcp.md) | MCP (LLM-интеграция) |
| [11-operations.md](11-operations.md) | Развёртывание и конфигурация |
| [16-multi-account.md](16-multi-account.md) | Multi-account, IAM, monitoring, plugin policies |
