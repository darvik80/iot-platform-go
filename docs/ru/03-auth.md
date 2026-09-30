# Аутентификация и пользователи

## Способы входа

- **Логин или email** + пароль
- **Саморегистрация** с подтверждением email (опционально)
- **Google OAuth** — вход / регистрация / привязка к существующему аккаунту

Первый администратор и первый аккаунт создаются **мастером настройки**, который запускается автоматически, если база данных пуста. Переменные окружения `ADMIN_*` для создания администратора не используются.

Саморегистрация создаёт **личный аккаунт**, в котором новый пользователь — `admin`. `REGISTER_DEFAULT_ROLE` (по умолчанию `admin`) записывается только в глобальное поле `role` пользователя.

> **Замечания по безопасности.** При значениях по умолчанию (`REGISTER_ENABLED=true`, `REQUIRE_EMAIL_VERIFY=false`) любой, у кого есть доступ к дашборду, может зарегистрироваться и получить собственный аккаунт. Для закрытых установок задайте `REGISTER_ENABLED=false` и добавляйте пользователей через приглашения или раздел **Members**.

---

## Эндпоинты

### Публичные (без авторизации)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/auth/config` | Флаги: `register_enabled`, `require_email_verify`, `google_enabled` |
| POST | `/api/v1/auth/register` | Регистрация нового пользователя |
| POST | `/api/v1/auth/login` | Вход — возвращает JWT |
| GET | `/api/v1/auth/verify-email?token=` | Подтверждение email (HTML-страница) |
| POST | `/api/v1/auth/resend-verification` | Повторная отправка письма подтверждения |
| GET | `/api/v1/auth/google` | Начало Google OAuth (редирект) |
| GET | `/api/v1/auth/google/callback` | Callback от Google → редирект в SPA с `#auth=<jwt>` |
| GET | `/api/v1/auth/check-slug?slug=` | Доступность slug аккаунта |
| GET | `/api/v1/invitations/{token}` | Информация о приглашении |

### С авторизацией (активный аккаунт не требуется)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/me` | Текущий пользователь |
| GET | `/api/v1/me/accounts` | Аккаунты пользователя |
| POST | `/api/v1/auth/switch-account` | `{ "account_id" }` → новый JWT |
| GET / POST | `/api/v1/me/tokens` | Список / создание API-токенов |
| DELETE | `/api/v1/me/tokens/{id}` | Отзыв API-токена |
| GET / POST / DELETE | `/api/v1/me/avatar` | Аватар |
| POST | `/api/v1/invitations/{token}/accept` | Принять приглашение |

### Только для администратора

| Метод | Путь |
|-------|------|
| GET | `/api/v1/users` |
| PATCH | `/api/v1/users/{id}` |
| DELETE | `/api/v1/users/{id}` |
| GET / POST | `/api/v1/invitations` |
| POST | `/api/v1/invitations/{id}/resend` |
| DELETE | `/api/v1/invitations/{id}` |
| GET / POST | `/api/v1/members` |
| PATCH / DELETE | `/api/v1/members/{userId}` |
| GET | `/api/v1/audit-log` |

Полный список: [02-api.md](02-api.md).

---

## Регистрация

```json
{
  "login": "jdoe",
  "email": "jdoe@example.com",
  "password": "secret123",
  "display_name": "Jane Doe"
}
```

Требования к логину: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`.

Если `REQUIRE_EMAIL_VERIFY=true`, вход по паролю возвращает `email not verified` до подтверждения email.

## Вход

```json
{ "email": "jdoe", "password": "secret123" }
```

Поле `email` принимает **логин или email**.

---

## Подтверждение email

| Переменная | По умолчанию | Описание |
|-----------|-------------|----------|
| `EMAIL_MODE` | `log` | `log` — ссылка в stdout; `smtp` — отправка письма |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | — | Обязательны для режима `smtp` |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Ссылки подтверждения, Google callback, OTA URL |
| `REQUIRE_EMAIL_VERIFY` | `false` | Требовать подтверждение для входа |
| `REGISTER_ENABLED` | `true` | Разрешить саморегистрацию |
| `REGISTER_DEFAULT_ROLE` | `admin` | Глобальная `role`, записываемая саморегистрирующимся пользователям |

---

## Google OAuth

### Настройка

1. Создать OAuth-клиент в Google Cloud Console (тип: Web application)
2. Authorized redirect URI: `{PUBLIC_BASE_URL}/api/v1/auth/google/callback`
3. Установить переменные:

```
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
PUBLIC_BASE_URL=http://localhost:8080
```

### Поток

Дашборд → **Continue with Google** → Google → callback → редирект в `/#auth=<jwt>`.

### Привязка аккаунтов

Если локальный аккаунт с тем же email уже существует, Google **привязывается** к нему (`google_sub`). Новые Google-пользователи получают автогенерированный логин из email.

---

## Роли

| Роль | Возможности |
|------|-----------|
| `admin` | Полный доступ + управление пользователями |
| `operator` | Запись устройств, RPC, OTA, атрибуты |
| `viewer` | Только чтение |

---

## API-токены

Для MCP и скриптов — долгоживущие API-токены вместо JWT. Создаются в дашборде (**Settings → API tokens**), показываются один раз (`cip_…`).

```bash
# Создание токена через API
JWT=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin","password":"<password>"}' | jq -r .token)

curl -s -X POST http://localhost:8080/api/v1/me/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"mcp"}'
# → { "token": "cip_…", "token_prefix": "cip_…", … }
```

---

## Аудит-лог

В таблицу `audit_log` логируется часть изменяющих действий: создание продуктов и устройств, ротация токена, desired-атрибуты, RPC, запуск и уведомление OTA, изменение политик плагинов. Поля:

- `user_id` — кто выполнил действие (nullable для системных действий)
- `actor_kind` — тип актора (в схеме зарезервированы `user`, `platform_admin`, `api_token`, `setup`; сейчас пишется только `user`)
- `action` — что было сделано (например, `create`, `update`, `delete`)
- `entity_type` — что было затронуто (например, `device`, `product`, `user`)
- `entity_id` — идентификатор затронутой сущности
- `detail` — JSON с дополнительным контекстом
- `created_at` — временная метка

Сейчас у всех записей `actor_kind = user`, в том числе для действий через API-токен. Значения `platform_admin`, `api_token` и `setup` зарезервированы и пока не пишутся.
