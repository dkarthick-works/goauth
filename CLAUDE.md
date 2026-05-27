# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run locally (requires .env)
go run ./cmd/server

# Build binary
go build -o server ./cmd/server

# Docker (PostgreSQL + app together)
docker compose up --build

# Regenerate Swagger docs (after changing handler godoc annotations)
swag init -g cmd/server/main.go
```

There are no tests in this codebase yet.

## Architecture

Three-layer structure: **Handler → Service → Repository**, all in `internal/auth/`.

- `handler.go` — HTTP request decoding, response encoding, error mapping via `writeServiceError`
- `service.go` — Business logic (validation, bcrypt, token orchestration, email dispatch)
- `repository.go` — Raw SQL against PostgreSQL; returns sentinel errors from `errors.go`
- `tokens.go` — JWT generation/validation (`HS256`) and `crypto/rand`-based opaque tokens
- `middleware.go` — `AuthMiddleware` (JWT validation, injects claims into context) + in-memory `RateLimiter` for login

**Token model:** Access tokens are short-lived JWTs (15 min). Refresh tokens are 64-char hex strings stored in the DB (`refresh_tokens` table), rotated on every use, and fully invalidated on password reset.

**Migrations:** A single SQL file (`internal/db/migrations/001_initial_schema.sql`) is embedded via `//go:embed` and executed on every startup using `IF NOT EXISTS`. No migration versioning library is used.

**Email:** `internal/mailer/resend.go` calls the Resend HTTP API directly (no SDK). Links use `APP_BASE_URL` from config.

**Config:** All env vars are loaded in `config/config.go`. Missing required vars panic at startup. `PORT` defaults to `8090`.

## Key design notes

- `ForgotPassword` always returns HTTP 200 regardless of whether the email exists (prevents enumeration).
- `ResetPassword` invalidates all existing refresh tokens for the user (`DeleteAllRefreshTokens`).
- The rate limiter (`NewRateLimiter(5, 15*time.Minute)`) is per-IP, in-memory only — it resets on restart.
- `isUniqueViolation` in `repository.go` checks for pgx unique constraint errors by string matching, not error type assertion.
- Token durations are package-level `var`s in `tokens.go` — easy to override in tests.
- Swagger annotations live on godoc comments; regenerate with `swag init` after editing them.

## Docker setup

`docker-compose.yml` requires these env vars to be set (no defaults): `POSTGRES_PASSWORD`, `APP_DB_PASSWORD`, `JWT_SECRET`, `RESEND_API_KEY`. The `scripts/init-db.sh` script creates the `goauth_app` DB user and database on first PostgreSQL startup.
