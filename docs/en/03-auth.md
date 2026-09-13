# Authentication & users

## Login methods

- **Login or email** + password
- **Self-registration** with email verification (optional)
- **Google OAuth** — sign-in / sign-up / link to existing account

Bootstrap admin is created from environment variables: `ADMIN_LOGIN` / `ADMIN_EMAIL` / `ADMIN_PASSWORD`.

Default role for new users: `viewer` (configurable via `REGISTER_DEFAULT_ROLE`).

---

## Endpoints

### Public (no auth)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/auth/config` | Flags: `register_enabled`, `require_email_verify`, `google_enabled` |
| POST | `/api/v1/auth/register` | Register new user |
| POST | `/api/v1/auth/login` | Login — returns JWT |
| GET | `/api/v1/auth/verify-email?token=` | Email verification (HTML page) |
| POST | `/api/v1/auth/resend-verification` | Resend verification email |
| GET | `/api/v1/auth/google` | Start Google OAuth (redirect) |
| GET | `/api/v1/auth/google/callback` | Google callback → redirect to SPA with `#auth=<jwt>` |

### Admin only

| Method | Path |
|--------|------|
| GET | `/api/v1/users` |
| PATCH | `/api/v1/users/{id}` |
| DELETE | `/api/v1/users/{id}` |

---

## Registration

```json
{
  "login": "jdoe",
  "email": "jdoe@example.com",
  "password": "secret123",
  "display_name": "Jane Doe"
}
```

Login requirements: `^[a-zA-Z][a-zA-Z0-9._-]{2,31}$`.

If `REQUIRE_EMAIL_VERIFY=true`, password login returns `email not verified` until email is confirmed.

## Login

```json
{ "email": "jdoe", "password": "secret123" }
```

The `email` field accepts **login or email**.

---

## Email verification

| Env | Default | Description |
|-----|---------|-------------|
| `EMAIL_MODE` | `log` | `log` — link in stdout; `smtp` — send email |
| `SMTP_HOST` / `SMTP_PORT` / `SMTP_USER` / `SMTP_PASS` / `SMTP_FROM` | — | Required for `smtp` mode |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | Verification links, Google callback, OTA URLs |
| `REQUIRE_EMAIL_VERIFY` | `true` | Require verification for login |
| `REGISTER_ENABLED` | `true` | Allow self-registration |

---

## Google OAuth

### Setup

1. Create OAuth client in Google Cloud Console (type: Web application)
2. Authorized redirect URI: `{PUBLIC_BASE_URL}/api/v1/auth/google/callback`
3. Set environment variables:

```
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
PUBLIC_BASE_URL=http://localhost:8080
```

### Flow

Dashboard → **Continue with Google** → Google → callback → redirect to `/#auth=<jwt>`.

### Account linking

If a local account with the same email already exists, Google is **linked** to it (`google_sub`). New Google users get an auto-generated login from email.

---

## Roles

| Role | Capabilities |
|------|-------------|
| `admin` | Full access + user management |
| `operator` | Write devices, RPC, OTA, attributes |
| `viewer` | Read-only |

---

## API tokens

For MCP and scripts — long-lived API tokens instead of JWT. Created in dashboard (**Settings → API tokens**), shown once (`cip_…`).

```bash
# Create token via API
JWT=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin","password":"admin"}' | jq -r .token)

curl -s -X POST http://localhost:8080/api/v1/me/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"mcp"}'
# → { "token": "cip_…", "token_prefix": "cip_…", … }
```

---

## Audit log

All admin actions are logged in the `audit_log` table with the following fields:

- `user_id` — who performed the action (nullable for system actions)
- `actor_kind` — type of actor: `user`, `platform_admin`, `api_token`, `setup`
- `action` — what was done (e.g., `create`, `update`, `delete`)
- `entity_type` — what was affected (e.g., `device`, `product`, `user`)
- `entity_id` — identifier of the affected entity
- `detail` — JSON with additional context
- `created_at` — timestamp

The `actor_kind` field distinguishes between:
- `user` — regular account admin actions
- `platform_admin` — platform-wide administrative actions (e.g., account deletion)
- `api_token` — actions performed via API token
- `setup` — actions during initial setup wizard
