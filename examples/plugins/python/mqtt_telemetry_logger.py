#!/usr/bin/env python3
"""
MQTT Telemetry Logger — External Plugin (Node-RED style)

Connects to the platform MQTT broker directly, subscribes to rules bus events,
and logs all telemetry/status/events to console.

Usage:
    pip install paho-mqtt requests
    python mqtt_telemetry_logger.py --platform http://localhost:8080 --token <admin-jwt>

The plugin:
1. Registers with the platform using admin JWT (gets API token + MQTT credentials)
2. Connects to MQTT broker using the provided credentials
3. Subscribes to platform/v1/events/# (all device events)
4. Logs everything to console
"""

import argparse
import json
import logging
import signal
import sys
from datetime import datetime

import paho.mqtt.client as mqtt
import requests

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("mqtt-logger")

# ─── Configuration ───────────────────────────────────────────────────────────

PLUGIN_ID = "mqtt-telemetry-logger"
PLUGIN_NAME = "MQTT Telemetry Logger"
PLUGIN_DESC = "Subscribes to rules bus via MQTT and logs all device telemetry"


# ─── Registration ────────────────────────────────────────────────────────────

def register(platform_url: str, jwt_token: str = None) -> dict:
    """Register plugin with platform, get MQTT credentials."""
    url = f"{platform_url}/api/v1/plugins/external"
    payload = {
        "id": PLUGIN_ID,
        "name": PLUGIN_NAME,
        "description": PLUGIN_DESC,
        # No endpoint = polling mode, but we use MQTT directly
    }
    headers = {"Content-Type": "application/json"}
    if jwt_token:
        headers["Authorization"] = f"Bearer {jwt_token}"
    log.info("Registering plugin with platform: %s", url)
    resp = requests.post(url, json=payload, headers=headers, timeout=10)
    resp.raise_for_status()
    data = resp.json()
    log.info("Registered: id=%s mqtt_user=%s", data["id"], data.get("mqtt_user"))
    return data


# ─── MQTT Client ─────────────────────────────────────────────────────────────

class MQTTLoggerPlugin:
    def __init__(self, config: dict):
        self.config = config
        self.mqtt_host = config.get("mqtt_host", "localhost")
        self.mqtt_port = config.get("mqtt_port", 1883)
        self.mqtt_user = config["mqtt_user"]
        self.mqtt_pass = config["mqtt_pass"]
        self.rules_bus = config.get("rules_bus", "platform/v1")
        self.client: mqtt.Client | None = None
        self.running = False

    def connect(self):
        """Connect to MQTT broker."""
        client_id = f"extplg_{PLUGIN_ID}"
        self.client = mqtt.Client(
            client_id=client_id,
            callback_api_version=mqtt.CallbackAPIVersion.VERSION2,
        )
        self.client.username_pw_set(self.mqtt_user, self.mqtt_pass)
        self.client.on_connect = self._on_connect
        self.client.on_message = self._on_message
        self.client.on_disconnect = self._on_disconnect

        log.info("Connecting to MQTT broker %s:%d as %s",
                 self.mqtt_host, self.mqtt_port, self.mqtt_user)
        self.client.connect(self.mqtt_host, self.mqtt_port, keepalive=60)
        self.running = True

    def _on_connect(self, client, userdata, flags, rc, properties=None):
        if rc == 0 or (hasattr(rc, 'value') and rc.value == 0):
            log.info("MQTT connected")
            # Subscribe to all rules bus events
            topic = f"{self.rules_bus}/events/#"
            client.subscribe(topic, qos=0)
            log.info("Subscribed to %s", topic)
        else:
            log.error("MQTT connect failed: rc=%s", rc)

    def _on_disconnect(self, client, userdata, flags, rc, properties=None):
        log.warning("MQTT disconnected: rc=%s", rc)
        self.running = False

    def _on_message(self, client, userdata, msg: mqtt.MQTTMessage):
        topic = msg.topic
        try:
            payload = json.loads(msg.payload.decode())
        except (json.JSONDecodeError, UnicodeDecodeError):
            payload = msg.payload.decode(errors="replace")

        # Parse topic: platform/v1/events/{product}/{device}/{kind}
        parts = topic.split("/")
        kind = parts[-1] if len(parts) > 3 else "unknown"
        device = parts[-2] if len(parts) > 3 else parts[-1]

        ts = datetime.now().strftime("%H:%M:%S")

        if kind == "telemetry":
            if isinstance(payload, dict):
                metrics = payload.get("metrics", payload)
                log.info("📊 [%s] %s telemetry: %s", ts, device, json.dumps(metrics, ensure_ascii=False))
            else:
                log.info("📊 [%s] %s telemetry: %s", ts, device, payload)
        elif kind == "status":
            online = payload.get("online", "?") if isinstance(payload, dict) else payload
            log.info("🔌 [%s] %s status: %s", ts, device, online)
        elif kind == "event":
            log.info("⚡ [%s] %s event: %s", ts, device, json.dumps(payload, ensure_ascii=False))
        elif kind == "log":
            log.info("📝 [%s] %s log: %s", ts, device, payload)
        elif kind == "ota":
            log.info("📦 [%s] %s OTA: %s", ts, device, json.dumps(payload, ensure_ascii=False))
        else:
            log.info("📨 [%s] %s/%s: %s", ts, device, kind, json.dumps(payload, ensure_ascii=False))

    def stop(self):
        self.running = False
        if self.client:
            self.client.disconnect()
            log.info("MQTT disconnected")


# ─── Main ────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="MQTT Telemetry Logger Plugin")
    parser.add_argument("--platform", default="http://localhost:8080",
                        help="Platform URL (default: http://localhost:8080)")
    parser.add_argument("--token", required=True,
                        help="Admin JWT token for registration")
    args = parser.parse_args()

    # Register
    config = register(args.platform, args.token)

    # Create plugin
    plugin = MQTTLoggerPlugin(config)

    # Graceful shutdown
    def shutdown(sig, frame):
        log.info("Shutting down...")
        plugin.stop()
        sys.exit(0)

    signal.signal(signal.SIGINT, shutdown)
    signal.signal(signal.SIGTERM, shutdown)

    # Connect and loop
    plugin.connect()
    plugin.client.loop_forever()


if __name__ == "__main__":
    main()
