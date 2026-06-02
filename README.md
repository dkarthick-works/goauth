# GoAuth

Minimal production-grade authentication backend in Go.

## Stack

- **Language:** Go 1.22+
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
| `APP_BASE_URL` | Base URL of the app (for email links) |
| `FROM_EMAIL` | Sender email address |

### 3. Start PostgreSQL

Ensure PostgreSQL is running and the database specified in `DATABASE_URL` exists. Migrations run automatically on startup.

### 4. Run the server

```bash
go run ./cmd/server
```

The server starts on port 8090 (configurable via `PORT` env var).

### Docker

```bash
# Build and run with PostgreSQL
docker compose up --build
```

This starts both PostgreSQL 17 and the app. Environment variables are set in `docker-compose.yml` — update `JWT_SECRET`, `RESEND_API_KEY`, `FROM_EMAIL`, and `APP_BASE_URL` there before going to production.

To build just the image:

```bash
docker build -t goauth .
docker run -p 8090:8090 \
  -e DATABASE_URL=postgres://user:pass@host:5433/goauth?sslmode=disable \
  -e JWT_SECRET=your-secret \
  -e RESEND_API_KEY=your-key \
  -e APP_BASE_URL=http://localhost:8090 \
  -e FROM_EMAIL=noreply@example.com \
  goauth
```

## API Endpoints

| Method | Route | Description |
|---|---|---|
| POST | `/auth/signup` | Register with email + password |
| POST | `/auth/login` | Login, returns access token (JSON) + refresh token (HttpOnly cookie) |
| GET | `/auth/verify?token=` | Verify email, redirects to `/login` |
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

Login endpoint is rate-limited to 5 failed attempts per IP per 15 minutes.

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
