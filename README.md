# GoAuth

Minimal production-grade authentication backend in Go.

## Stack

- **Language:** Go 1.25+
- **Router:** Chi
- **Database:** PostgreSQL
- **Email:** Resend (resend.com)
- **Password hashing:** bcrypt (cost 12)
- **Sessions:** JWT access token (15 min) + opaque refresh token (7 days, stored in DB)
- **API docs:** Swagger UI at `/swagger/`

## Setup

### 1. Clone and install dependencies

```bash
go mod tidy
```

### 2. Configure environment

Copy the example and fill in your values:

```bash
cp .env.example .env
```

| Variable | Description |
|---|---|
| `DATABASE_URL` | PostgreSQL connection string |
| `JWT_SECRET` | Secret key for signing JWTs |
| `RESEND_API_KEY` | Resend API key |
| `APP_BASE_URL` | Base URL of the app (required at startup) |
| `APP_BASE_URL_FOR_MAILER` | Base URL embedded in verification and password-reset email links |
| `FROM_EMAIL` | Sender email address |
| `PORT` | HTTP port (optional, defaults to `8090`) |

For local development, `APP_BASE_URL` and `APP_BASE_URL_FOR_MAILER` are usually the same value (e.g. `http://localhost:8090`).

### 3. Start PostgreSQL

Ensure PostgreSQL is running before the app starts. If `DATABASE_URL` points at `/goauth`, that database must already exist. If it points at any other database such as `/postgres`, the app opens a bootstrap connection, creates the `goauth` database when missing, rewrites the connection URL to `/goauth`, and then connects to it; that user must have permission to create databases.

Migrations run automatically on startup.

For a disposable local database, one option is:

```bash
docker run --name goauth-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5433:5432 \
  -d postgres:16
```

Then set `DATABASE_URL` to the bootstrap database so the app can create `goauth`:

```env
DATABASE_URL=postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable
```

If your database user cannot create databases, create `goauth` yourself and point `DATABASE_URL` directly at `/goauth`.

### 4. Run the server

```bash
go run ./cmd/server
```

The server starts on port 8090 (configurable via `PORT` env var).

### Docker

Build and run the app container:

```bash
# Build and run the app container
docker compose up --build
```

The compose file runs the app only and expects an external PostgreSQL instance via `DATABASE_URL`. It also joins an external Docker network named `coolify` and exposes the app on that network with the alias `goauth`; create that network locally if your environment does not provide it:

```bash
docker network create coolify
```

Set `JWT_SECRET`, `RESEND_API_KEY`, `FROM_EMAIL`, `APP_BASE_URL`, `APP_BASE_URL_FOR_MAILER`, and `DATABASE_URL` before starting the service. The container healthcheck calls the bundled `/healthcheck` binary, which requests `GET /health`.

To build just the image:

```bash
docker build -t goauth .
docker run -p 8090:8090 \
  -e DATABASE_URL=postgres://user:pass@host:5433/goauth?sslmode=disable \
  -e JWT_SECRET=your-secret \
  -e RESEND_API_KEY=your-key \
  -e APP_BASE_URL=http://localhost:8090 \
  -e APP_BASE_URL_FOR_MAILER=http://localhost:8090 \
  -e FROM_EMAIL=noreply@example.com \
  goauth
```

## Runtime Notes

- `GET /health` returns `200` only when the process can ping PostgreSQL; Docker health checks use this endpoint.
- The HTTP server uses a 15-second read timeout, 60-second write timeout, and 60-second idle timeout.
- The Resend client has a 30-second request timeout. Signup and password reset requests can fail if Resend rejects or times out while sending email.
- Email links are built from `APP_BASE_URL_FOR_MAILER`. Verification links call the backend `GET /auth/verify` route directly; password-reset links point to `GET /auth/reset-password?token=...` for a client reset page to consume, then call the backend `POST /auth/reset-password` API. This service does not serve a reset-password HTML form.
- `APP_BASE_URL` is required at startup but does not control email links; use `APP_BASE_URL_FOR_MAILER` when changing verification or reset destinations.
- Refresh token cookies are always set with `Secure`, `HttpOnly`, and `SameSite=Strict`. Browser-based local testing should use HTTPS or a client that can manually preserve the cookie.

## API Endpoints

| Method | Route | Description |
|---|---|---|
| GET | `/health` | Health check (includes DB ping) |
| GET | `/swagger/*` | Swagger UI |
| POST | `/auth/signup` | Register with email + password |
| POST | `/auth/login` | Login, returns access token (JSON) + refresh token (HttpOnly cookie) |
| GET | `/auth/verify?token=` | Verify email and return an HTML confirmation page |
| POST | `/auth/resend-verification` | Request a new verification email without revealing whether the account exists |
| POST | `/auth/refresh` | Exchange refresh token (from cookie) for new access token |
| POST | `/auth/logout` | Invalidate refresh token, clear cookie |
| POST | `/auth/forgot-password` | Send password reset email (always returns 200) |
| POST | `/auth/reset-password` | Reset password using token |
| GET | `/auth/me` | Return authenticated user's ID and email (requires Bearer token) |

See [API.md](API.md) for request/response details and curl examples for signup, login, refresh, logout, and password-reset workflows. Regenerate Swagger docs after changing handler annotations:

```bash
swag init -g cmd/server/main.go
```

### Protected routes

Include the access token as a Bearer token:

```
Authorization: Bearer <access_token>
```

### Rate limiting

| Endpoint | Limit |
|---|---|
| `POST /auth/login` | 5 failed login attempts per IP per 15 minutes |

The login limiter keys attempts by client IP. The handler checks `X-Forwarded-For` first, then `X-Real-IP`, then the socket remote address, so reverse proxies should overwrite or sanitize those headers before forwarding traffic to the app.

`POST /auth/resend-verification` always returns 200 for known, unknown, and already-verified emails to avoid account enumeration. The current handler initializes a resend limiter but does not record requests, so do not rely on it as an enforced quota.

## Operations and troubleshooting

### Startup checklist

| Symptom | What to check |
|---|---|
| `panic: missing required environment variable: ...` | Copy `.env.example` to `.env` or export every required variable before running the server. `PORT` is the only optional variable. |
| `ping bootstrap connection` or `ping database` | PostgreSQL is not reachable from the app container/process, or the host, port, credentials, or `sslmode` in `DATABASE_URL` are wrong. |
| `create database goauth: permission denied` | The configured URL points at a bootstrap database, but the user lacks `CREATE DATABASE`. Grant permission or create `goauth` manually and point `DATABASE_URL` at it. |
| `execute migration: function gen_random_uuid() does not exist` | The schema uses `gen_random_uuid()` for UUID defaults. Enable `pgcrypto` or use a PostgreSQL environment where the function is available, then restart. |
| Docker reports `network coolify declared as external, but could not be found` | Create the expected network with `docker network create coolify`, or adjust compose networking for your local environment. |
| Login succeeds but browser refresh/logout does not send a cookie over local HTTP | Refresh cookies are always `Secure`, `HttpOnly`, `SameSite=Strict`, and scoped to `/auth`. Use HTTPS locally or an API client that preserves and sends the cookie manually. |
| Signup or password reset returns `internal server error` after database work succeeds | Email delivery is part of those flows. Check `RESEND_API_KEY`, `FROM_EMAIL`, sender/domain verification, outbound network access to Resend, and the 30-second Resend client timeout. |

### Signup diagnostics

`POST /auth/signup` performs these steps in order:

1. Validate email format and password length.
2. Hash the password with bcrypt cost 12.
3. Insert the user.
4. Generate and store a verification token.
5. Send the verification email through Resend.

The service logs each step with `signup:` prefixes, including per-step `step=` and cumulative `elapsed=` timings where available:

```text
signup: start email=user@example.com
signup: validation ok email=user@example.com elapsed=...
signup: bcrypt done email=user@example.com step=... elapsed=...
signup: user created email=user@example.com user_id=... step=... elapsed=...
signup: verification token stored user_id=... step=... elapsed=...
signup: email sent user_id=... step=... elapsed=...
signup: complete email=user@example.com user_id=... total=...
```

Use these logs to identify whether slow or failed signup requests are blocked on hashing, database writes, token storage, or email delivery. Resend calls use a 30-second HTTP client timeout. If signup returns an internal error after the user and token were created but before email delivery succeeds, fix the mailer configuration and call `POST /auth/resend-verification` for that email.

### Database operations

Database connections are managed by `database/sql` with pgx: max 25 open connections, max 5 idle connections, 5-minute connection lifetime, and 1-minute idle lifetime. The embedded migration file creates tables and indexes with `IF NOT EXISTS`, including indexes on token table `user_id` columns for user-scoped deletes and `ON DELETE CASCADE` cleanup.

The initial schema uses `gen_random_uuid()` for UUID defaults and does not create database extensions. If startup fails while running migrations with `function gen_random_uuid() does not exist`, enable `pgcrypto` or use a PostgreSQL version/environment where that function is already available, then restart the app.

## Project structure

```
/cmd/server/main.go       Entry point + route wiring
/cmd/healthcheck/         Docker health probe binary
/config/config.go         Environment variable loading
/docs/                    Generated Swagger output
/internal/auth/           Authentication logic
  handler.go              HTTP handlers
  service.go              Business logic
  repository.go           Database queries
  tokens.go               JWT + random token generation
  middleware.go           Auth middleware + rate limiter
  errors.go               Sentinel errors
/internal/db/
  postgres.go             Database connection + migration runner
  migrations/             SQL migration files
/internal/mailer/
  resend.go               Resend email integration
API.md                    Detailed API specification
```
