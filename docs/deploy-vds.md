# Deploy VDS (systemd, no Docker)

Target: **`iot.cogitor.dev`** on the Cogitor VPS.

Infra: **`devops/cogitor`** (`iot.yml`) — nginx TLS, systemd unit, dirs, `.env`.  
CI workflow: **`.gitea/workflows/deploy-vds.yml`** — builds the binary and restarts the service.

## Prerequisites (cogitor ansible)

```bash
# DNS: iot.cogitor.dev → VPS
cd /path/to/devops/cogitor
ansible-playbook -i inventory/hosts.yml iot.yml
# then set secrets in /opt/iot-platform-go/etc/.env
```

## Gitea secrets

| Secret | Example |
|--------|---------|
| `IOT_DEPLOY_HOST` | `207.154.222.119` or `iot.cogitor.dev` |
| `IOT_DEPLOY_USER` | `darvik` (sudo NOPASSWD) |
| `IOT_DEPLOY_SSH_KEY` | private key (ed25519) whose pubkey is in `darvik` `authorized_keys` |

Push to `main`/`master` → `deploy-vds.yml` → `scp` → `systemctl restart iot-platform`.

## Manual deploy

```bash
make build-linux
scp bin/platform darvik@HOST:/tmp/iot-platform.new
ssh darvik@HOST 'sudo install -o iot -g iot -m 755 /tmp/iot-platform.new /opt/iot-platform-go/bin/platform && sudo systemctl restart iot-platform'
```

## Endpoints

| | |
|--|--|
| UI / API | `https://iot.cogitor.dev` |
| MQTT TCP | `iot.cogitor.dev:1883` |
| MQTT WS | `wss://iot.cogitor.dev/mqtt` |

Docker / Synology: [deploy-docker.md](deploy-docker.md).
