# Authentication & users

## Features

- Login by **login** or **email** + password
- Self-registration with **email confirmation**
- **Google OAuth** sign-in / sign-up
- Admin **Users** page (roles, disable, delete)
- Bootstrap admin from env (`ADMIN_LOGIN` / `ADMIN_EMAIL` / `ADMIN_PASSWORD`)

Default role for new users: `viewer` (`REGISTER_DEFAULT_ROLE`).

## Endpoints

| Method | Path | Auth |
|--------|------|------|
| GET | `/api/v1/auth/config` | public |
| POST | `/api/v1/auth/register` | public |
| POST | `/api/v1/auth/login` | public |
| GET | `/api/v1/auth/verify-email?token=` | public (HTML) |
| POST | `/api/v1/auth/resend-verification` | public |
| GET | `/api/v1/auth/google` | public → redirect |
| GET | `/api/v1/auth/google/callback` | public → SPA `#auth=jwt` |
| GET | `/api/v1/users` | admin |
| PATCH | `/api/v1/users/{id}` | admin |
| DELETE | `/api/v1/users/{id}` | admin |

### Register

```json
{
  "login": "jdoe",
  "email": "jdoe@example.com",
  "password": "secret123",
  "display_name": "Jane Doe"
}
```

Login: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`.

Until email is verified (if `REQUIRE_EMAIL_VERIFY=true`), password login returns `email not verified`.

### Login

```json
{ "email": "jdoe", "password": "secret123" }
```

`email` field accepts login **or** email address.

## Email (verification)

| Env | Default | Notes |
|-----|---------|-------|
| `EMAIL_MODE` | `log` | `log` prints the link in server logs; `smtp` sends mail |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | | required for `smtp` |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | used in verification links |
| `REQUIRE_EMAIL_VERIFY` | `true` | |
| `REGISTER_ENABLED` | `true` | |

## Google OAuth

1. Create OAuth client in Google Cloud Console (Web application).
2. Authorized redirect URI: `{PUBLIC_BASE_URL}/api/v1/auth/google/callback`
3. Set:

```
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
# optional override:
# GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
PUBLIC_BASE_URL=http://localhost:8080
```

Flow: dashboard → Continue with Google → Google → callback → redirect `/#auth=<jwt>`.

If a local account with the same email exists, Google is **linked** (`google_sub`). New Google users get an auto login derived from email.

## Roles

| Role | Capabilities |
|------|----------------|
| `admin` | everything + users admin |
| `operator` | write devices/RPC/OTA |
| `viewer` | read-only |
