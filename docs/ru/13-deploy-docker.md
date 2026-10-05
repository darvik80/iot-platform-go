# Деплой в Docker (Synology / compose)

Контейнерный деплой платформы — удобно для NAS (Synology, QNAP) или любого Docker-хоста. Многоэтапная сборка: Node.js → Go → Alpine (итоговый образ ~30 МБ).

Один и тот же образ можно запустить тремя способами:

- **Docker Compose** — `deploy/docker-compose.yml` (сервис `platform`, опционально `nodered`), порты и папки хоста в `deploy/.env`;
- **`docker run`** — на любом хосте;
- **Gitea CI** — `.gitea/workflows/deploy-docker.yml` пересобирает образ и пересоздаёт контейнер при push в `master` (раннер на Synology).

Продакшн на VPS работает через systemd — см. [12-deploy-vds.md](12-deploy-vds.md).

---

## Устройство образа

```
docker build -t localhost/iot-platform-go:latest .
         │
         ├── node:22-bookworm     → npm ci && npm run build  (React SPA)
         ├── golang:1.26-bookworm → go build                 (Go-бинарник + встроенный SPA)
         └── alpine:3.20          → /opt/iot-platform/bin/platform
                                    │
                              ┌─────┴──────┐
                              │  Контейнер │   пользователь "platform" (uid 1000)
                              │  :8080 HTTP│   через docker-entrypoint.sh (su-exec)
                              │  :1883 MQTT│
                              │  :8083 WS  │
                              │  :8883 TLS │
                              └─────┬──────┘
                                    │
                 <папка хоста>/
                 ├── data/ → /opt/iot-platform/lib   SQLite platform.db + firmware/
                 └── etc/  → /opt/iot-platform/etc   config.yml (мастер установки) + certs/
```

Что зашито в образ (`Dockerfile`, `docker-entrypoint.sh`):

| | Значение |
|---|---|
| Порты контейнера | `8080` HTTP, `1883` MQTT, `8083` MQTT over WebSocket, `8883` MQTT over TLS (`EXPOSE`) |
| Env слушателей | `HTTP_ADDR=:8080`, `MQTT_ADDR=:1883`, `MQTT_WS_ADDR=:8083`, `MQTT_TLS_ADDR=:8883` |
| База | `SQLITE_PATH=/opt/iot-platform/lib/platform.db` (DSN по умолчанию; мастер установки подставляет тот же путь) |
| Тома | `/opt/iot-platform/lib`, `/opt/iot-platform/etc` (`VOLUME`) |
| Здоровье | `HEALTHCHECK` → `GET /healthz` (отвечает и в режиме мастера установки) |
| Entrypoint | от root: `chown` каталогов `lib/` и `etc/` на uid 1000, затем `su-exec platform`; при `--user` запускает команду как есть, права на тома — ваша забота |

**Порты:** меняйте только **хостовую** сторону (`-p ХОСТ:КОНТЕЙНЕР`). Порты контейнера зафиксированы образом. Если порт хоста отличается от порта контейнера, сообщите платформе, какой порт использовать клиентам: `PUBLIC_BASE_URL` для HTTP и `MQTT_PUBLIC_PORT` / `MQTT_WS_PUBLIC_PORT` / `MQTT_TLS_PUBLIC_PORT` для MQTT (они показываются в credentials устройств, в Settings и в статусе MQTT). Compose и CI-воркфлоу выставляют MQTT-порты автоматически.

**Папки:** оба bind-монтирования могут принадлежать root на хосте — entrypoint при каждом старте отдаёт их uid 1000. `config.yml` содержит секреты и пишется с правами `0600`.

---

## Docker Compose

```bash
cd deploy
cp .env.example .env                   # порты, папки хоста, имя образа
cp platform.env.example platform.env   # опционально: закреплённые настройки платформы
docker compose up -d --build           # только платформа
docker compose --profile nodered up -d # + Node-RED
```

`deploy/.env` (только для подстановки в compose):

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `IOT_IMAGE` | `localhost/iot-platform-go:latest` | Запускаемый образ; `--build` помечает локальную сборку этим именем. Готовый образ: `darvik80/iot-platform:latest` (amd64 + arm64) |
| `IOT_CONTAINER_NAME` | `iot-platform-go` | Имя контейнера |
| `IOT_HTTP_PORT` | `8080` | Порт хоста → 8080 |
| `IOT_MQTT_PORT` | `1883` | Порт хоста → 1883, он же `MQTT_PUBLIC_PORT` |
| `IOT_MQTT_WS_PORT` | `8083` | Порт хоста → 8083, он же `MQTT_WS_PUBLIC_PORT` |
| `IOT_MQTT_TLS_PORT` | `8883` | Порт хоста → 8883, он же `MQTT_TLS_PUBLIC_PORT` |
| `IOT_DATA_DIR` | `./iot/data` | Папка хоста → `/opt/iot-platform/lib` |
| `IOT_ETC_DIR` | `./iot/etc` | Папка хоста → `/opt/iot-platform/etc` |
| `NODERED_PORT` | `1880` | Порт Node-RED на хосте (профиль `nodered`) |

`deploy/platform.env` передаётся через `env_file` и необязателен. Каждая переменная из него **перекрывает** `config.yml` мастера установки, поэтому оставляйте только то, что хотите закрепить (`PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`, `JWT_SECRET`, `DATABASE_*`, `MQTT_TLS_*`, …), и не оставляйте переменные с пустым значением: пустая переменная всё равно считается заданной и скрывает значение из `config.yml`.

---

## docker run

```bash
docker build -t iot-platform-go .
docker run -d --name iot-platform-go --restart unless-stopped \
  -p 8080:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v "$PWD/iot/data:/opt/iot-platform/lib" \
  -v "$PWD/iot/etc:/opt/iot-platform/etc" \
  iot-platform-go
```

Другие порты хоста, например HTTP на 9083 и MQTT на 11883:

```bash
docker run -d --name iot-platform-go --restart unless-stopped \
  -p 9083:8080 -p 11883:1883 -p 8083:8083 -p 8883:8883 \
  -e MQTT_PUBLIC_PORT=11883 \
  -e PUBLIC_BASE_URL=http://192.168.1.22:9083 \
  -e BROKER_HOST_PUBLIC=192.168.1.22 \
  -v /srv/iot/data:/opt/iot-platform/lib \
  -v /srv/iot/etc:/opt/iot-platform/etc \
  iot-platform-go
```

Бинарник не читает `.env`; используйте `-e VAR=…` или `--env-file <файл>`.

- Мастер установки (первый запуск): `http://<host>:<HTTP-порт>/setup`
- MQTT: `mqtt://<host>:<MQTT-порт>`, WS: `ws://<host>:<WS-порт>/`, TLS: `mqtts://<host>:<TLS-порт>` (только если заданы cert/key)

---

## Конфигурация

Приоритет: **OS env → `./config.yml` (= `/opt/iot-platform/config.yml`, не на томе) → `/opt/iot-platform/etc/config.yml`**. Полный список переменных — [11-operations.md](11-operations.md#конфигурация).

### Первый запуск — мастер установки

При пустой базе контейнер отдаёт **мастер установки** на HTTP-порту (`/setup`, API `/api/v1/setup/*`) — логина администратора по умолчанию нет, переменные `ADMIN_*` игнорируются. Подробнее: [Эксплуатация → мастер установки](11-operations.md).

- На шаге 1 оставьте предложенный SQLite DSN (`/opt/iot-platform/lib/platform.db`) — он лежит на томе данных.
- Мастер пишет `config.yml` в `/opt/iot-platform/etc/config.yml` (том etc) и продолжает запуск платформы; следующий старт контейнера прочитает его оттуда.
- На шаге 7 укажите публичный URL и хост брокера **со стороны хоста** (например, `http://192.168.1.22:9083`, `192.168.1.22`).
- Эндпоинты мастера не требуют авторизации: проходите его из доверенной сети.

### MQTT TLS

Положите файлы в `<папка etc на хосте>/certs/mqtt/` (или сгенерируйте мастером в `/opt/iot-platform/etc/certs/mqtt`) и задайте `MQTT_TLS_CERT` / `MQTT_TLS_KEY` (пути внутри контейнера: `/opt/iot-platform/etc/certs/mqtt/…`). См. [05-mqtt-tls.md](05-mqtt-tls.md).

---

## Synology NAS + Gitea CI

`.gitea/workflows/deploy-docker.yml` запускается на **push в `master`** и вручную (**workflow_dispatch**), одним джобом на раннере NAS (образ собирается там же, где работает контейнер):

1. Собирает `localhost/<контейнер>:latest` с `VERSION` (из `version.txt`) и `COMMIT`.
2. Пересоздаёт контейнер с `--restart unless-stopped`, публикует все четыре порта и монтирует `<IOT_HOST_DIR>/data` → `/opt/iot-platform/lib`, `<IOT_HOST_DIR>/etc` → `/opt/iot-platform/etc`. Отсутствующие папки Docker создаёт сам.
3. Ждёт, пока healthcheck контейнера станет `healthy` (≈90 с); иначе печатает последние 100 строк лога и падает.
4. Удаляет «висячие» образы.

Всё переопределяется **переменными репозитория** (Settings → Actions → Variables), править файл не нужно:

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `IOT_CONTAINER_NAME` | `iot-platform-go` | Имя контейнера и образа |
| `IOT_HTTP_PORT` | `9083` | Порт хоста → 8080 |
| `IOT_MQTT_PORT` | `1883` | Порт хоста → 1883 (+ `MQTT_PUBLIC_PORT`) |
| `IOT_MQTT_WS_PORT` | `8083` | Порт хоста → 8083 (+ `MQTT_WS_PUBLIC_PORT`) |
| `IOT_MQTT_TLS_PORT` | `8883` | Порт хоста → 8883 (+ `MQTT_TLS_PUBLIC_PORT`) |
| `IOT_HOST_DIR` | `/volume1/docker/iot-platform-go` | Содержит `data/` и `etc/` |
| `PUBLIC_BASE_URL`, `BROKER_HOST_PUBLIC`, `TIMEZONE` | — | Передаются как `-e` только если заданы; иначе действует `config.yml` |

Дашборд: `http://<nas-ip>:9083` · MQTT: `<nas-ip>:1883`.

`.gitea/workflows/deploy-docker-hub.yml` собирает тот же `Dockerfile` под `linux/amd64` + `linux/arm64` и публикует `darvik80/iot-platform` (`latest` из `master`, версионные теги из `v*`).

### Переход со старой раскладки `/data`

Старые образы хранили базу в `/data`, а конфиг в `/etc/iot-platform`. Папки на хосте те же (`data/`, `etc/`), поменялась только сторона контейнера. Entrypoint создаёт ссылки `/data` → `/opt/iot-platform/lib` и `/etc/iot-platform` → `/opt/iot-platform/etc`, поэтому существующий `config.yml` с `dsn: /data/platform.db` или путями сертификатов в `/etc/iot-platform/certs` продолжает работать. Позже эти пути можно заменить на новые.

### Ручная пересборка

```bash
export DOCKER_API_VERSION=1.43
docker build -t localhost/iot-platform-go:latest .
docker rm -f iot-platform-go || true
docker run -d --name iot-platform-go --restart unless-stopped \
  -p 9083:8080 -p 1883:1883 -p 8083:8083 -p 8883:8883 \
  -v /volume1/docker/iot-platform-go/data:/opt/iot-platform/lib \
  -v /volume1/docker/iot-platform-go/etc:/opt/iot-platform/etc \
  localhost/iot-platform-go:latest
```

---

## Обновление

Данные лежат в папках хоста, поэтому пересоздание контейнера безопасно:

```bash
cd deploy && docker compose up -d --build        # compose
# или: docker build … && docker rm -f iot-platform-go && docker run … (те же аргументы)
```

Перед крупными обновлениями сделайте копию `data/platform.db` (остановите контейнер или выполните на хосте `sqlite3 platform.db ".backup out.db"` — в образе `sqlite3` нет) и `etc/config.yml`.

---

## Проверка

```bash
docker ps --filter name=iot-platform-go          # в STATUS видно (healthy)
docker logs -f iot-platform-go
curl -fsS http://localhost:<HTTP-порт>/healthz
curl -fsS http://localhost:<HTTP-порт>/readyz     # 503, пока не поднят MQTT; "setup" в режиме мастера
docker exec iot-platform-go ls -l /opt/iot-platform/lib /opt/iot-platform/etc
```

---

## Конфликт порта 1883

Если на хосте 1883 занят другим брокером, опубликуйте MQTT на другом порту: `IOT_MQTT_PORT=1884` (compose / переменная Gitea) или `-p 1884:1883 -e MQTT_PUBLIC_PORT=1884` (`docker run`). Устройства подключаются к `mqtt://<host>:1884`, и именно этот порт платформа показывает в credentials устройств.

---

## Node-RED (опционально)

Сервис `nodered` в `deploy/docker-compose.yml`, запускается с `--profile nodered`:

- образ `nodered/node-red:4.0`, контейнер `cogitor-nodered`, порт `${NODERED_PORT:-1880}:1880`
- `extra_hosts: host.docker.internal:host-gateway`
- env `NODERED_CREDENTIAL_SECRET` (по умолчанию `cogitor-nodered-dev`), `MQTT_BROKER_HOST` (по умолчанию `host.docker.internal`; для Podman `host.containers.internal`; `platform`, если оба сервиса из этого compose), `ACCOUNT_SLUG`
- bind-монтирования `./nodered/data:/data`, `./nodered/settings.js`, `./nodered/flows.json`, `./nodered/entrypoint.sh` (относительно `deploy/`)

Шаблон флоу подключается к порту 1883 хоста `MQTT_BROKER_HOST`; при `MQTT_BROKER_HOST=platform` это порт контейнера, независимо от `IOT_MQTT_PORT`. Отдельных MQTT-учёток у Node-RED нет — зарегистрируйте его как **внешний плагин** в дашборде и используйте выданные `extplg_…` логин/пароль. Настройка автоматизаций — [06-automation.md](06-automation.md).

---

## Сводка портов

| Порт хоста (по умолчанию) | Порт контейнера | Протокол | Назначение |
|---|---|---|---|
| `IOT_HTTP_PORT` (8080 compose / 9083 CI) | 8080 | HTTP | Дашборд + API (+ мастер установки при первом запуске) |
| `IOT_MQTT_PORT` (1883) | 1883 | MQTT TCP | Устройства |
| `IOT_MQTT_WS_PORT` (8083) | 8083 | MQTT WS | Устройства (WebSocket) |
| `IOT_MQTT_TLS_PORT` (8883) | 8883 | MQTT TLS | Устройства (TLS; слушатель только при заданных cert/key) |
| `NODERED_PORT` (1880) | 1880 | HTTP | Node-RED (опционально) |

VPS (systemd): [12-deploy-vds.md](12-deploy-vds.md).
