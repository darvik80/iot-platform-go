# Cogitor IoT Platform

**Self-hosted IoT-бэкенд и дашборд для управления ESP32-устройствами.**

[English version](README.md)

Один Go-бинарник. Встроенный MQTT-брокер. SQLite или PostgreSQL. React-дашборд. Мультиаккаунтность. Без облачных зависимостей.

![Архитектура](docs/images/architecture.svg)

---

## Содержание

- [Что это?](#что-это)
- [Возможности](#возможности)
- [Быстрый старт](#быстрый-старт)
- [Дашборд](#дашборд)
- [Подключение устройств](#подключение-устройств)
- [Телеметрия и атрибуты](#телеметрия-и-атрибуты)
- [RPC (удалённые вызовы)](#rpc-удалённые-вызовы)
- [OTA-обновления](#ota-обновления)
- [Автоматизация и плагины](#автоматизация-и-плагины)
- [MCP — интеграция с LLM / AI](#mcp--интеграция-с-llm--ai)
- [Аутентификация и пользователи](#аутентификация-и-пользователи)
- [Конфигурация](#конфигурация)
- [Развёртывание](#развёртывание)
- [MQTT-протокол](#mqtt-протокол)
- [Документация](#документация)
- [Лицензия](#лицензия)

---

## Что это?

Cogitor IoT Platform — **однопросцессный IoT-бэкенд**, работающий на NAS, Raspberry Pi или любом Linux/Windows-сервере. Заменяет типичный стек (MQTT-брокер + бэкенд + БД + дашборд) одним бинарником, который можно собрать и запустить за минуты.

| Слой | Стек |
|------|------|
| API / ingest | Go 1.26+, chi router, SQLite / PostgreSQL |
| Брокер | Встроенный MQTT ([mochi-mqtt](https://github.com/mochi-mqtt/server)) в процессе платформы |
| Автоматизация | Встроенные плагины + внешние плагины / Node-RED через MQTT-шину `platform/v1/...` |
| UI | React 19 + Bootstrap 5 (Vite), встроен в Go-бинарник |
| MCP | Встроенный `/mcp` (JWT) — LLM-инструменты для устройств |

**Без облака. Без SaaS. Без платы за устройство.** Данные остаются на вашем оборудовании.

---

## Возможности

- **Жизненный цикл устройств** — создание продуктов, регистрация устройств, одноразовые токены, ротация токенов
- **Приём телеметрии** — хранение и запросы метрик устройств с автоматической очисткой
- **Желаемые атрибуты** — отправка конфигурации на устройства через MQTT downlink
- **RPC-консоль** — вызов методов устройства (перезагрузка, пользовательские) с автозаполнением форм
- **OTA-обновления** — загрузка `.bin` прошивок, хостинг URL, авто-отправка при переподключении (Auto-OTA)
- **Встроенный MQTT-брокер** — внешний брокер не нужен; TCP, WebSocket и TLS (опционально mTLS); ACL по идентификатору устройства
- **Дашборд реального времени** — обновления статуса, телеметрии и MQTT-сессий через WebSocket
- **Автоматизация** — встроенные плагины (LED-гирлянда, расписание, Prometheus-экспортер, дашборд метрик); шина правил для внешних плагинов и Node-RED
- **Внешние плагины** — отдельные процессы на любом языке, подключаются по MQTT
- **Мультиаккаунтность** — аккаунты, участники, приглашения, политики плагинов для каждого аккаунта
- **Мастер установки** — при первом запуске настраивает БД, администратора, SMTP и MQTT TLS
- **MCP-эндпоинт** — AI-клиенты (Cursor, Claude и др.) могут просматривать устройства, читать телеметрию, вызывать RPC
- **Аутентификация и роли** — JWT-логин, Google OAuth, саморегистрация, роли admin / operator / viewer
- **API-токены** — долгоживущие токены для скриптов и MCP-интеграции

---

## Быстрый старт

### Требования

- Go 1.26+ (для bare-metal) или Docker (для контейнерного развёртывания)
- Node.js 22+ (только для сборки фронтенда; в Docker уже собран)

### 1. Сборка и запуск (bare metal)

```bash
git clone git@gitea.darvik.synology.me:darvik/iot-platform-go.git
cd iot-platform-go

# Собрать дашборд + API за один шаг
make build

# Запустить
./bin/platform.exe
```

Или пошагово:

```bash
make web                                          # собрать фронтенд
go build -o bin/platform.exe ./cmd/platform       # собрать Go-бинарник
./bin/platform.exe
```

### 2. Запуск через Docker

```bash
# Compose: образ с Docker Hub; порты и папки хоста задаются в .env (см. docs/ru/13-deploy-docker.md)
cp .env.example .env && docker compose up -d     # --build — собрать из текущего checkout
# + Node-RED: docker compose --profile nodered up -d

# Или просто docker run: -p ХОСТ:КОНТЕЙНЕР, данные → /opt/iot-platform/lib, конфиг → /opt/iot-platform/etc
docker build -t cogitor-iot .
docker run -d --name cogitor-iot -p 8080:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v "$PWD/iot/data:/opt/iot-platform/lib" -v "$PWD/iot/etc:/opt/iot-platform/etc" cogitor-iot
```

### 3. Первый запуск — мастер установки

Если `DATABASE_DSN` не задан и `config.yml` нет, платформа стартует в **режиме установки**: откройте http://localhost:8080, и мастер проведёт через выбор БД (SQLite / PostgreSQL), первого администратора, первый аккаунт, SMTP, JWT-секрет, MQTT TLS и публичный URL. Результат записывается в `config.yml`, платформа продолжает запуск. Логина `admin/admin` по умолчанию нет.

> **Безопасность**: эндпоинты мастера работают без авторизации. Проходите мастер на localhost или в доверенной сети и перезапустите процесс после завершения.

| Сервис | URL |
|--------|-----|
| Дашборд / API | http://localhost:8080 |
| MQTT-брокер | `mqtt://localhost:1883` |
| MQTT WebSocket | `ws://localhost:8083/` (подпротокол `mqtt`) |
| MQTT TLS | `mqtts://localhost:8883` (если настроены сертификаты) |
| Node-RED (опционально) | http://localhost:1880 |

> **Совет**: Для разработки с hot-reload UI выполните `cd web && npm run dev` в отдельном терминале (API должен работать на `:8080`).

---

## Дашборд

![Дашборд](docs/images/dashboard-overview.png)

React SPA, встроенный в Go-бинарник. Тёмная тема, адаптивный интерфейс для управления всем IoT-развёртыванием.

### Навигация

| Страница | Описание |
|----------|----------|
| **Overview** | Счётчики онлайн/офлайн/всего, статус MQTT, последние события, активные сессии |
| **MQTT** | Живые сессии брокера, подключённые клиенты, события брокера в реальном времени |
| **Products** | Архетипы устройств — создание продуктов, определение RPC-методов, метрик, атрибутов |
| **Devices** | Все зарегистрированные устройства со статусом, версией прошивки, последним появлением |
| **Device Detail** | Информация о подключении, телеметрия, желаемые атрибуты, OTA-панель, плагины, RPC-консоль, события |
| **Events** | События устройств с фильтрацией по серьёзности, устройству, времени |
| **Logs** | Лог-сообщения устройств из MQTT `v1/me/l` |
| **OTA** | Управление прошивками — загрузка `.bin`, регистрация URL, уведомление устройств, Auto-OTA, отслеживание задач |
| **Settings** | Информация о платформе, API-токены, внешние плагины |
| **Users** | (Только admin) Список пользователей, управление ролями, отключение/удаление |
| **Account** | Участники аккаунта, приглашения, политики плагинов |
| **Audit Log** | Журнал действий в аккаунте |
| **Docs** | Встроенная справка |

![Устройства](docs/images/dashboard-devices.png)

![Карточка устройства](docs/images/dashboard-device-detail.png)

### Обновления в реальном времени

Дашборд держит WebSocket (`GET /api/v1/ws?token=<jwt>`) и получает события `device_status`, `telemetry`, `event`, `mqtt` и др. При подключении устройства, телеметрии или смене статуса страницы обновляются автоматически; клиент сам переподключается при обрыве. За reverse proxy нужно пробросить `Upgrade`/`Connection`.

---

## Подключение устройств

![Подключение устройства](docs/images/device-onboarding.svg)

### Шаг 1 — Создать продукт

**Продукт** определяет архетип устройства: RPC-методы, метрики телеметрии, атрибуты.

1. **Products** → ввести `product_id` (напр. `esp32-cam`) и имя
2. Нажать **Create**
3. Открыть продукт для определения:
   - **RPC-методы** — `reboot`, пользовательские методы с типизированными параметрами
   - **Метрики** — ключи телеметрии с метками, единицами, форматом
   - **Атрибуты** — reported / desired / both, с типом и доступностью записи

Новые продукты получают **ESP-значения по умолчанию** автоматически.

### Шаг 2 — Зарегистрировать устройство

1. **Devices** → **Add device**
2. Выбрать продукт, ввести `device_id` (напр. `cam-001`)
3. Нажать **Create**
4. **Скопировать токен доступа** из модального окна — показывается **только один раз**

### Шаг 3 — Прошить устройство

```cpp
CogitorCredentials::createAccessToken("esp32-cam", "cam-001", "192.168.1.22", "<token>");
```

Параметры MQTT CONNECT:

```
client_id = {product_id}.{device_id}
username  = {product_id}.{device_id}
password  = <access_token>
```

### Шаг 4 — Устройство онлайн

После подключения:
- Статус **online** в дашборде (зелёный бейдж)
- Телеметрия начинает поступать на страницу устройства
- Auto-OTA отправляет последнюю прошивку при различии версий
- Платформа автоопределяет стиль топиков (полный `iot/v1/...` или короткий `v1/me/...`)

### Ротация токена

1. Открыть страницу устройства
2. Нажать **Rotate token**
3. Старая MQTT-сессия немедленно отключена
4. Новый токен показан один раз — обновить прошивку устройства

---

## Телеметрия и атрибуты

### Телеметрия

Устройства публикуют телеметрию на `v1/me/t` (короткий) или `iot/v1/.../up/telemetry/data` (полный). Платформа:
- Парсит JSON-пейлоад
- Сохраняет в БД с временными метками
- Транслирует через WebSocket на дашборд
- Публикует в шину правил для внешних плагинов / Node-RED

**Запрос телеметрии** через REST:

```
GET /api/v1/devices/{id}/telemetry?keys=temperature,humidity&from=...&to=...&limit=100
GET /api/v1/devices/{id}/telemetry/latest
```

**Хранение**: телеметрия старше `TELEMETRY_RETENTION_DAYS` (по умолчанию 30) автоматически очищается каждый час.

### Атрибуты

- **Reported** — устройство сообщает платформе текущее состояние
- **Desired** — платформа указывает устройству желаемое состояние

Установить desired-атрибуты через страницу устройства или REST:

```bash
PATCH /api/v1/devices/{id}/attributes/desired
{ "targetTemperature": 22, "mode": "auto" }
```

Платформа сохраняет и публикует через MQTT `v1/me/a/upd`.

---

## RPC (удалённые вызовы)

RPC-консоль на странице устройства позволяет вызывать любой метод, поддерживаемый устройством.

1. Дашборд отправляет `POST /api/v1/devices/{id}/rpc` с методом и параметрами
2. Платформа публикует MQTT downlink на `v1/me/r/req`
3. Устройство обрабатывает и отвечает на `v1/me/r/res`
4. Платформа сопоставляет ответ по корреляционному `id`

### Встроенные методы

- **`rpc-list`** — возвращает полный каталог методов устройства
- **`reboot`** — перезагрузка устройства

---

## OTA-обновления

### Загрузка прошивки

1. Страница **OTA**
2. Выбрать продукт, ввести версию (напр. `1.2.0`), опционально имя модуля
3. Выбрать `.bin` файл (макс. 32 МБ)
4. Нажать **Upload**

Платформа генерирует публичный URL:

```
{PUBLIC_BASE_URL}/ota/fw/{token}.bin
```

### Уведомление устройств

- **Notify product** — отправка OTA всем устройствам продукта
- **Notify single device** — одному устройству

Устройство получает URL через MQTT `v1/me/o/upd` и начинает загрузку.

### Auto-OTA

Включить **Auto-OTA** для продукта. При включении:
- Устройство подключается → платформа проверяет версию → отправляет обновление
- Устройство сообщает `ota/version` → аналогичная проверка
- Устройство было офлайн при уведомлении → отложенная задача доставляется при переподключении

---

## Автоматизация и плагины

### Шина правил (Node-RED)

Платформа публикует нормализованные JSON-события:

```
platform/v1/events/{account_slug}/{product_id}/{device_id}/telemetry
platform/v1/events/{account_slug}/{product_id}/{device_id}/status
platform/v1/events/{account_slug}/{product_id}/{device_id}/event
platform/v1/events/{account_slug}/{product_id}/{device_id}/attributes/reported
platform/v1/events/{account_slug}/{product_id}/{device_id}/rpc/result
platform/v1/events/{account_slug}/{product_id}/{device_id}/ota
platform/v1/events/{account_slug}/{product_id}/{device_id}/log
```

Внешние плагины (включая Node-RED) подписываются на события своего аккаунта (`platform/v1/events/{account_slug}/#`) и могут публиковать команды обратно:

```
platform/v1/commands/{account_slug}/{product_id}/{device_id}/rpc
platform/v1/commands/{account_slug}/{product_id}/{device_id}/attributes/desired
platform/v1/commands/{account_slug}/{product_id}/{device_id}/ota
```

Отдельных кредов для Node-RED нет: зарегистрируйте его в **Settings → External plugins** и используйте выданные MQTT-логин `extplg_{account_slug}_{id}` и пароль в MQTT-узле Node-RED. Подробнее — [06-automation](docs/ru/06-automation.md).

> Команды **обязаны** содержать slug собственного аккаунта плагина: ACL брокера отклоняет чужие slug, а целевое устройство ищется только внутри этого аккаунта. Старая форма без slug `commands/{product_id}/{device_id}/…` больше не принимается.

### Встроенные плагины

Плагины — **внутрипроцессные** автоматизации для каждого устройства (не Node-RED).

#### LED-гирлянда

Управление WS2812-лентами:
- Эффекты: `off`, `white`, `warm`, `red`, `green`, `blue`, `blink`, `strobe`, `breathe`, `chase`, `comet`, `scanner`, `fill`, `theater`, `sparkle`, `fire`, `rainbow`, `police`, `christmas`
- **Тихие часы** — принудительное `off` или приглушение ночью
- **Активное окно** — лента выключена вне настроенного диапазона времени

#### Расписание

Вызов RPC устройства в настраиваемое локальное время:
- Установить время и дни недели (напр. перезагрузка в 04:00 по будням)
- С учётом часовой зоны (`TIMEZONE` или переопределение для каждого binding)

#### Prometheus-экспортер и дашборд метрик

- **prometheus-exporter** — отдельный HTTP-листенер (по умолчанию `:9100`, опционально bearer-токен) со счётчиками устройств / телеметрии / RPC с меткой аккаунта
- **metrics-dashboard** — дашборд устройства: графики телеметрии, индикаторы, история RPC, таймлайн онлайна

Плагины каталога включаются и выключаются для каждого аккаунта (страница **Account**, `/api/v1/plugins/policies`).

### Внешние плагины

Внешние плагины работают как **отдельные процессы** (любой язык) и общаются с платформой по **MQTT** (шина правил): подписываются на события своего аккаунта и публикуют команды. Транспорты webhook / polling / gRPC из [08-external-plugins](docs/ru/08-external-plugins.md) запланированы, но не реализованы.

Создать внешний плагин: **Settings → External Plugins**. Платформа выдаёт MQTT-креды (`extplg_{account_slug}_{id}` + пароль, показывается один раз). Пишите на Python, Go, Java, C++ — любом языке с MQTT-клиентом. Примеры: [examples/plugins/](examples/plugins/).

---

## MCP — интеграция с LLM / AI

Платформа предоставляет **MCP (Model Context Protocol)** эндпоинт на `/mcp`. Любой MCP-совместимый AI-клиент (Cursor, Claude и др.) может взаимодействовать с устройствами.

### Настройка

1. **Settings** → создать API-токен (напр. имя: `mcp`)
2. Скопировать `cip_…` токен (показывается один раз)
3. Настроить AI-клиент:

```json
{
  "mcpServers": {
    "cogitor-iot": {
      "url": "http://192.168.1.22:8080/mcp",
      "headers": {
        "Authorization": "Bearer cip_…"
      }
    }
  }
}
```

### Доступные инструменты

| Инструмент | Назначение |
|------------|-----------|
| `overview` | Счётчики онлайн/офлайн, статус MQTT |
| `list_devices` | Фильтрация по продукту / статусу / запросу |
| `get_device` | Полная информация (телеметрия, атрибуты, схема) |
| `list_products` / `get_product` | Схемы продуктов |
| `list_device_methods` | Каталог RPC-методов |
| `invoke_rpc` | Вызов любого метода устройства |
| `reboot_device` | Быстрая перезагрузка |
| `get_telemetry` | Последние сэмплы телеметрии |
| `get_attributes` | Reported + desired атрибуты |
| `set_desired_attributes` | Отправка desired-атрибутов |

Операции записи (`invoke_rpc`, `reboot_device`, `set_desired_attributes`) требуют роли **operator** или **admin**.

> MCP-инструменты ограничены активным аккаунтом токена (членство и статус аккаунта проверяются на каждый запрос); инструменты записи требуют роль `operator` или `admin` в этом аккаунте.

---

## Аутентификация и пользователи

### Методы входа

- **Пароль** — логин по username или email + пароль
- **Google OAuth** — вход / регистрация через Google
- **Саморегистрация** — создаёт пользователя **и личный аккаунт**, в котором он admin (включена по умолчанию; для приватных инсталляций задайте `REGISTER_ENABLED=false`)

### Роли

Роли назначаются **в рамках аккаунта** (membership): один пользователь может быть admin в одном аккаунте и viewer в другом.

| Роль | Чтение | Запись (устройства, RPC, OTA) | Админ (участники, приглашения, политики) |
|------|--------|-------------------------------|-------------------------------|
| `admin` | да | да | да |
| `operator` | да | да | нет |
| `viewer` | да | нет | нет |

### API-токены

Долгоживущие токены для скриптов и MCP:
- Создать в **Settings** → указать имя → скопировать `cip_…` (показывается один раз)
- Использовать как `Authorization: Bearer cip_…`
- Отозвать в любой момент на той же странице

---

## Конфигурация

Приоритет конфигурации (первый непустой выигрывает): **OS env → `./config.yml` → `/opt/iot-platform/etc/config.yml`**. Мастер установки записывает `config.yml` (в нём секреты — держите `chmod 600`). Бинарник **не читает** `.env`; `.env` используется только Docker Compose / systemd `EnvironmentFile`.

### Основные переменные

| Переменная | По умолчанию | Описание |
|------------|-------------|----------|
| `HTTP_ADDR` | `:8080` | Адрес HTTP |
| `MQTT_ADDR` | `:1883` | MQTT TCP |
| `MQTT_WS_ADDR` | `:8083` | MQTT over WebSocket. `off` для отключения |
| `MQTT_TLS_ADDR` | `:8883` | MQTT over TLS (требует cert + key) |
| `MQTT_TLS_CERT` | | Путь к TLS-сертификату (включает TLS при указании с ключом) |
| `MQTT_TLS_KEY` | | Путь к приватному ключу TLS |
| `MQTT_TLS_CA` | | Путь к CA-сертификату (включает mutual TLS) |
| `MQTT_PUBLIC_PORT` / `MQTT_WS_PUBLIC_PORT` / `MQTT_TLS_PUBLIC_PORT` | порт слушателя | Порт для устройств, если хост пробрасывает другой (Docker `-p 11883:1883`) |
| `SQLITE_PATH` | `/opt/iot-platform/lib/platform.db` | Файл БД SQLite (legacy, используйте DATABASE_DSN) |
| `DATABASE_DRIVER` | `sqlite` | Драйвер БД: `sqlite` или `postgres` |
| `DATABASE_DSN` | (auto) | DSN базы данных (напр. `postgres://user:pass@host/db?sslmode=disable`) |
| `DB_MAX_OPEN_CONNS` | `8` | Читается, но пока не применяется (SQLite — 1, PostgreSQL — 8) |
| `DATA_DIR` | (auto) | Каталог данных (по умолчанию — каталог БД для SQLite, `./data` для Postgres) |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Публичный URL для OTA, email, OAuth. **Доступен с устройств** |
| `PLATFORM_NAME` | `Cogitor IoT` | Отображаемое имя платформы |
| `TIMEZONE` | `UTC` | Часовая зона IANA для расписаний и тихих часов |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `JWT_SECRET` | (dev) | Ключ JWT — **сменить в production** |
| `ADMIN_LOGIN` / `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin` / `admin@cogitor.local` / `admin` | Используются только мастером установки; существующая БД не пересоздаётся |
| `TELEMETRY_RETENTION_DAYS` | `30` | Период хранения телеметрии |
| `RPC_DEFAULT_TIMEOUT_MS` | `30000` | Таймаут ожидания ответа RPC |
| `BROKER_HOST_PUBLIC` | `localhost` | MQTT-хост для credentials устройств |
| `REGISTER_ENABLED` | `true` | Разрешить саморегистрацию (отключайте на приватных инсталляциях) |
| `REGISTER_DEFAULT_ROLE` | `admin` | Роль в личном аккаунте зарегистрировавшегося |
| `REQUIRE_EMAIL_VERIFY` | `false` | Требовать подтверждение email |
| `RULES_BUS_ENABLED` | `true` | Шина событий/команд для внешних плагинов и Node-RED |
| `RULES_BUS_PREFIX` | `platform/v1` | Префикс топиков шины правил |
| `LAB_ALLOW_UNKNOWN_DEVICES` | `false` | Зарезервировано, сейчас не используется |
| `BOOTSTRAP_ACCOUNT_SLUG` / `BOOTSTRAP_ACCOUNT_NAME` | `default` / `Default` | Первый аккаунт, создаваемый мастером установки |

---

## Развёртывание

### Docker

Образ платформы собирается из корневого `Dockerfile` (см. [Запуск через Docker](#2-запуск-через-docker)). Тома образа: `/opt/iot-platform/lib` (SQLite `platform.db` + прошивки) и `/opt/iot-platform/etc` (`config.yml` от мастера установки + сертификаты); порты контейнера `8080 1883 8083 8883`.

`docker-compose.yml` в корне репозитория запускает платформу (образ `darvik80/iot-platform:latest` или локальная сборка, порты и папки хоста — из `.env`, необязательные настройки — из `platform.env`) и, с `--profile nodered`, Node-RED на `:1880`:

```bash
cp .env.example .env
docker compose up -d            # или: docker compose up -d --build
```

Подробности, деплой через Gitea CI и переход со старой раскладки `/data` — [docs/ru/13-deploy-docker.md](docs/ru/13-deploy-docker.md).

### Bare Metal (разработка)

```bash
make build
./bin/platform.exe
```

### VPS (`iot.cogitor.dev`)

Production — **systemd** сервис (без Docker). CI: [docs/ru/12-deploy-vds.md](docs/ru/12-deploy-vds.md).

```bash
make build-linux
# scp + systemctl restart iot-platform
```

### Проверки здоровья

| Эндпоинт | Назначение |
|----------|-----------|
| `GET /healthz` | Liveness probe (без авторизации) |
| `GET /readyz` | DB + MQTT ready (без авторизации); содержит `mqtt_clients` |
| `GET /metrics` | Метрики платформы (сейчас **без авторизации** — ограничьте на reverse proxy) |

---

## MQTT-протокол

Платформа **является** MQTT-брокером. Устройства и Node-RED подключаются к `MQTT_ADDR` (по умолчанию `:1883`), MQTT-over-WebSocket на `MQTT_WS_ADDR` (по умолчанию `:8083`) или MQTT over TLS на `MQTT_TLS_ADDR` (по умолчанию `:8883`, см. [05-mqtt-tls](docs/ru/05-mqtt-tls.md)).

### Стили топиков

Новые устройства по умолчанию используют `short`; стиль **автоопределяется** по первому пакету устройства:

| Префикс | Стиль |
|---------|-------|
| `iot/v1/{product}/{device}/…` | Полный |
| `v1/me/…` или `v1/me/t`, `v1/me/r/req`, … | Короткий |

### Основные топики (короткий и полный стиль)

Топики полного стиля начинаются с `iot/v1/{product}/{device}/` (в таблице — `…/`). Короткие аплинк-топики принимаются и как `v1/me/…`, и как `v1/…`; короткий даунлинк платформа всегда шлёт в `v1/me/…`.

| Назначение | Напр. | Короткий | Полный |
|------------|-------|----------|--------|
| Статус (LWT — онлайн/офлайн) | up | `v1/me/s` | `…/up/status` |
| Телеметрия | up | `v1/me/t` | `…/up/telemetry/data` |
| События устройства | up | `v1/me/e` | `…/up/events/post` |
| Отчёт об атрибутах | up | `v1/me/a` | `…/up/attributes/report` |
| Запрос desired-атрибутов | up | `v1/me/a/req` | `…/up/attributes/request` |
| Ответ с desired-атрибутами | down | `v1/me/a/res` | `…/down/attributes/response` |
| Обновление desired-атрибутов | down | `v1/me/a/upd` | `…/down/attributes/update` |
| RPC-запрос (платформа → устройство) | down | `v1/me/r/req` | `…/down/rpc/request` |
| RPC-ответ (устройство → платформа) | up | `v1/me/r/res` | `…/up/rpc/response` |
| RPC-запрос (устройство → платформа) | up | `v1/me/r/creq` | `…/up/rpc/request` |
| RPC-ответ (платформа → устройство) | down | `v1/me/r/cres` | `…/down/rpc/response` |
| Запрос NTP | up | `v1/me/n/req` | `…/up/ntp/request` |
| Ответ NTP | down | `v1/me/n/res` | `…/down/ntp/response` |
| Версия OTA | up | `v1/me/o/ver` | `…/up/ota/version` |
| Запрос OTA (pull) | up | `v1/me/o/q` | `…/up/ota/query` |
| Уведомление OTA (URL) | down | `v1/me/o/upd` | `…/down/ota/update` |
| Отмена OTA | down | `v1/me/o/can` | `…/down/ota/cancel` |
| Прогресс OTA | up | `v1/me/o/p` | `…/up/ota/progress` |
| Лог-сообщения устройства | up | `v1/me/l` | `…/up/logs/report` |

### Аутентификация устройства

1. Дашборд создаёт устройство → генерирует токен доступа → в БД хранится SHA-256
2. MQTT CONNECT: `username={product}.{device}`, `password=token`
3. ACL разрешает устройству его топики `iot/v1/{product}/{device}/…` и дерево короткого стиля `v1/…`
4. Ротация токена разрывает текущую сессию; устройство должно переподключиться с новым токеном

> Даунлинк-топики короткого стиля (`v1/me/…`) у всех устройств одинаковые, но платформа доставляет каждое сообщение напрямую в сессию целевого устройства. Публикации устройство→устройство брокер не пересылает: сообщения устройств получает только платформа.

### Мониторинг

Страница **MQTT** показывает:
- Статус брокера (listening / down)
- Подключённых клиентов (тип, username, IP, подписки)
- Ленту событий брокера в реальном времени (auth, connect/disconnect, ACL deny, ingest, publish)

![Страница MQTT](docs/images/dashboard-mqtt.png)

---

## Документация

Полный указатель — [docs/README_ru.md](docs/README_ru.md).

| Документ | RU | EN |
|----------|----|----|
| Требования к продукту (PRD) | [01-prd.md](docs/ru/01-prd.md) | [01-prd.md](docs/en/01-prd.md) |
| REST API | [02-api.md](docs/ru/02-api.md) | [02-api.md](docs/en/02-api.md) |
| Аутентификация | [03-auth.md](docs/ru/03-auth.md) | [03-auth.md](docs/en/03-auth.md) |
| MQTT-протокол | [04-mqtt.md](docs/ru/04-mqtt.md) | [04-mqtt.md](docs/en/04-mqtt.md) |
| MQTT over TLS | [05-mqtt-tls.md](docs/ru/05-mqtt-tls.md) | [05-mqtt-tls.md](docs/en/05-mqtt-tls.md) |
| Автоматизация / Node-RED | [06-automation.md](docs/ru/06-automation.md) | [06-automation.md](docs/en/06-automation.md) |
| Встроенные плагины | [07-internal-plugins.md](docs/ru/07-internal-plugins.md) | [07-internal-plugins.md](docs/en/07-internal-plugins.md) |
| Внешние плагины | [08-external-plugins.md](docs/ru/08-external-plugins.md) | [08-external-plugins.md](docs/en/08-external-plugins.md) |
| Схема продукта | [09-product-schema.md](docs/ru/09-product-schema.md) | [09-product-schema.md](docs/en/09-product-schema.md) |
| MCP (LLM-инструменты) | [10-mcp.md](docs/ru/10-mcp.md) | [10-mcp.md](docs/en/10-mcp.md) |
| Эксплуатация | [11-operations.md](docs/ru/11-operations.md) | [11-operations.md](docs/en/11-operations.md) |
| Развёртывание VDS | [12-deploy-vds.md](docs/ru/12-deploy-vds.md) | [12-deploy-vds.md](docs/en/12-deploy-vds.md) |
| Развёртывание Docker | [13-deploy-docker.md](docs/ru/13-deploy-docker.md) | [13-deploy-docker.md](docs/en/13-deploy-docker.md) |
| Диаграммы | [14-diagrams.md](docs/ru/14-diagrams.md) | [14-diagrams.md](docs/en/14-diagrams.md) |
| Multi-Account и мониторинг (single v1.x) | [16-multi-account.md](docs/ru/16-multi-account.md) | [16-multi-account.md](docs/en/16-multi-account.md) |
| HA / Clustering (v2.x) | [17-ha-architecture.md](docs/ru/17-ha-architecture.md) | [17-ha-architecture.md](docs/en/17-ha-architecture.md) |

---

## Чек-лист приёмки

- [ ] Устройство подключается с токеном и появляется **online** в дашборде
- [ ] Телеметрия видна на странице устройства и обновляется через WebSocket
- [ ] Desired-атрибут достигает устройства (`v1/me/a/upd`)
- [ ] RPC round-trip с совпадающим `id` и `code: 0`
- [ ] NTP-запрос получает `serverRecvTime` / `serverSendTime`
- [ ] Ротация токена отключает старую сессию; новый токен работает
- [ ] Node-RED получает `platform/v1/events/{account}/.../telemetry`
- [ ] Auto-OTA отправляет прошивку при включении и переподключении устройства

---

## Лицензия

Проприетарная — Cogitor.
