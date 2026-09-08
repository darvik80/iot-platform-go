# Аутентификация и пользователи

## Возможности

- Вход по **логину** или **email** + пароль
- Саморегистрация с **подтверждением email**
- **Google OAuth** — вход / регистрация
- Страница **Users** для администратора (роли, отключение, удаление)
- Начальный администратор из env (`ADMIN_LOGIN` / `ADMIN_EMAIL` / `ADMIN_PASSWORD`)

Роль по умолчанию для новых пользователей: `viewer` (`REGISTER_DEFAULT_ROLE`).

## Эндпоинты

| Метод | Путь | Авторизация |
|-------|------|------------|
| GET | `/api/v1/auth/config` | публичный |
| POST | `/api/v1/auth/register` | публичный |
| POST | `/api/v1/auth/login` | публичный |
| GET | `/api/v1/auth/verify-email?token=` | публичный (HTML) |
| POST | `/api/v1/auth/resend-verification` | публичный |
| GET | `/api/v1/auth/google` | публичный → редирект |
| GET | `/api/v1/auth/google/callback` | публичный → SPA `#auth=jwt` |
| GET | `/api/v1/users` | admin |
| PATCH | `/api/v1/users/{id}` | admin |
| DELETE | `/api/v1/users/{id}` | admin |

### Регистрация

```json
{
  "login": "jdoe",
  "email": "jdoe@example.com",
  "password": "secret123",
  "display_name": "Jane Doe"
}
```

Логин: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`.

Пока email не подтверждён (если `REQUIRE_EMAIL_VERIFY=true`), вход по паролю возвращает `email not verified`.

### Вход

```json
{ "email": "jdoe", "password": "secret123" }
```

Поле `email` принимает логин **или** email.

## Email (подтверждение)

| Переменная | По умолчанию | Примечания |
|-----------|-------------|-----------|
| `EMAIL_MODE` | `log` | `log` выводит ссылку в stdout; `smtp` отправляет письмо |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | | обязательны для `smtp` |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | ссылки подтверждения, Google callback, URL OTA |
| `REQUIRE_EMAIL_VERIFY` | `true` | |
| `REGISTER_ENABLED` | `true` | |

## Google OAuth

1. Создать OAuth-клиент в Google Cloud Console (Web application).
2. Authorized redirect URI: `{PUBLIC_BASE_URL}/api/v1/auth/google/callback`
3. Установить:

```
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
# опционально:
# GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
PUBLIC_BASE_URL=http://localhost:8080
```

Поток: дашборд → Continue with Google → Google → callback → редирект `/#auth=<jwt>`.

Если локальный аккаунт с тем же email существует, Google **привязывается** (`google_sub`). Новые Google-пользователи получают автологин из email.

## Роли

| Роль | Возможности |
|------|-----------|
| `admin` | всё + управление пользователями |
| `operator` | запись устройств/RPC/OTA |
| `viewer` | только чтение |
