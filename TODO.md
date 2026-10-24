# TODO — Cogitor IoT Platform

## Security

### [P0] CORS: restrict allowed origins
- `internal/api/server.go:46-54` — `AllowedOrigins: ["*"]` + `AllowCredentials: true` lets any website make authenticated cross-origin requests
- Read origins from config (`Platform.PublicBaseURL` + optional `CORSOrigins` list)
- In dev, allow `localhost:*`; in prod, only the configured domain

### [P0] Security headers middleware
- No `X-Content-Type-Options`, `X-Frame-Options`, `Strict-Transport-Security`, `Content-Security-Policy`, `Referrer-Policy` anywhere
- Add a chi middleware that sets these on every response

### [P0] HTTP server timeouts
- `cmd/platform/main.go:236` — `http.Server` has no `ReadTimeout`, `WriteTimeout`, `IdleTimeout`
- Slowloris-style attacks can hold connections open indefinitely
- Set reasonable defaults: read 30s, write 30s, idle 120s

### [P1] Stop accepting JWT in query string
- `internal/api/server.go:311` — `r.URL.Query().Get("token")` leaks tokens into server logs, browser history, referrer headers
- Accept JWT only via `Authorization: Bearer` header
- WebSocket auth can keep token in query string (no header support in browsers)

### [P1] Protect /metrics endpoint
- `internal/api/server.go:87-90` — Prometheus metrics are public, leak device counts, account slugs, product IDs
- Require admin auth, or put behind a separate config flag (`MetricsPublic: bool`)

### [P2] Request body size limits
- Only firmware upload and avatar upload have `MaxBytesReader`
- Add a global middleware limiting request body to e.g. 1 MB; firmware upload keeps its own larger limit

---

## Reliability / Data Integrity

### [P0] Paginate all list queries
- `ListDevices`, `ListProducts`, `ListUsers`, `ListFirmware`, `ListInvitations`, `ListAccountMembers`, `ListAllPluginBindings` — all unbounded, no LIMIT
- Add cursor or offset pagination with sensible defaults (50/100 items per page)
- Update frontend to paginate or infinite-scroll

### [P1] Batch retention deletes
- `internal/services/platform.go:370-381` — single massive DELETE on large tables causes lock contention (SQLite) or long transactions (PostgreSQL)
- Delete in batches of 1000 with a sleep between batches

### [P1] Add missing database indexes
- `attribute_history.updated_at` — used in retention deletes, currently full table scan
- `audit_log.created_at` — used in ListAuditLog ORDER BY
- `device_logs.created_at` — used in log queries

### [P1] Don't silently discard errors
- `platform.go:223` — topic style update error ignored
- `platform.go:286` — dedup upsert error ignored
- `platform.go:299` — audit log insert error ignored (should at least log)
- `devices.go:212-213` — credential history insert error ignored

### [P2] Use config for PostgreSQL pool size
- `internal/db/db.go:303-305` — hardcoded `MaxOpenConns=8`, `MaxIdleConns=2`
- `DatabaseConfig.MaxOpenConns` exists in config but is never used

### [P2] Offline command queue
- RPC commands sent to offline devices are lost
- Persist pending commands, deliver on device reconnect
- Add TTL/expiry for stale pending commands

---

## Performance / Scalability

### [P1] Add compression middleware
- No gzip/brotli on API responses
- Telemetry JSON, device lists, firmware listings all sent uncompressed
- Add `middleware.Compress(1)` to chi

### [P1] Optimize device map rebuild
- `platform.go:96-122` — full map rebuild on every device create/update/delete/rotate
- Use write-through cache: update maps in-place on mutation instead of full reload
- Or use `sync.Map` for concurrent reads during rebuild

### [P2] MQTT publish is O(N)
- `broker.go:233-266` — `PublishToUser`/`PublishToDevice` iterate ALL connected clients
- Build client index by user ID / device token for O(1) lookup

### [P2] N+1 queries in device detail
- `server.go:651-687` — getDevice fetches device, then attributes, telemetry, events, product separately
- Combine into fewer queries with JOINs or batch loads

### [P2] Add caching layer
- No cache for frequently accessed data (products, device metadata, account settings)
- Consider in-memory LRU cache with TTL for hot data
- Invalidate on mutation

### [P3] Request deduplication / cancellation
- Frontend has no AbortController support
- Navigating away from a page doesn't cancel in-flight requests
- Add request cancellation on unmount

---

## Frontend

### [P1] Token refresh mechanism
- `web/src/api/client.ts:38-41` — on 401, user is immediately logged out
- JWT TTL is 24h, so users must re-authenticate daily
- Add refresh token flow or sliding window JWT renewal

### [P1] Network error handling in API client
- No try/catch around `fetch` — network failures propagate unhandled
- Add retry logic for transient failures (5xx, network errors)
- Show user-friendly error messages

### [P2] WebSocket reconnection improvements
- `web/src/api/ws.ts` — reconnects after 3s, but no exponential backoff
- No message queue for events during disconnection
- Add backoff (3s → 6s → 12s → 30s cap)

### [P2] Missing loading/error states
- Many pages don't show loading spinners during data fetch
- Error boundaries exist but aren't used consistently
- Add skeleton loaders for dashboard, device list, product list

### [P3] Accessibility (a11y)
- No ARIA labels on interactive elements
- No keyboard navigation for device list, product list
- No focus management in modals
- Color contrast may fail WCAG AA in some places (muted text `#8b9bb0` on dark `#1c2430`)

---

## Operational / DevOps

### [P1] Graceful MQTT broker shutdown
- `main.go:216` — `brk.Close()` is deferred, but shutdown context only stops HTTP server
- In-flight MQTT messages are dropped during shutdown
- Stop accepting new connections, drain existing, then close

### [P1] Log correlation
- Request IDs are generated (`middleware.RequestID`) but never included in log entries
- Add request ID to slog context in each request handler
- Enables tracing a request through logs

### [P1] JSON log format option
- No structured log output for log aggregation (Loki, ELK, CloudWatch)
- Add `log_format: json` config option

### [P2] Backup / restore
- No backup endpoint or scheduled backup
- For SQLite: no hot backup mechanism
- Add `POST /api/v1/admin/backup` (admin only) that dumps DB + firmware storage

### [P2] Device offline alerting
- Devices go "offline" but no notification is sent to users
- Add configurable alert rules: device offline > N minutes → email/webhook

### [P2] Stale MQTT session cleanup
- `inspect.go:81-113` — no session expiry; if `OnDisconnect` is not called (crash), stale sessions accumulate
- Add periodic background sweep of sessions inactive for > N minutes

### [P3] API versioning strategy
- All routes under `/api/v1/` but no v2 infrastructure or deprecation mechanism
- Add `Sunset` and `Deprecation` header support for future API changes

### [P3] Health check improvements
- `/readyz` checks DB + MQTT but not external dependencies (SMTP, Google OAuth)
- Add dependency-specific health endpoints

---

## Features / UX

### [P2] Device shadow delta on reconnect
- Desired vs. reported attribute diff should be pushed to device on reconnect
- Currently desired attributes are stored but not automatically synced

### [P2] Data export
- No way to export telemetry, events, or device data
- Add CSV/JSON export for device telemetry, event logs, audit log

### [P2] Webhook notifications
- No outbound webhook system for integrations
- Add configurable webhooks for events: device online/offline, OTA complete, alert triggered

### [P3] Multi-language email templates
- Email templates are English-only
- Add i18n for email content (at least EN + RU)

### [P3] Dashboard customization
- Device dashboard panels are hardcoded
- Allow users to add/remove/rearrange panels

### [P3] Firmware rollback
- OTA supports update but no rollback to previous firmware version
- Store previous firmware hash, add rollback API endpoint

---

## Code Quality

### [P2] Remove `fmt.Sprintf` for SQL table names
- `internal/services/ingest.go:244` — `fmt.Sprintf` to insert table name into SQL
- Not exploitable now (internal values only), but a code smell
- Use a whitelist map or constants

### [P2] Add tests for mcpbridge
- `internal/mcpbridge` has zero test coverage
- Add integration tests for MCP tool invocations

### [P3] Remove Python `__pycache__` from repo
- `examples/plugins/python/__pycache__/` — compiled Python cache committed
- Add to `.gitignore`, remove from tracking

### [P3] Extract shared test helpers
- Multiple test files duplicate `newTestPlatform`, `newAccountAdmin`, etc.
- Move to a shared `testutil` package or `_test.go` helpers file

---

## Summary

| Priority | Count | Focus |
|----------|-------|-------|
| P0 | 5 | Security vulnerabilities that must be fixed before production |
| P1 | 12 | Reliability, performance, and UX issues affecting users |
| P2 | 14 | Scalability, operational readiness, missing features |
| P3 | 8 | Nice-to-have improvements, code quality |
