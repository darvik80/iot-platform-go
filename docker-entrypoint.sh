#!/bin/sh
set -e
# Fix ownership of mounted volumes (may be root-owned).
chown -R platform:platform /opt/iot-platform 2>/dev/null || true
exec su-exec platform "$@"
