#!/usr/bin/env bash
# Remote deploy script — executed on the target VPS via SSH.
# Placeholders __DEPLOY_PATH__ and __SERVICE_NAME__ are replaced by CI before upload.
set -euo pipefail

DEPLOY_PATH="__DEPLOY_PATH__"
SERVICE_NAME="__SERVICE_NAME__"
UNIT="/etc/systemd/system/${SERVICE_NAME}.service"

# --- install systemd unit (idempotent) ---
if [ ! -f "$UNIT" ] || ! cmp -s /tmp/iot-platform.service "$UNIT"; then
  sudo install -m 0644 /tmp/iot-platform.service "$UNIT"
  sudo systemctl daemon-reload
  sudo systemctl enable "$SERVICE_NAME"
  echo "service file installed & enabled"
else
  echo "service file unchanged"
fi
rm -f /tmp/iot-platform.service

# --- deploy binary ---
sudo install -o iot -g iot -m 0755 /tmp/iot-platform.new "$DEPLOY_PATH/bin/platform"
rm -f /tmp/iot-platform.new
sudo systemctl restart "$SERVICE_NAME"

sleep 2
systemctl is-active --quiet "$SERVICE_NAME"
curl -fsS --max-time 5 http://127.0.0.1:8080/healthz >/dev/null
echo "deploy ok"
