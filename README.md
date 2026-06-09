# GoAuth

Minimal production-grade authentication backend in Go.

## Stack

- **Language:** Go 1.25+
- **Router:** Chi
- **Database:** PostgreSQL
- **Email:** Resend (resend.com)
- **Password hashing:** bcrypt (cost 12)
- **Sessions:** JWT access token (15 min) + refresh token (7 days, stored in DB)

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
| `APP_BASE_URL` | Required app base URL setting |
| `APP_BASE_URL_FOR_MAILER` | Public base URL used in verification and password reset email links |
| `FROM_EMAIL` | Sender email address |

### 3. Start PostgreSQL

Ensure PostgreSQL is running. The app connects with `DATABASE_URL`, runs the embedded schema migration on startup, and uses the `goauth` database.

If `DATABASE_URL` already points at `/goauth`, that database must exist. If it points at another database such as `/postgres`, startup attempts to create `goauth` and then reconnect to it, so the configured database user must have permission to create databases.

### 4. Run the server

```bash
go run ./cmd/server
```

The server starts on port 8090 (configurable via `PORT` env var).

### Docker

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
- Refresh token cookies are always set with `Secure`, `HttpOnly`, and `SameSite=Strict`. Browser-based local testing should use HTTPS or a client that can manually preserve the cookie.

## API Endpoints

| Method | Route | Description |
|---|---|---|
| POST | `/auth/signup` | Register with email + password |
| POST | `/auth/login` | Login, returns access token (JSON) + refresh token (HttpOnly cookie) |
| GET | `/auth/verify?token=` | Verify email and return an HTML confirmation page |
| POST | `/auth/resend-verification` | Request a new verification email without revealing whether the account exists |
| POST | `/auth/refresh` | Exchange refresh token (from cookie) for new access token |
| POST | `/auth/logout` | Invalidate refresh token, clear cookie |
| POST | `/auth/forgot-password` | Send password reset email (always returns 200) |
| POST | `/auth/reset-password` | Reset password using token |

### Protected routes

Use the `AuthMiddleware` to protect routes. Include the access token as a Bearer token:

```
Authorization: Bearer <access_token>
```

### Rate limiting

The login endpoint records failed attempts and allows 5 failed attempts per IP per 15 minutes. Successful login resets that IP's counter. IP detection checks `X-Forwarded-For`, then `X-Real-IP`, then the socket address.

## Project structure

```
/cmd/server/main.go       Entry point
/config/config.go         Environment variable loading
/internal/auth/           Authentication logic
  handler.go              HTTP handlers
  service.go              Business logic
  repository.go           Database queries
  tokens.go               JWT + random token generation
  middleware.go            Auth middleware + rate limiter
  errors.go               Sentinel errors
/internal/db/
  postgres.go             Database connection + migration runner
  migrations/             SQL migration files
/internal/mailer/
  resend.go               Resend email integration
```
