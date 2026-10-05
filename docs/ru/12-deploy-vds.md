# Развёртывание на VDS (systemd, без Docker)

Production-развёртывание платформы на виртуальном сервере. Платформа — это **один Go-бинарник**, который systemd запускает как сервис. Никакого Docker, никаких дополнительных процессов.

**Что получается:** `iot.cogitor.dev` — дашборд по HTTPS, MQTT по TCP и WebSocket.

---

## Архитектура

```mermaid
graph TB
    Internet["Internet<br/>(клиенты: браузер, устройства, Node-RED)"]

    subgraph VDS["VPS (Ubuntu / Debian)"]
        direction TB
        NGINX["nginx :80 / :443<br/>TLS termination"]
        SYSTEMD["systemd<br/>iot-platform.service"]
        PLAT["/opt/iot-platform-go/bin/platform<br/>:8080 HTTP (REST + SSE)<br/>:1883 MQTT TCP<br/>:8083 MQTT-over-WS"]
        DB[("SQLite (WAL)<br/>/opt/iot-platform-go/data/")]

        NGINX -->|"proxy_pass :8080"| PLAT
        NGINX -->|"proxy_pass :8083"| PLAT
        SYSTEMD -. "manages" .-> PLAT
        PLAT --> DB
    end

    Internet -->|"https://iot.cogitor.dev<br/>(дашборд + API + SSE)"| NGINX
    Internet -->|"wss://iot.cogitor.dev/mqtt"| NGINX
    Internet -->|"iot.cogitor.dev:1883<br/>(MQTT TCP, напрямую)"| PLAT
```

> **Уточнение к диаграмме**: real-time канал дашборда — WebSocket `/api/v1/ws`, а не SSE. БД может быть SQLite (по умолчанию) или PostgreSQL (`DATABASE_DRIVER=postgres`, `DATABASE_DSN`).

Ниже описаны два варианта установки: ручная раскладка в `/opt/iot-platform-go` (её использует CI-workflow `deploy-vds.yml`) и [Debian-пакет](#debian-пакет) (`make deb`) со своей раскладкой.

## CI/CD pipeline

```mermaid
graph LR
    DEV["Разработчик<br/>push в ветку vds"] --> CI["Gitea Actions<br/>.gitea/workflows/deploy-vds.yml"]
    CI -->|"npm ci + run build"| WEB["React SPA dist"]
    CI -->|"go build<br/>CGO_ENABLED=0 GOOS=linux"| BIN["/bin/platform (linux/amd64)"]
    CI -->|"scp + ssh"| VPS["VPS<br/>sudo install → /opt/.../bin/platform"]
    CI -->|"systemctl restart"| SVC["iot-platform.service"]
    CI -->|"curl /healthz"| CHECK{"200 OK?"}
    CHECK -->|да| OK["✅ Деплой завершён"]
    CHECK -->|нет| FAIL["❌ Провал → см. journalctl -u iot-platform"]
```

---

## Пререквизиты

| Компонент | Требование |
|-----------|-----------|
| VPS | Linux (Ubuntu 22+, Debian 12+), 512 МБ RAM, 1 vCPU |
| DNS | Запись `iot.cogitor.dev` → IP сервера |
| SSH | Пользователь с `sudo NOPASSWD` |
| nginx | Для TLS-терминации HTTPS и WSS |
| Домен | Для Let's Encrypt (или self-signed для internal use) |

---

## Подготовка сервера

### 1. Пользователь и каталоги

```bash
# На сервере
sudo useradd -r -m -d /opt/iot-platform-go -s /usr/sbin/nologin iot
sudo mkdir -p /opt/iot-platform-go/{bin,data,etc}
sudo chown -R iot:iot /opt/iot-platform-go
```

### 2. Systemd unit

```ini
# /etc/systemd/system/iot-platform.service
[Unit]
Description=IoT Platform (Go)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=iot
Group=iot
WorkingDirectory=/opt/iot-platform-go
EnvironmentFile=/opt/iot-platform-go/etc/.env
ExecStart=/opt/iot-platform-go/bin/platform
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=iot-platform

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/opt/iot-platform-go/data
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable iot-platform
```

### 3. Переменные окружения

Сам бинарник `.env` не читает; здесь файл загружает systemd через `EnvironmentFile=` в юните выше. Заданные здесь значения — это OS env, поэтому они перекрывают любой `config.yml`.

```bash
sudo tee /opt/iot-platform-go/etc/.env << 'EOF'
# --- Обязательные ---
PUBLIC_BASE_URL=https://iot.cogitor.dev
JWT_SECRET=<сгенерировать-случайный-64-символа>
BROKER_HOST_PUBLIC=iot.cogitor.dev

# --- База данных (SQLite по умолчанию; или DATABASE_DRIVER=postgres + DSN postgres://) ---
DATABASE_DRIVER=sqlite
DATABASE_DSN=/opt/iot-platform-go/data/platform.db

# --- Сеть ---
HTTP_ADDR=:8080
MQTT_ADDR=:1883

# --- Опционально ---
TIMEZONE=UTC
TELEMETRY_RETENTION_DAYS=30
LOG_LEVEL=info
EOF

sudo chown iot:iot /opt/iot-platform-go/etc/.env
sudo chmod 600 /opt/iot-platform-go/etc/.env
```

### 4. Первый запуск — мастер настройки

Администратора в env нет (переменные `ADMIN_*` игнорируются). При первом запуске БД пуста, поэтому платформа вместо дашборда поднимает на `:8080` **мастер настройки** (`/setup`, API `/api/v1/setup/*`). Он создаёт администратора и аккаунт по умолчанию — см. [Эксплуатация → мастер настройки](11-operations.md#первый-запуск-мастер-настройки).

- Эндпоинты мастера без авторизации — откройте мастер **до** включения публичного сайта в nginx, например через `ssh -L 8080:127.0.0.1:8080 user@HOST` и `http://localhost:8080/setup`.
- На шаге 1 укажите DSN, совпадающий с `DATABASE_DSN` из `.env` (`/opt/iot-platform-go/data/platform.db`); значение мастера по умолчанию `/opt/iot-platform/lib/platform.db` недоступно на запись при `ProtectSystem=strict` в этом юните.
- В этом юните на запись открыт только `/opt/iot-platform-go/data`, поэтому мастер не сможет сохранить `config.yml` (предупреждение `failed to write config.yml` ожидаемо); источником истины остаются значения в `.env`.
- После завершения мастера перезапустите сервис, чтобы маршруты мастера отключились.

---

## nginx (TLS-терминация)

```nginx
# /etc/nginx/sites-available/iot-platform
server {
    listen 443 ssl http2;
    server_name iot.cogitor.dev;

    ssl_certificate     /etc/letsencrypt/live/iot.cogitor.dev/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/iot.cogitor.dev/privkey.pem;

    # Дашборд + API
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket (дашборд real-time)
    location /api/v1/ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }

    # MQTT over WebSocket
    location /mqtt {
        proxy_pass http://127.0.0.1:8083;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400;
    }
}

server {
    listen 80;
    server_name iot.cogitor.dev;
    return 301 https://$host$request_uri;
}
```

```bash
sudo ln -s /etc/nginx/sites-available/iot-platform /etc/nginx/sites-enabled/
sudo certbot --nginx -d iot.cogitor.dev
sudo nginx -t && sudo systemctl reload nginx
```

**Порты на firewall:**

| Порт | Протокол | Назначение |
|------|----------|-----------|
| 80 | TCP | HTTP → redirect на HTTPS |
| 443 | TCP | HTTPS (дашборд + API + WebSocket + MQTT-over-WebSocket) |
| 1883 | TCP | MQTT TCP (для устройств) |
| 8883 | TCP | MQTT over TLS (только если заданы `MQTT_TLS_CERT` / `MQTT_TLS_KEY`) |

---

## Деплой бинарника

### Автоматический (Gitea CI)

Workflow **`.gitea/workflows/deploy-vds.yml`** — push в ветку `vds` или ручной запуск:

1. Собирает web UI (`npm ci && npm run build`, Node 22)
2. Собирает Go-бинарник (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`)
3. `scp` бинарник + `deploy/iot-platform.service` на сервер, запускает `deploy/remote-deploy.sh`
4. Устанавливает юнит, если он изменился, `systemctl restart iot-platform`
5. Проверка health: `curl http://127.0.0.1:8080/healthz`

> **Известная проблема**: `deploy-vds.yml` (и `build-deb.yml`) фиксируют `go-version: "1.25.x"`, тогда как `go.mod` требует `go 1.26.0`, а `Dockerfile` использует `golang:1.26-bookworm`. Если сборка падает на проверке версии Go, поднимите версию в workflow до `1.26.x` (или положитесь на автозагрузку toolchain).

**Секреты Gitea:**

| Секрет | Значение |
|--------|---------|
| `IOT_DEPLOY_HOST` | IP или домен сервера (`iot.cogitor.dev`) |
| `IOT_DEPLOY_USER` | SSH-пользователь с sudo NOPASSWD |
| `IOT_DEPLOY_SSH_KEY` | Приватный ключ ed25519 (публичный в `authorized_keys`) |

### Ручной деплой

```bash
# Локально — собрать и загрузить
make build-linux
scp bin/platform darvik@HOST:/tmp/iot-platform.new

# На сервере — установить и перезапустить
ssh darvik@HOST '
  sudo install -o iot -g iot -m 755 /tmp/iot-platform.new /opt/iot-platform-go/bin/platform
  sudo systemctl restart iot-platform
  sleep 2
  systemctl is-active --quiet iot-platform && echo "OK" || echo "FAILED"
'
```

---

## Проверка

```bash
# Статус сервиса
sudo systemctl status iot-platform

# Логи
sudo journalctl -u iot-platform -f

# Health endpoints
curl -fsS https://iot.cogitor.dev/healthz   # liveness
curl -fsS https://iot.cogitor.dev/readyz    # readiness (MQTT + DB)

# MQTT — подписка с кредами устройства на его собственные downlink-топики
# (маска '#' отклоняется ACL брокера)
mosquitto_sub -h iot.cogitor.dev -u 'product.device' -P '<token>' -v -t 'v1/me/r/req'
# отправить тестовую телеметрию
mosquitto_pub -h iot.cogitor.dev -u 'product.device' -P '<token>' -t 'v1/t' -m '{"temp":21.5}'
```

---

## Обновление платформы

При push в `vds` — CI автоматически:
1. Собирает новый бинарник
2. Загружает на сервер
3. Перезапускает сервис (простой ~2 секунды)

SQLite-база и данные сохраняются в `/opt/iot-platform-go/data/` — не затрагиваются при обновлении. При PostgreSQL данные хранятся на сервере БД; бэкап через `pg_dump` (см. [Эксплуатация → База данных](11-operations.md#база-данных)).

---

## Debian-пакет

Альтернатива ручной раскладке выше: собрать `.deb` через `make deb` (запускает `deb/build.sh`, нужен Linux или WSL с `dpkg-deb`, Node и Go). CI-workflow `.gitea/workflows/build-deb.yml` собирает тот же пакет при push в `master` / теги `v*` и на тегах прикладывает его к Gitea Release.

```bash
make deb                                   # → dist/iot-platform_<version>_amd64.deb (версия из version.txt)
sudo dpkg -i dist/iot-platform_<version>_amd64.deb
```

| Что | Где |
|-----|-----|
| Бинарник | `/opt/iot-platform/bin/platform` |
| Системный пользователь / группа | `iot-platform` (создаётся в `preinst`, home `/opt/iot-platform`, без login shell) |
| Конфиг | `/opt/iot-platform/etc/config.yml` (заготовка при первой установке; записывается мастером настройки), необязательный `/opt/iot-platform/etc/.env`, сертификаты в `/opt/iot-platform/etc/certs/` |
| Данные | `/opt/iot-platform/lib` (файл SQLite по умолчанию `/opt/iot-platform/lib/platform.db`, прошивки в `lib/firmware`) |
| Логи | `/var/log/iot-platform/platform.log` (ротация через `/etc/logrotate.d/iot-platform`) |
| systemd-юнит | `/lib/systemd/system/iot-platform.service` (исходник: `deb/iot-platform.service`) |

Юнит запускается от `iot-platform` с `WorkingDirectory=/opt/iot-platform`, `EnvironmentFile=/opt/iot-platform/etc/.env` (необязательный) и `ProtectSystem=strict` с `ReadWritePaths=/opt/iot-platform/etc /opt/iot-platform/lib /var/log/iot-platform`. `postinst` включает и (пере)запускает сервис и печатает адрес мастера `http://<host>:8080`. Так как `/opt/iot-platform/etc` доступен на запись, мастер сохраняет результат в `/opt/iot-platform/etc/config.yml`; при обновлении файл сохраняется. Предупреждения безопасности мастера действуют и здесь — завершите его до открытия порта 8080 наружу.

---

## Сводка эндпоинтов

| Сервис | URL |
|--------|-----|
| Дашборд / API | `https://iot.cogitor.dev` |
| WebSocket (real-time) | `wss://iot.cogitor.dev/api/v1/ws` |
| MQTT TCP | `iot.cogitor.dev:1883` |
| MQTT TLS (опционально) | `mqtts://iot.cogitor.dev:8883` |
| MQTT WebSocket | `wss://iot.cogitor.dev/mqtt` |
| Healthcheck | `https://iot.cogitor.dev/healthz` |

Docker / Synology: [13-deploy-docker.md](13-deploy-docker.md).
