#!/bin/sh
set -e

LIB=/opt/iot-platform/lib
ETC=/opt/iot-platform/etc

# Older images used /data and /etc/iot-platform. A config.yml written back then
# still points there (database.dsn: /data/platform.db, cert paths), so keep both
# paths resolving to the new volumes unless something is mounted at them.
[ -e /data ] || ln -s "$LIB" /data 2>/dev/null || true
[ -e /etc/iot-platform ] || ln -s "$ETC" /etc/iot-platform 2>/dev/null || true

if [ "$(id -u)" = "0" ]; then
  # Bind mounts are usually root-owned on the host: hand them to the runtime user.
  mkdir -p "$LIB" "$ETC"
  chown -R platform:platform "$LIB" "$ETC" 2>/dev/null || true
  exec su-exec platform "$@"
fi

# Started with --user: the caller is responsible for volume permissions.
exec "$@"
