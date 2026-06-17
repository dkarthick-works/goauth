# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run locally (requires .env)
go run ./cmd/server

# Build binary
go build -o server ./cmd/server

# Docker (app container; requires external PostgreSQL via DATABASE_URL)
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

**Email:** `internal/mailer/resend.go` calls the Resend HTTP API directly (no SDK). Links use `APP_BASE_URL_FOR_MAILER` from config.

**Config:** All env vars are loaded in `config/config.go`. Missing required vars panic at startup. Required vars are `DATABASE_URL`, `JWT_SECRET`, `RESEND_API_KEY`, `APP_BASE_URL`, `APP_BASE_URL_FOR_MAILER`, and `FROM_EMAIL`; `PORT` defaults to `8090`.

**Database bootstrap:** If `DATABASE_URL` points directly at `/goauth`, that database must already exist. If it points at another database (for example `/postgres`), startup attempts to create the `goauth` database and reconnect to it, so the configured user needs database creation privileges.

## Key design notes

- `ForgotPassword` always returns HTTP 200 regardless of whether the email exists (prevents enumeration).
- `ResetPassword` invalidates all existing refresh tokens for the user (`DeleteAllRefreshTokens`).
- The login rate limiter (`NewRateLimiter(5, 15*time.Minute)`) records failed login attempts per IP, resets on successful login, and is in-memory only.
- The resend-verification handler initializes a `NewRateLimiter(3, 10*time.Minute)`, but attempts are not currently recorded in that flow; do not document or rely on an effective resend quota without updating the implementation.
- `isUniqueViolation` in `repository.go` checks for pgx unique constraint errors by string matching, not error type assertion.
- Token durations are package-level `var`s in `tokens.go` — easy to override in tests.
- Swagger annotations live on godoc comments; regenerate with `swag init` after editing them.

## Docker setup

`docker-compose.yml` runs only the app container. It requires an external PostgreSQL server through `DATABASE_URL` and joins an external Docker network named `coolify` with the app alias `goauth`. The image includes a `/healthcheck` binary used by Compose; it calls `GET /health`.

## graphify

This project may have a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
