# Эксплуатация

## Конфигурационные файлы

Приоритет (первый непустой выигрывает): **OS env → `.env` → `./config.yml` → `/etc/iot-platform-go/config.yml`**.

YAML: `server.public_base_url`, `server.http_addr`, `mqtt.addr`. `.env` / compose: `PUBLIC_BASE_URL`, `HTTP_ADDR` (побеждают yaml). Полные примеры: `config.yml.example`, `.env.example`.

## Локальный запуск (разработка)

```bash
# Терминал 1 — API + встроенный MQTT
export SQLITE_PATH=./data/platform.db
go run ./cmd/platform

# Терминал 2 (опционально) — Node-RED
cd deploy && docker compose up -d nodered
```

Дашборд: http://localhost:8080
MQTT: `mqtt://localhost:1883`

Если ESP32 выдаёт `transport_read(): EOF` / `mqtt_message_receive() returned -2`, почти наверняка другой брокер уже занял `:1883` (RabbitMQ MQTT, Mosquitto, Windows WinNAT). Платформа теперь эксклюзивно биндится на IPv4 `0.0.0.0:1883` и откажется стартовать, если порт занят. Остановить другой слушатель или установить `MQTT_ADDR=:1884`.

## SQLite

- Путь: `SQLITE_PATH` (по умолчанию `./data/platform.db`)
- Режим WAL включён
- Ежечасная очистка телеметрии / событий / логов старше `TELEMETRY_RETENTION_DAYS`

Бэкап: остановить запись или использовать `sqlite3 .backup` на checkpointed-копии.

## Здоровье

- `GET /healthz` — процесс жив
- `GET /readyz` — MQTT-слушатель готов

## OTA

1. На **OTA** загрузить `.bin` (продукт + версия). Платформа сохраняет в `{SQLITE_PATH dir}/firmware` (или `OTA_DIR`) и генерирует публичный URL `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`. ESP скачивает этот URL **без JWT**. Тот же продукт+модуль+версия заменяет образ. Альтернативно — зарегистрировать внешний HTTPS URL.
2. **Notify product** — MQTT `v1/o/upd` каждому устройству продукта.
3. **Notify device** — то же, одному устройству.
4. Включить **Auto-OTA** на продукте (устройство может отключить). Новая прошивка + переподключение / отчёт версии — автоотправка.
5. Прогресс устройства приходит на `v1/o/p`; задачи показывают статус на OTA-странице.

`PUBLIC_BASE_URL` должен быть доступен с устройства (NAS/LAN IP, не `localhost`).

## Часовая зона

`TIMEZONE` (IANA, напр. `Asia/Bangkok`) в `.env` / `config.yml`. Используется расписанием и тихими часами LED. По умолчанию `UTC`. Плагин `config.tz` переопределяет для каждого устройства.

## Ротация токена

1. Оператор ротирует токен в UI/API.
2. Новый хеш сохраняется; живая MQTT-сессия для `{product}.{device}` отключается.
3. Прошивка устройства должна быть обновлена новым токеном.

## Логирование

Структурированные логи в **stdout** (`slog`). Docker: `docker compose logs -f platform` / `docker logs -f cogitor-platform`. HTTP access log (chi) идёт в stderr.

`LOG_LEVEL` (`debug` | `info` | `warn` | `error`, по умолчанию `info`). Телеметрия ingest — **debug**. Connect / ACL / RPC / OTA остаются info.

```bash
# compose
LOG_LEVEL=debug docker compose up -d
docker compose logs -f platform
```

Дашборд → **MQTT** показывает живые сессии и ленту событий брокера (`GET /api/v1/mqtt`).

## Production-заметки

- Сменить `JWT_SECRET`, `ADMIN_PASSWORD`
- Предпочитать MQTT TLS (front proxy / listener) и HTTPS перед `:8080`
- Не экспортировать MQTT `:1883` в публичный интернет без auth + firewall
- Если Synology (или другой хост) всё ещё держит RabbitMQ MQTT на `:1883`, остановить или привязать платформу к другому порту (`MQTT_ADDR`)
- Включить Node-RED `adminAuth` в `deploy/nodered/settings.js`
