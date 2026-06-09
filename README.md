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

Ensure PostgreSQL is running and the database specified in `DATABASE_URL` exists. Migrations run automatically on startup.

### 4. Run the server

```bash
go run ./cmd/server
```

The server starts on port 8090 (configurable via `PORT` env var).

### Docker

Build and run the app container:

```bash
docker compose up --build
```

The compose file runs the **app only** — provide PostgreSQL via `DATABASE_URL` (e.g. from your host or a managed service). The container includes a `/healthcheck` binary used for the built-in health probe.

Set required env vars in `.env` or your deployment platform before starting. At minimum: `DATABASE_URL`, `JWT_SECRET`, `RESEND_API_KEY`, `APP_BASE_URL`, `APP_BASE_URL_FOR_MAILER`, and `FROM_EMAIL`.

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

## API Endpoints

| Method | Route | Description |
|---|---|---|
| GET | `/health` | Health check (includes DB ping) |
| GET | `/swagger/*` | Swagger UI |
| POST | `/auth/signup` | Register with email + password |
| POST | `/auth/login` | Login, returns access token (JSON) + refresh token (HttpOnly cookie) |
| GET | `/auth/verify?token=` | Verify email, returns HTML confirmation page |
| POST | `/auth/resend-verification` | Resend verification email (always returns 200) |
| POST | `/auth/refresh` | Exchange refresh token (from cookie) for new access token |
| POST | `/auth/logout` | Invalidate refresh token, clear cookie |
| POST | `/auth/forgot-password` | Send password reset email (always returns 200) |
| POST | `/auth/reset-password` | Reset password using token |
| GET | `/auth/me` | Return authenticated user's ID and email (requires Bearer token) |

See [API.md](API.md) for request/response details. Regenerate Swagger docs after changing handler annotations:

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
| `POST /auth/login` | 5 attempts per IP per 15 minutes |
| `POST /auth/resend-verification` | 3 requests per IP per 10 minutes |

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
