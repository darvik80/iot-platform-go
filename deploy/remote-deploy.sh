#!/usr/bin/env bash
# Remote deploy script — executed on the target VPS via SSH.
# Uses standard FHS paths matching the .deb package layout.
set -euo pipefail

SERVICE_NAME="__SERVICE_NAME__"
UNIT="/etc/systemd/system/${SERVICE_NAME}.service"
SERVICE_USER="iot-platform"

# Directories (matching deb package layout)
CONF_DIR="/etc/iot-platform"
DATA_DIR="/var/lib/iot-platform"
LOG_DIR="/var/log/iot-platform"

# --- ensure service user exists ---
if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  sudo useradd -r -s /usr/sbin/nologin -d "$DATA_DIR" "$SERVICE_USER"
  echo "created user $SERVICE_USER"
fi

# --- ensure directories ---
sudo mkdir -p "$CONF_DIR" "$DATA_DIR" "$LOG_DIR"
sudo chown -R "$SERVICE_USER:$SERVICE_USER" "$CONF_DIR" "$DATA_DIR" "$LOG_DIR"

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
sudo install -o "$SERVICE_USER" -g "$SERVICE_USER" -m 0755 /tmp/iot-platform.new /usr/local/bin/iot-platform
rm -f /tmp/iot-platform.new

# --- ensure .env exists (create default if missing) ---
# NOTE: JWT_SECRET, ADMIN_* are NOT set here — they are managed by the setup wizard
# which writes them to config.yml. The .env only provides runtime defaults.
if [ ! -f "$CONF_DIR/.env" ]; then
  echo "Creating minimal $CONF_DIR/.env (setup wizard will configure the rest)..."
  sudo tee "$CONF_DIR/.env" > /dev/null << 'ENVEOF'
# Runtime defaults (setup wizard writes config.yml for persistent settings)
HTTP_ADDR=:8080
MQTT_ADDR=:1883
MQTT_WS_ADDR=:8083
SQLITE_PATH=/var/lib/iot-platform/platform.db
LOG_LEVEL=info
ENVEOF
  sudo chown root:"$SERVICE_USER" "$CONF_DIR/.env"
  sudo chmod 640 "$CONF_DIR/.env"
  echo "Default .env created — run setup wizard to configure admin, JWT, etc."
fi

sudo systemctl restart "$SERVICE_NAME"

sleep 2
systemctl is-active --quiet "$SERVICE_NAME"
curl -fsS --max-time 5 http://127.0.0.1:8080/healthz >/dev/null
echo "deploy ok"
