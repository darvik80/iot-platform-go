# MCP (LLM → устройства)

Streamable HTTP MCP **встроен в платформу** на `/mcp` (тот же процесс, JWT/API-токен). Отдельного MCP-бинарника или контейнера нет.

## Инструменты

| Инструмент | Назначение |
|-----------|-----------|
| `overview` | Счётчики онлайн/офлайн, статус MQTT |
| `list_devices` | Фильтрация по продукту/статусу/запросу |
| `get_device` | Полная информация (телеметрия, атрибуты, схема) |
| `list_products` / `get_product` | Схемы продуктов |
| `list_device_methods` | RPC-методы схемы продукта (включая встроенный `rpc-list`) |
| `invoke_rpc` | Вызов метода устройства (`reboot`, пользовательские, …) |
| `reboot_device` | Быстрая перезагрузка |
| `get_telemetry` | Последние сэмплы |
| `get_attributes` | Reported + desired |
| `set_desired_attributes` | Отправка desired-атрибутов |

Идентификация устройства по platform `id` **или** `product_id` + `device_id`.

## Эндпоинт / авторизация

```
http://<platform-host>:8080/mcp
Authorization: Bearer <API-токен или JWT>
```

Предпочтительно долгоживущий **API-токен** (Settings → API tokens), не login JWT.

1. Открыть дашборд → **Settings**
2. Создать токен (напр. имя `mcp`) — скопировать `cip_…` один раз
3. Конфигурация Cursor:

```json
{
  "mcpServers": {
    "cogitor-iot": {
      "url": "http://192.168.1.100:8080/mcp",
      "headers": {
        "Authorization": "Bearer cip_…"
      }
    }
  }
}
```

Или через API:

```bash
# login JWT (короткоживущий) для создания API-токена
JWT=$(curl -s -X POST http://192.168.1.100:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin","password":"admin"}' | jq -r .token)

curl -s -X POST http://192.168.1.100:8080/api/v1/me/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"mcp"}'
# → { "token": "cip_…", "token_prefix": "cip_…", … }
```

Инструменты записи (`invoke_rpc`, `reboot_device`, `set_desired_attributes`) требуют роль operator/admin у пользователя токена.

## Развёртывание

MCP — часть `/platform`. Без дополнительного Docker-сервиса, порта или env.
