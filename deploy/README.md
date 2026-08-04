# Crearts IoT — deploy stack

See root [README](../README.md) for full platform docs.

```bash
cp .env.example .env
docker compose up -d
```

Services: RabbitMQ (MQTT/AMQP/management), Node-RED, platform API (`:8080`).
