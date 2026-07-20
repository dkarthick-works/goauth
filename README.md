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

Ensure PostgreSQL is running before the app starts. If `DATABASE_URL` points at `/goauth`, that database must already exist. If it points at another database such as `/postgres`, the app opens a bootstrap connection, creates the `goauth` database when missing, and then connects to `/goauth`; that user must have permission to create databases.

Migrations run automatically on startup.

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

If the `swag` binary is not installed, run it through Go:

```bash
go run github.com/swaggo/swag/cmd/swag init -g cmd/server/main.go
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

### Startup and deployment checklist

The server loads `.env` when present, then panics if any required variable is missing. `PORT` defaults to `8090`; every other variable in `.env.example` is required.

| Symptom | Likely cause | Check or fix |
|---|---|---|
| `missing required environment variable: ...` on startup | Required env var was not set in the shell/container | Set `DATABASE_URL`, `JWT_SECRET`, `RESEND_API_KEY`, `APP_BASE_URL`, `APP_BASE_URL_FOR_MAILER`, and `FROM_EMAIL` |
| `ping bootstrap connection` or `create database goauth` failure | `DATABASE_URL` points at a bootstrap database but the user cannot connect or create databases | Point `DATABASE_URL` at an existing `/goauth` database, or grant create-database permission when bootstrapping from `/postgres` |
| `failed to run migrations` with `gen_random_uuid` | PostgreSQL does not expose the UUID generation function used by the schema | Enable `pgcrypto` in the `goauth` database, or use a PostgreSQL setup where `gen_random_uuid()` is available |
| Docker container remains unhealthy | `/healthcheck` cannot get `200` from `GET /health`, usually because the app cannot ping PostgreSQL | Verify database reachability from the container and inspect app logs for the startup error above |
| Login works in an API client but browser refresh/logout fails | Refresh cookie is `Secure`, `HttpOnly`, and `SameSite=Strict`; the app also has no CORS middleware | Serve the browser client same-site with the API or put both behind a same-site reverse proxy with HTTPS |

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
