# Эксплуатация

Руководство по запуску, конфигурации и обслуживанию платформы.

---

## Конфигурация

Приоритет (первый непустой выигрывает): **OS env → `./config.yml` → `/opt/iot-platform/etc/config.yml`**. Расширение `.yaml` тоже принимается (если есть оба файла, побеждает `config.yml`). Строка лога при старте `config sources files=…` показывает, какие файлы применены; нечитаемый или некорректный файл пишется в лог как `config file ignored` с причиной.

YAML-ключи разворачиваются в имена переменных (`server.public_base_url` → `SERVER_PUBLIC_BASE_URL`, `mqtt.addr` → `MQTT_ADDR`). Переменные окружения переопределяют YAML.

Бинарник **не** читает файлы `.env` (загрузчика dotenv при старте нет). Файл `.env` работает, только если его передаёт в окружение процесса кто-то другой — Docker Compose `env_file:` или systemd `EnvironmentFile=`.

Полные примеры: `config.yml.example`, `.env.example`.

### Ключевые переменные

| Переменная | По умолчанию | Назначение |
|-----------|-------------|-----------|
| `HTTP_ADDR` | `:8080` | HTTP-адрес (дашборд + API) |
| `MQTT_ADDR` | `:1883` | MQTT TCP-слушатель |
| `MQTT_TLS_ADDR` | `:8883` | Слушатель MQTT over TLS (активен только при заданных `MQTT_TLS_CERT` + `MQTT_TLS_KEY`) |
| `MQTT_PUBLIC_PORT` / `MQTT_WS_PUBLIC_PORT` / `MQTT_TLS_PUBLIC_PORT` | порт слушателя | Порт, который сообщается клиентам (credentials устройств, Settings, статус MQTT), если он отличается от порта слушателя, например Docker `-p 11883:1883`. Слушатель не переносит |
| `BROKER_HOST_PUBLIC` | хост из `PUBLIC_BASE_URL` (мастер) / `localhost` | MQTT-хост для устройств (TCP и TLS). URL или `host:port` сводится к хосту. За Cloudflare используйте отдельное DNS-only имя — см. [12-deploy-vds](12-deploy-vds.md#cloudflare) |
| `MQTT_WS_PUBLIC_URL` | — | Полный URL MQTT over WebSocket для клиентов, например `wss://iot.example.com/mqtt` за nginx. Пусто = `ws://{BROKER_HOST_PUBLIC}:{WS-порт}/` |
| `DATABASE_DRIVER` | `sqlite` | `sqlite` \| `postgres` |
| `DATABASE_DSN` | `/opt/iot-platform/lib/platform.db` | Путь к файлу SQLite или URL PostgreSQL (`postgres://user:pass@host:5432/db?sslmode=disable`) |
| `SQLITE_PATH` | `/opt/iot-platform/lib/platform.db` | Устаревшая: файл SQLite, если `DATABASE_DSN` пуст |
| `DATA_DIR` | каталог файла SQLite (`./data` для Postgres) | Базовый каталог данных (прошивки и т. п.) |
| `OTA_DIR` | `{DATA_DIR}/firmware` | Каталог хранения прошивок |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Публичный URL (OTA, email, OAuth) |
| `JWT_SECRET` | — | Ключ подписи JWT. Если не задан или оставлен dev-значением, платформа берёт `settings.jwt_secret` из БД либо генерирует случайный и сохраняет его туда (с предупреждением в логе) |
| `TIMEZONE` | `UTC` | Часовая зона для расписаний и тихих часов |
| `TELEMETRY_RETENTION_DAYS` | `30` | Автоочистка старых данных |
| `LOG_LEVEL` | `info` | Уровень логов: `debug` \| `info` \| `warn` \| `error` |

`ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_LOGIN` и `BOOTSTRAP_ACCOUNT_*` **не** создают администратора или аккаунт — используйте мастер настройки (ниже).

`DATABASE_MAX_OPEN_CONNS` (алиас `DB_MAX_OPEN_CONNS`) читается, но сейчас **не применяется**: размер пула захардкожен в `internal/db/db.go` (SQLite — 1 соединение, PostgreSQL — 8 открытых / 2 простаивающих).

---

## Первый запуск: мастер настройки

Если настроенная БД пуста (нет ни пользователей, ни аккаунтов) — или БД не настроена — платформа не стартует в обычном режиме, а поднимает на `HTTP_ADDR` только **мастер настройки** (setup wizard):

- Веб-UI: `http://<host>:8080/setup` (SPA перенаправляет туда автоматически)
- API: `GET /api/v1/setup/state`, `POST /api/v1/setup/step`, `POST /api/v1/setup/apply`, `POST /api/v1/setup/generate-tls`

Шаги: **база данных** (драйвер + DSN) → **администратор** (логин, email, пароль) → **аккаунт** (slug, имя) → **SMTP** → **JWT secret** (случайное значение сгенерировано заранее) → **MQTT TLS** (пути или генерация самоподписанного CA + серверного сертификата) → **публичный URL** (`PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`) → **применение**.

При применении мастер создаёт администратора, аккаунт по умолчанию и membership администратора, сохраняет настройки в таблицу `settings` и пишет `config.yml` в `/opt/iot-platform/etc/config.yml`, если каталог существует и доступен на запись, иначе в `./config.yml`. Затем процесс перечитывает конфиг (значения из записанного файла перекрывают env) и продолжает запуск полной платформы — перезапуск не нужен.

При старте конфигурация берётся из env → `./config.yml` → `/opt/iot-platform/etc/config.yml`; из таблицы `settings` читается только `jwt_secret` (если настоящий секрет не задан). `config.yml` пишется с правами `0600`. Если в логе есть `failed to write config.yml`, перенесите как минимум `database.*` в файл конфига или env вручную. Также проверьте, что DSN, выбранный на шаге 1, доступен на запись пользователю сервиса (мастер подставляет `/opt/iot-platform/lib/platform.db`).

**Логина по умолчанию нет** (`admin/admin` или `admin@cogitor.local` на свежей установке не существует).

> **Безопасность**: пока работает мастер, его эндпоинты **без авторизации** — кто первым откроет порт, тот и задаст пароль администратора. Запускайте мастер только в доверенной сети или через `localhost` / SSH-туннель и не публикуйте порт 8080, пока настройка не завершена. `GET /api/v1/setup/state` не отдаёт секреты (только флаги `admin_password_set` / `smtp_pass_set` / `jwt_secret_set` и DSN со скрытым паролем). После применения маршруты мастера удаляются, а `GET /api/v1/setup/state` отвечает `{"completed":true}`. `generate-tls` пишет только в каталог сертификатов по умолчанию или в относительный путь внутри рабочего каталога.
>
> Пользователь, созданный мастером, — **админ платформы** (`settings.platform_admin_user_id`): только он может смотреть и удалять всех пользователей. Самостоятельно зарегистрированные пользователи — админы только своего личного аккаунта, независимо от `REGISTER_DEFAULT_ROLE`.

---

## Локальный запуск

```bash
# Терминал 1 — платформа (API + MQTT); пустая БД → мастер настройки на :8080
export DATABASE_DRIVER=sqlite
export DATABASE_DSN=./data/platform.db
go run ./cmd/platform

# Терминал 2 (опционально) — Node-RED
MQTT_BROKER_HOST=host.docker.internal docker compose up -d nodered
```

- Дашборд: http://localhost:8080
- MQTT: `mqtt://localhost:1883`

### Конфликт порта 1883

Если ESP32 выдаёт `transport_read(): EOF` / `mqtt_message_receive() returned -2` — другой процесс занял `:1883` (RabbitMQ, Mosquitto, Windows WinNAT). Платформа биндится на `0.0.0.0:1883` и откажется стартовать, если порт занят. Решение: остановить другой слушатель или установить `MQTT_ADDR=:1884`.

---

## База данных

Выбирается через `DATABASE_DRIVER`:

- **SQLite** (по умолчанию) — pure-Go драйвер, режим WAL, `busy_timeout=5000`, одно соединение. Файл: `DATABASE_DSN` (устаревшая `SQLITE_PATH`), по умолчанию `/opt/iot-platform/lib/platform.db`.
- **PostgreSQL** — `DATABASE_DRIVER=postgres`, `DATABASE_DSN=postgres://…` (если DSN пуст, используется `postgres://localhost:5432/iot_platform?sslmode=disable`).

Общее для обоих:

- Ежечасная очистка телеметрии, событий и логов старше `TELEMETRY_RETENTION_DAYS`
- Миграции в `internal/db/migrations/` — автоприменение при старте

**Бэкап**:

- SQLite: остановить запись или использовать `sqlite3 platform.db ".backup /backup/platform-$(date +%F).db"` (безопасно при WAL).
- PostgreSQL: `pg_dump -Fc -d "$DATABASE_DSN" -f /backup/iot-platform-$(date +%F).dump`; восстановление — `pg_restore -d <db> <file>`.
- Также сохраняйте `config.yml` (в нём `JWT_SECRET`), каталог прошивок и TLS-сертификаты MQTT.

---

## Здоровье

| Эндпоинт | Назначение |
|----------|-----------|
| `GET /healthz` | Процесс жив (liveness probe) |
| `GET /readyz` | MQTT-слушатель + DB готовы (readiness probe); во время работы мастера настройки возвращает `"status": "setup"` |

Оба доступны без авторизации — для Kubernetes / Docker healthcheck.

---

## OTA-обновления

1. Загрузить `.bin` на странице **OTA** (или зарегистрировать внешний URL)
2. Платформа сохраняет файл в `OTA_DIR` (по умолчанию `{DATA_DIR}/firmware`; `DATA_DIR` по умолчанию — каталог файла SQLite) и генерирует публичный URL: `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`
3. ESP скачивает прошивку **без JWT** по HTTP
4. Уведомить устройство или весь продукт → MQTT `v1/me/o/upd` (короткий стиль) или `iot/v1/{product}/{device}/down/ota/update` (полный стиль)
5. Включить **Auto-OTA** на продукте — автоотправка при переподключении

`PUBLIC_BASE_URL` должен быть доступен с устройства (NAS/LAN IP, не `localhost`).

---

## Часовая зона

`TIMEZONE` (IANA, напр. `Asia/Bangkok`) — для расписаний и тихих часов LED-гирлянды. По умолчанию `UTC`. Плагин может переопределить через `config.tz`.

---

## Ротация токена устройства

1. Оператор ротирует токен в UI или API
2. Новый хеш сохраняется в БД; живая MQTT-сессия для `{product}.{device}` отключается
3. Прошивка устройства должна быть обновлена новым токеном

---

## Логирование

Структурированные логи в **stdout** (`slog`):

- `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe` — info
- `ingest` (включая телеметрию), `command`, downlink `publish` — info

Docker: `docker logs -f iot-platform-go` (имя контейнера из `deploy-docker.yml`). Debian-пакет / systemd: `/var/log/iot-platform/platform.log`.

```bash
# Включить debug-логи (Docker)
docker run ... -e LOG_LEVEL=debug ...
docker logs -f iot-platform-go
```

Дашборд → **MQTT** показывает живые сессии и ленту событий брокера.

---

## Production-чеклист

- [ ] Пройти мастер настройки из доверенной сети, затем перезапустить сервис
- [ ] Задать надёжный пароль администратора (в мастере)
- [ ] Оставить случайный `JWT_SECRET`, сгенерированный мастером (никогда не dev-значение)
- [ ] Установить `PUBLIC_BASE_URL` на реальный хост (не `localhost`)
- [ ] Включить MQTT TLS или reverse proxy с HTTPS
- [ ] Не экспортировать MQTT `:1883` в публичный интернет без firewall
- [ ] Включить `adminAuth` в Node-RED (`deploy/nodered/settings.js`)
- [ ] Настроить бэкап БД (SQLite `.backup` или PostgreSQL `pg_dump`)
