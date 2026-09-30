-- Drop Node-RED dedicated MQTT creds (Node-RED now connects as an external plugin)
DELETE FROM settings WHERE key IN ('nodered_mqtt_user', 'nodered_mqtt_pass');
