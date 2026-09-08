# Аутентификация и пользователи

## Способы входа

- **Логин или email** + пароль
- **Саморегистрация** с подтверждением email (опционально)
- **Google OAuth** — вход / регистрация / привязка к существующему аккаунту

Начальный администратор создаётся из переменных окружения: `ADMIN_LOGIN` / `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

Роль по умолчанию для новых пользователей: `viewer` (настраивается через `REGISTER_DEFAULT_ROLE`).

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

### Только для администратора

| Метод | Путь |
|-------|------|
| GET | `/api/v1/users` |
| PATCH | `/api/v1/users/{id}` |
| DELETE | `/api/v1/users/{id}` |

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
| `REQUIRE_EMAIL_VERIFY` | `true` | Требовать подтверждение для входа |
| `REGISTER_ENABLED` | `true` | Разрешить саморегистрацию |

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
  -d '{"email":"admin","password":"admin"}' | jq -r .token)

curl -s -X POST http://localhost:8080/api/v1/me/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"mcp"}'
# → { "token": "cip_…", "token_prefix": "cip_…", … }
```
