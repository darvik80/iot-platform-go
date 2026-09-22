# Эксплуатация

Руководство по запуску, конфигурации и обслуживанию платформы.

---

## Конфигурация

Приоритет (первый непустой выигрывает): **OS env → `./config.yml` → `/etc/iot-platform/config.yml`**.

YAML-ключи: `server.public_base_url`, `server.http_addr`, `mqtt.addr`. Переменные окружения (`.env` / compose) переопределяют YAML.

Полные примеры: `config.yml.example`, `.env.example`.

### Ключевые переменные

| Переменная | По умолчанию | Назначение |
|-----------|-------------|-----------|
| `HTTP_ADDR` | `:8080` | HTTP-адрес (дашборд + API) |
| `MQTT_ADDR` | `:1883` | MQTT TCP-слушатель |
| `SQLITE_PATH` | `/var/lib/iot-platform/platform.db` | Файл базы данных SQLite |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Публичный URL (OTA, email, OAuth) |
| `JWT_SECRET` | dev-значение | Ключ подписи JWT — **сменить в production** |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@cogitor.local` / `admin` | Начальный администратор |
| `TIMEZONE` | `UTC` | Часовая зона для расписаний и тихих часов |
| `TELEMETRY_RETENTION_DAYS` | `30` | Автоочистка старых данных |
| `LOG_LEVEL` | `info` | Уровень логов: `debug` \| `info` \| `warn` \| `error` |

---

## Локальный запуск

```bash
# Терминал 1 — платформа (API + MQTT)
export SQLITE_PATH=/var/lib/iot-platform/platform.db
go run ./cmd/platform

# Терминал 2 (опционально) — Node-RED
cd deploy && docker compose up -d nodered
```

- Дашборд: http://localhost:8080
- MQTT: `mqtt://localhost:1883`

### Конфликт порта 1883

Если ESP32 выдаёт `transport_read(): EOF` / `mqtt_message_receive() returned -2` — другой процесс занял `:1883` (RabbitMQ, Mosquitto, Windows WinNAT). Платформа биндится на `0.0.0.0:1883` и откажется стартовать, если порт занят. Решение: остановить другой слушатель или установить `MQTT_ADDR=:1884`.

---

## База данных

SQLite с режимом WAL (параллельное чтение при записи).

- Путь: `SQLITE_PATH` (по умолчанию `/var/lib/iot-platform/platform.db`)
- Ежечасная очистка телеметрии, событий и логов старше `TELEMETRY_RETENTION_DAYS`
- Миграции в `internal/db/migrations/` — автоприменение при старте

**Бэкап**: остановить запись или использовать `sqlite3 .backup` на checkpointed-копии.

---

## Здоровье

| Эндпоинт | Назначение |
|----------|-----------|
| `GET /healthz` | Процесс жив (liveness probe) |
| `GET /readyz` | MQTT-слушатель + DB готовы (readiness probe) |

Оба доступны без авторизации — для Kubernetes / Docker healthcheck.

---

## OTA-обновления

1. Загрузить `.bin` на странице **OTA** (или зарегистрировать внешний URL)
2. Платформа сохраняет в `{SQLITE_PATH dir}/firmware` и генерирует публичный URL: `{PUBLIC_BASE_URL}/ota/fw/{token}.bin`
3. ESP скачивает прошивку **без JWT** по HTTP
4. Уведомить устройство или весь продукт → MQTT `v1/o/upd`
5. Включить **Auto-OTA** на продукте — автоотправка при переподключении

`PUBLIC_BASE_URL` должен быть доступен с устройства (NAS/LAN IP, не `localhost`).

---

## Часовая зона

`TIMEZONE` (IANA, напр. `Asia/Bangkok`) — для расписаний и тихих часов LED-гирлянды. По умолчанию `UTC`. Плагин может переопределить через `config.tz`.

---

## Ротация токена устройства

1. Оператор ротирует токен в UI или API
2. Новый хеш сохраняется в SQLite; живая MQTT-сессия для `{product}.{device}` отключается
3. Прошивка устройства должна быть обновлена новым токеном

---

## Логирование

Структурированные логи в **stdout** (`slog`):

- `connect`, `disconnect`, `auth_fail`, `acl_deny`, `subscribe` — info
- `ingest` (кроме телеметрии), `command`, downlink `publish` — info
- Телеметрия ingest — **debug**

Docker: `docker compose logs -f platform`

```bash
# Включить debug-логи
LOG_LEVEL=debug docker compose up -d
docker compose logs -f platform
```

Дашборд → **MQTT** показывает живые сессии и ленту событий брокера.

---

## Production-чеклист

- [ ] Сменить `JWT_SECRET` на случайное значение
- [ ] Сменить `ADMIN_PASSWORD`
- [ ] Установить `PUBLIC_BASE_URL` на реальный хост (не `localhost`)
- [ ] Включить MQTT TLS или reverse proxy с HTTPS
- [ ] Не экспортировать MQTT `:1883` в публичный интернет без firewall
- [ ] Включить `adminAuth` в Node-RED (`deploy/nodered/settings.js`)
- [ ] Настроить бэкап SQLite
