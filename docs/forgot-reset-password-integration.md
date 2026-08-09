# Forgot Password / Reset Password — Integration Guide

Details for wiring another app to goauth's forgot-password / reset-password flow.

## 1. Endpoints

Base URL: `http://localhost:8090` (or deployment host). No `/api` prefix, no auth required on either endpoint.

### `POST /auth/forgot-password`

Request:

```json
{ "email": "user@example.com" }
```

Response — **always 200**, regardless of whether the email exists (enumeration prevention):

```json
{ "message": "if the email is registered, a password reset link has been sent" }
```

400 only on malformed JSON body.

### `POST /auth/reset-password`

Request:

```json
{ "token": "a3f8c2...(64-char hex)", "new_password": "newpassword123" }
```

Responses:

| Status | Body | Cause |
|---|---|---|
| 200 | `{"message":"password reset successful"}` | success |
| 400 | `{"error":"invalid request body"}` | malformed JSON |
| 400 | `{"error":"token not found"}` | unknown token |
| 400 | `{"error":"token expired"}` | past `PasswordResetDuration` (30 min) |
| 400 | `{"error":"token already used"}` | reused token |
| 422 | `{"error":"password must be at least 8 characters"}` | short password |

## 2. Business logic

- `ForgotPassword`: looks up user by email; if not found, silently returns success (no error surfaced to caller). If found, generates a token, stores it, emails the reset link. Any internal error here is swallowed by the handler (`_ = h.service.ForgotPassword(...)`), so the client always sees 200.
- `ResetPassword`: validates password length (>=8 chars) first, then loads token, checks expiry, checks `used` flag, hashes the new password with bcrypt (cost 12), updates the user's password, marks the token used, and **deletes all refresh tokens for that user** — every other active session is logged out.
- Token: 32-byte `crypto/rand` value, hex-encoded (64 chars), stored raw (not hashed) in `password_reset_tokens.token`.
- Token lifetime: 30 min (`PasswordResetDuration` in `internal/auth/tokens.go`). Single-use via `used` boolean.
- No rate limiting on either endpoint currently (unlike `/auth/login` and `/auth/resend-verification`, which use `RateLimiter`). Add one if exposing publicly.
- Both endpoints are stateless/cookie-free — no session dependency, safe to call cross-origin from a separate app.

Relevant source:

```221:280:internal/auth/service.go
func (s *Service) ForgotPassword(email string) error {
	user, err := s.repo.FindUserByEmail(email)
	if err != nil {
		return nil
	}

	resetToken, err := GenerateRandomToken()
	if err != nil {
		return err
	}

	err = s.repo.CreatePasswordResetToken(user.ID, resetToken, time.Now().Add(PasswordResetDuration))
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}

	if err := s.mailer.SendPasswordResetEmail(email, resetToken); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}

	return nil
}

func (s *Service) ResetPassword(token, newPassword string) error {
	if len(newPassword) < 8 {
		return ErrPasswordTooShort
	}

	prt, err := s.repo.FindPasswordResetToken(token)
	if err != nil {
		return err
	}

	if time.Now().After(prt.ExpiresAt) {
		return ErrTokenExpired
	}

	if prt.Used {
		return ErrTokenUsed
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.repo.UpdateUserPassword(prt.UserID, string(hash)); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	if err := s.repo.MarkPasswordResetTokenUsed(token); err != nil {
		return fmt.Errorf("mark token used: %w", err)
	}

	if err := s.repo.DeleteAllRefreshTokens(prt.UserID); err != nil {
		return fmt.Errorf("invalidate refresh tokens: %w", err)
	}

	return nil
}
```

## 3. Email link

```76:83:internal/mailer/resend.go
func (m *ResendMailer) SendPasswordResetEmail(to, token string) error {
	link := fmt.Sprintf("%s/auth/reset-password?token=%s", m.appBaseURL, token)
	html := fmt.Sprintf(
		`<p>You requested a password reset. Click the link below to reset your password:</p><p><a href="%s">Reset Password</a></p><p>This link expires in 30 minutes. If you did not request this, ignore this email.</p>`,
		link,
	)
	return m.send(to, "Reset your password", html)
}
```

The reset link Resend sends points to `${APP_BASE_URL_FOR_MAILER}/auth/reset-password?token=...`.

**By default this is goauth's own address** (`APP_BASE_URL_FOR_MAILER` defaults to `http://localhost:8090` in `docker-compose.yml`), and goauth serves a real, self-contained page at `GET /auth/reset-password?token=...` — a server-rendered form (new password + confirm) that submits to the existing `POST /auth/reset-password` JSON endpoint via client-side JS and shows a success/error message inline. No other app needs to build anything for this step; clicking the emailed link is enough.

To integrate a **custom-branded** reset page on a different app instead of goauth's built-in one:

1. Set `APP_BASE_URL_FOR_MAILER` (goauth env var) to the other app's origin, e.g. `https://otherapp.com`.
2. Build a page at `/auth/reset-password` on that app which:
   - reads `token` from the query string
   - shows a new-password form
   - POSTs `{token, new_password}` to this goauth API's `/auth/reset-password`

## 4. Config goauth needs

```1:25:config/config.go
type Config struct {
	DatabaseURL         string
	JWTSecret           string
	ResendAPIKey        string
	AppBaseURL          string
	AppBaseURLForMailer string
	FromEmail           string
	Port                string
}
```

All required except `PORT` (defaults to `8090`) — missing any causes a panic at startup.

## 5. DB schema involved

```sql
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    token VARCHAR(64) UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW()
);
```

## 6. Checklist before wiring a separate app to this API

- **CORS**: none configured (`chi` router has no `cors.Handler` anywhere in the codebase). If the other app is a different origin calling this API directly from the browser, add `github.com/go-chi/cors` middleware allowing that origin for `/auth/forgot-password` and `/auth/reset-password` (and set allowed methods/headers). A same-origin/reverse-proxy setup or server-to-server call works fine without it.
- **Rate limiting**: forgot/reset-password have none. If public-facing, add a limiter like the existing ones (`NewRateLimiter(...)` in `internal/auth/middleware.go`), keyed by IP.
- **CSRF**: N/A — no cookies touched by these two routes.
- **Swagger**: docs for these routes already exist in `docs/`. Regenerate with `swag init -g cmd/server/main.go` after any handler changes.
