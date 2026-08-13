#!/usr/bin/env python3
"""
Telemetry Logger — простой внешний плагин для IoT Platform.

Получает события телеметрии от устройств и выводит их в консоль.

Использование:
    pip install requests flask
    python telemetry_logger.py

Переменные окружения:
    PLATFORM_URL - URL платформы (по умолчанию http://localhost:8080)
    PLUGIN_PORT  - порт для webhook (по умолчанию 9000)
"""

import os
import time
import threading
import requests
from flask import Flask, request, jsonify

# Конфигурация
PLATFORM_URL = os.environ.get("PLATFORM_URL", "http://localhost:8080")
PLUGIN_PORT = int(os.environ.get("PLUGIN_PORT", "9000"))

# Глобальные переменные
PLUGIN_TOKEN = ""
app = Flask(__name__)
start_time = time.time()
events_count = 0


def log(msg):
    """Вывод сообщения с временной меткой."""
    ts = time.strftime("%H:%M:%S")
    print(f"[{ts}] {msg}")


# ── Heartbeat ──────────────────────────────────────────────────

def heartbeat_loop():
    """Периодический heartbeat в платформу."""
    global events_count
    while True:
        try:
            resp = requests.post(
                f"{PLATFORM_URL}/api/plugins/external/rpc",
                headers={"Authorization": f"Bearer {PLUGIN_TOKEN}"},
                json={
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "platform.heartbeat",
                    "params": {
                        "plugin_id": "telemetry-logger",
                        "status": "ok",
                        "stats": {
                            "events_processed": events_count,
                            "uptime_sec": int(time.time() - start_time),
                        }
                    }
                },
                timeout=5,
            )
            if resp.status_code == 200:
                result = resp.json().get("result", {})
                pending = result.get("pending_events", 0)
                if pending > 0:
                    log(f"Heartbeat OK, pending events: {pending}")
        except Exception as e:
            log(f"Heartbeat failed: {e}")
        time.sleep(30)


# ── Регистрация плагина ────────────────────────────────────────

def register():
    """Регистрация плагина на платформе."""
    descriptor = {
        "id": "telemetry-logger",
        "name": "Telemetry Logger",
        "description": "Выводит телеметрию устройств в консоль",
        "endpoint": f"http://localhost:{PLUGIN_PORT}/rpc",
        "auto": False,  # не нужны тики
        "interval_ms": 0,
        "default_config": {},
        "form": [],
    }
    
    log(f"Регистрация плагина на {PLATFORM_URL}...")
    resp = requests.post(
        f"{PLATFORM_URL}/api/plugins/external/register",
        json=descriptor,
        timeout=5,
    )
    
    if resp.status_code == 200:
        data = resp.json()
        token = data.get("token", "")
        log(f"Зарегистрирован: id={data.get('id')}, token={token[:16]}...")
        return token
    else:
        raise RuntimeError(f"Registration failed: {resp.status_code} {resp.text}")


# ── Обработчики событий ────────────────────────────────────────

@app.route("/rpc", methods=["POST"])
def handle_rpc():
    """Единый endpoint для всех уведомлений от платформы."""
    data = request.get_json(force=True)
    method = data.get("method")
    params = data.get("params", {})
    
    if method == "plugin.apply_config":
        return handle_apply_config(params)
    elif method == "plugin.on_event":
        return handle_on_event(params)
    elif method == "plugin.detach":
        return handle_detach(params)
    else:
        log(f"Неизвестный метод: {method}")
        return jsonify({"error": f"unknown method: {method}"}), 400


def handle_apply_config(params):
    """Применение конфигурации binding."""
    binding_id = params.get("binding_id")
    log(f"Binding {binding_id}: конфигурация применена")
    return jsonify({"ok": True})


def handle_on_event(params):
    """Обработка события от устройства."""
    global events_count
    events_count += 1
    
    binding_id = params.get("binding_id")
    device_pk = params.get("device_pk")
    kind = params.get("kind")
    online = params.get("online", False)
    payload = params.get("payload", {})
    
    if kind == "telemetry":
        # Вывод телеметрии в консоль
        log(f"📊 Device #{device_pk} (binding {binding_id}):")
        for key, value in payload.items():
            log(f"   {key}: {value}")
    
    elif kind == "status":
        status = "online" if online else "offline"
        log(f"🔌 Device #{device_pk}: {status}")
    
    elif kind == "event":
        log(f"⚡ Device #{device_pk} event: {payload}")
    
    elif kind == "tick":
        # Тики не логируем (auto=False)
        pass
    
    else:
        log(f"📨 Device #{device_pk} [{kind}]: {payload}")
    
    return jsonify({"ok": True})


def handle_detach(params):
    """Binding удалён."""
    binding_id = params.get("binding_id")
    log(f"Binding {binding_id}: отключён")
    return jsonify({"ok": True})


# ── Запуск ─────────────────────────────────────────────────────

if __name__ == "__main__":
    log("=" * 50)
    log("Telemetry Logger Plugin")
    log("=" * 50)
    
    # Регистрация
    PLUGIN_TOKEN = register()
    
    # Запуск heartbeat в фоне
    threading.Thread(target=heartbeat_loop, daemon=True).start()
    
    # Запуск Flask сервера
    log(f"Listening on port {PLUGIN_PORT}...")
    log("Ожидание событий от платформы...")
    log("-" * 50)
    app.run(host="0.0.0.0", port=PLUGIN_PORT, debug=False)
