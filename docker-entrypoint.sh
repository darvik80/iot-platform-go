#!/bin/sh
set -e
# Fix ownership of the data volume (mounted volumes may be root-owned).
chown -R platform:platform /data 2>/dev/null || true
exec su-exec platform "$@"
