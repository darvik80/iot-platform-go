# Развёртывание VDS (systemd, без Docker)

Цель: **`iot.cogitor.dev`** на Cogitor VPS.

Инфра: **`devops/cogitor`** (`iot.yml`) — nginx TLS, systemd unit, каталоги, `.env`.
CI workflow: **`.gitea/workflows/deploy-vds.yml`** — собирает бинарник и перезапускает сервис.

## Требования (cogitor ansible)

```bash
# DNS: iot.cogitor.dev → VPS
cd /path/to/devops/cogitor
ansible-playbook -i inventory/hosts.yml iot.yml
# затем установить секреты в /opt/iot-platform-go/etc/.env
```

## Gitea секреты

| Секрет | Пример |
|--------|--------|
| `IOT_DEPLOY_HOST` | `207.154.222.119` или `iot.cogitor.dev` |
| `IOT_DEPLOY_USER` | `darvik` (sudo NOPASSWD) |
| `IOT_DEPLOY_SSH_KEY` | приватный ключ (ed25519), публичный ключ в `darvik` `authorized_keys` |

Push в `main`/`master` → `deploy-vds.yml` → `scp` → `systemctl restart iot-platform`.

## Ручной деплой

```bash
make build-linux
scp bin/platform darvik@HOST:/tmp/iot-platform.new
ssh darvik@HOST 'sudo install -o iot -g iot -m 755 /tmp/iot-platform.new /opt/iot-platform-go/bin/platform && sudo systemctl restart iot-platform'
```

## Эндпоинты

| | |
|--|--|
| UI / API | `https://iot.cogitor.dev` |
| MQTT TCP | `iot.cogitor.dev:1883` |
| MQTT WS | `wss://iot.cogitor.dev/mqtt` |

Docker / Synology: [deploy-docker.md](deploy-docker.md).
