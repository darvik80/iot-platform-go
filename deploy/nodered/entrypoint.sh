#!/bin/sh
# Entrypoint for Node-RED: copy initial flows from template if not exists
set -e

# Default broker host for different container runtimes:
# - Docker Desktop: host.docker.internal
# - Podman: host.containers.internal
# - Linux Docker: use host IP or 172.17.0.1
BROKER_HOST="${MQTT_BROKER_HOST:-host.docker.internal}"
# Rules-bus topics are account-scoped; must match the slug in the plugin's MQTT user (extplg_{slug}_{id}).
ACCOUNT_SLUG="${ACCOUNT_SLUG:-default}"

# Copy initial flows.json from template if not present in /data
if [ ! -f /data/flows.json ] && [ -f /templates/flows.json ]; then
    echo "Copying initial flows.json from template (broker: $BROKER_HOST, account: $ACCOUNT_SLUG)..."
    sed -e "s/__MQTT_BROKER__/$BROKER_HOST/g" -e "s/__ACCOUNT_SLUG__/$ACCOUNT_SLUG/g" /templates/flows.json > /data/flows.json
fi

# Run Node-RED directly (standard for nodered/node-red image)
exec node $NODE_OPTIONS /usr/src/node-red/node_modules/node-red/red.js --userDir /data $FLOWS
