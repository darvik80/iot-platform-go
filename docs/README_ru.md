# Cogitor IoT Platform — Документация

[English version](README_en.md)

## Документы

| Документ | Описание |
|----------|----------|
| [Требования к продукту (PRD)](ru/01-prd.md) | Видение продукта, функции, критерии приёмки |
| [REST API](ru/02-api.md) | Полная справка по REST API (`/api/v1`) |
| [Аутентификация](ru/03-auth.md) | Авторизация, регистрация, роли, Google OAuth |
| [MQTT-протокол](ru/04-mqtt.md) | Контракт устройств, стили топиков (native, alink, thingsboard), downlink |
| [MQTT over TLS](ru/05-mqtt-tls.md) | Настройка TLS, генерация сертификатов |
| [Автоматизация / Node-RED](ru/06-automation.md) | Шина правил, Node-RED как внешний плагин |
| [Встроенные плагины](ru/07-internal-plugins.md) | Внутрипроцессные плагины, каталог, жизненный цикл, RPC |
| [Внешние плагины](ru/08-external-plugins.md) | Внешние плагины по MQTT (реализовано); контракт webhook / gRPC (запланировано) |
| [Схема продукта](ru/09-product-schema.md) | Определение продукта: метрики, атрибуты, RPC-методы |
| [MCP (LLM-инструменты)](ru/10-mcp.md) | Эндпоинт Model Context Protocol для AI-инструментов |
| [Эксплуатация](ru/11-operations.md) | Бэкап, восстановление, мониторинг, устранение проблем |
| [Развёртывание VDS](ru/12-deploy-vds.md) | Развёртывание на bare-metal / VPS с systemd |
| [Развёртывание Docker](ru/13-deploy-docker.md) | Развёртывание Docker-образа (compose-файл содержит только Node-RED) |
| [Диаграммы](ru/14-diagrams.md) | Mermaid-диаграммы архитектуры и потоков |
| [Multi-Account и мониторинг](ru/16-multi-account.md) | Тенанты, IAM, приглашения, Prometheus exporter, политики плагинов (single mode v1.0..v1.6) |
| [HA / Clustering](ru/17-ha-architecture.md) | **Дизайн, не реализовано.** Multi-instance deployment, RabbitMQ, PostgreSQL, realtime fan-out, External Plugin SDK (v2.x) |

## Картинки

| Файл | Содержимое |
|------|------------|
| [architecture.svg](images/architecture.svg) | Текущая архитектура (входы, MQTT-транспорт, ядро, хранилище, интеграции) |
| [device-onboarding.svg](images/device-onboarding.svg) | Продукт → устройство → прошивка → онлайн |
| `dashboard-*.png` | Скриншоты веб-интерфейса (обзор, устройства, карточка устройства, MQTT) |
