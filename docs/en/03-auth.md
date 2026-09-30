# Authentication & users

## Login methods

- **Login or email** + password
- **Self-registration** with email verification (optional)
- **Google OAuth** — sign-in / sign-up / link to existing account

The first admin and the first account are created by the **setup wizard**, which starts automatically when the database is empty. `ADMIN_*` environment variables are not used to create the admin.

Self-registration creates a **personal account** in which the new user is `admin`. `REGISTER_DEFAULT_ROLE` (default `admin`) is only stored in the user's global `role` field.

> **Security notes.** With the defaults (`REGISTER_ENABLED=true`, `REQUIRE_EMAIL_VERIFY=false`) anyone who can reach the dashboard can sign up and get their own account. For private installs set `REGISTER_ENABLED=false` and add users via invitations or **Members**.

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
| GET | `/api/v1/auth/check-slug?slug=` | Account slug availability |
| GET | `/api/v1/invitations/{token}` | Invitation info |

### Authenticated (no active account required)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/me` | Current user |
| GET | `/api/v1/me/accounts` | User's accounts |
| POST | `/api/v1/auth/switch-account` | `{ "account_id" }` → new JWT |
| GET / POST | `/api/v1/me/tokens` | List / create API tokens |
| DELETE | `/api/v1/me/tokens/{id}` | Revoke API token |
| GET / POST / DELETE | `/api/v1/me/avatar` | Avatar |
| POST | `/api/v1/invitations/{token}/accept` | Accept invitation |

### Admin only

| Method | Path |
|--------|------|
| GET | `/api/v1/users` |
| PATCH | `/api/v1/users/{id}` |
| DELETE | `/api/v1/users/{id}` |
| GET / POST | `/api/v1/invitations` |
| POST | `/api/v1/invitations/{id}/resend` |
| DELETE | `/api/v1/invitations/{id}` |
| GET / POST | `/api/v1/members` |
| PATCH / DELETE | `/api/v1/members/{userId}` |
| GET | `/api/v1/audit-log` |

Full list: [02-api.md](02-api.md).

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
| `REQUIRE_EMAIL_VERIFY` | `false` | Require verification for login |
| `REGISTER_ENABLED` | `true` | Allow self-registration |
| `REGISTER_DEFAULT_ROLE` | `admin` | Global `role` stored for self-registered users |

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
  -d '{"email":"admin","password":"<password>"}' | jq -r .token)

curl -s -X POST http://localhost:8080/api/v1/me/tokens \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"name":"mcp"}'
# → { "token": "cip_…", "token_prefix": "cip_…", … }
```

---

## Audit log

Selected write actions are logged in the `audit_log` table: product and device creation, token rotation, desired attributes, RPC, OTA start / notify, plugin policy changes. Fields:

- `user_id` — who performed the action (nullable for system actions)
- `actor_kind` — type of actor (the schema reserves `user`, `platform_admin`, `api_token`, `setup`; currently only `user` is written)
- `action` — what was done (e.g., `create`, `update`, `delete`)
- `entity_type` — what was affected (e.g., `device`, `product`, `user`)
- `entity_id` — identifier of the affected entity
- `detail` — JSON with additional context
- `created_at` — timestamp

Today every entry has `actor_kind = user`, including actions made with an API token. The values `platform_admin`, `api_token` and `setup` are reserved and not written yet.
