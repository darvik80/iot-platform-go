-- Add MQTT credentials columns to external_plugins
ALTER TABLE external_plugins ADD COLUMN mqtt_user TEXT NOT NULL DEFAULT '';
ALTER TABLE external_plugins ADD COLUMN mqtt_pass_hash TEXT NOT NULL DEFAULT '';
