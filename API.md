# GoAuth — API Specification

## Base URL

```
http://localhost:8090
```

## Authentication

Access tokens are short-lived JWTs (15 min). Include them as a Bearer token in the `Authorization` header for protected endpoints:

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

Refresh tokens are stored in an `HttpOnly; Secure; SameSite=Strict` cookie named `refresh_token`, scoped to path `/auth`.

Because the cookie is always marked `Secure`, browsers only send it over HTTPS. For local HTTP testing, use an API client that can manually preserve the cookie or run behind local TLS.

---

## Endpoints

### 1. Health Check

```
GET /health
```

**Response (200):**
```json
{
  "status": "healthy"
}
```

**Response (503):**
```json
{
  "status": "unhealthy",
  "error": "dial tcp: connection refused"
}
```

---

### 2. Sign Up

```
POST /auth/signup
```

Register a new user. Sends a verification email on success.

**Request:**
```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

| Field | Type | Required | Constraints |
|---|---|---|---|
| `email` | string | Yes | Valid email format |
| `password` | string | Yes | Minimum 8 characters |

**Response (201):**
```json
{
  "message": "registration successful, please check your email to verify your account"
}
```

**Error Responses:**

| Status | Body | Condition |
|---|---|---|
| 400 | `{"error":"invalid request body"}` | Malformed JSON |
| 409 | `{"error":"email already registered"}` | Email already exists |
| 422 | `{"error":"invalid email format"}` | Email fails validation |
| 422 | `{"error":"password must be at least 8 characters"}` | Password too short |

---

### 3. Login

```
POST /auth/login
```

Authenticate and receive tokens. **Rate limited:** 5 failed attempts per IP per 15 minutes.

**Request:**
```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

**Response (200):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

Also sets cookie: `refresh_token=<token>; Path=/auth; HttpOnly; Secure; SameSite=Strict; Max-Age=604800`

**Error Responses:**

| Status | Body | Condition |
|---|---|---|
| 400 | `{"error":"invalid request body"}` | Malformed JSON |
| 401 | `{"error":"invalid credentials"}` | Wrong email or password |
| 403 | `{"error":"email not verified"}` | Email not yet verified |
| 429 | `{"error":"too many login attempts, try again later"}` | Rate limit hit |

---

### 4. Verify Email

```
GET /auth/verify?token=<verification_token>
```

Verify a user's email address via the link sent after signup or resend-verification.

**Query Parameters:**

| Parameter | Required | Description |
|---|---|---|
| `token` | Yes | The verification token from the email link |

**Response (200):**
```html
<!DOCTYPE html>
<html lang="en">
  ...
  <h1>&#10003; Email Verified</h1>
</html>
```

**Error Responses:**

| Status | Body | Condition |
|---|---|---|
| 400 | `{"error":"missing token"}` | No token provided |
| 400 | `{"error":"token not found"}` | Invalid or already-used token |
| 400 | `{"error":"token expired"}` | Token older than 24 hours |

---

### 5. Resend Verification

```
POST /auth/resend-verification
```

Request a new verification email. The endpoint always returns the same success response when the email is unknown or already verified to prevent account enumeration.

**Request:**
```json
{
  "email": "user@example.com"
}
```

| Field | Type | Required | Constraints |
|---|---|---|---|
| `email` | string | Yes | Existing unverified user receives a new token |

**Response (200):**
```json
{
  "message": "if the email is registered, a verification email has been sent"
}
```

**Behavior:**

| Condition | Result |
|---|---|
| Unknown email | Returns 200 and sends no email |
| Already verified user | Returns 200 and sends no email |
| Unverified user | Deletes existing verification tokens, creates a new 24-hour token, and sends a new email |

**Error Responses:**

| Status | Body | Condition |
|---|---|---|
| 400 | `{"error":"invalid request body"}` | Malformed JSON |
| 500 | `{"error":"internal server error"}` | Token creation or email dispatch failure |

---

### 6. Refresh Token

```
POST /auth/refresh
```

Exchange a refresh token (from cookie) for a new access token. **Token rotation:** the old refresh token is invalidated and a new one is issued.

**Request:**
No body required. Reads `refresh_token` cookie.

**Response (200):**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

Also sets a new `refresh_token` cookie.

**Error Responses:**

| Status | Body | Condition |
|---|---|---|
| 401 | `{"error":"missing refresh token"}` | No cookie present |
| 400 | `{"error":"token not found"}` | Token not in DB (already used) |
| 400 | `{"error":"token expired"}` | Token older than 7 days |

---

### 7. Logout

```
POST /auth/logout
```

Invalidate the current refresh token and clear the cookie.

**Request:**
No body required. Reads `refresh_token` cookie.

**Response (200):**
```json
{
  "message": "logged out"
}
```

Clears `refresh_token` cookie (`Max-Age=-1`).

**Note:** Always returns 200 even if no cookie is present.

---

### 8. Forgot Password

```
POST /auth/forgot-password
```

Request a password reset email. Always returns 200 to prevent user enumeration.

**Request:**
```json
{
  "email": "user@example.com"
}
```

**Response (200):**
```json
{
  "message": "if the email is registered, a password reset link has been sent"
}
```

**Note:** No error is returned if the email does not exist.

---

### 9. Reset Password

```
POST /auth/reset-password
```

Reset the password using the token from the forgot-password email. Invalidates all existing refresh tokens for the user.

**Request:**
```json
{
  "token": "abc123...",
  "new_password": "newpassword123"
}
```

| Field | Type | Required | Constraints |
|---|---|---|---|
| `token` | string | Yes | Token from reset email |
| `new_password` | string | Yes | Minimum 8 characters |

**Response (200):**
```json
{
  "message": "password reset successful"
}
```

**Error Responses:**

| Status | Body | Condition |
|---|---|---|
| 400 | `{"error":"invalid request body"}` | Malformed JSON |
| 400 | `{"error":"token not found"}` | Invalid token |
| 400 | `{"error":"token expired"}` | Token older than 30 minutes |
| 400 | `{"error":"token already used"}` | Token already consumed |
| 422 | `{"error":"password must be at least 8 characters"}` | Password too short |

---

### 10. Current User (Protected)

```
GET /auth/me
```

Returns the authenticated user's ID and email. Requires a valid access token.

**Headers:**
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

**Response (200):**
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com"
}
```

**Error Responses:**

| Status | Body | Condition |
|---|---|---|
| 401 | `{"error":"missing authorization header"}` | No `Authorization` header |
| 401 | `{"error":"invalid authorization header format"}` | Header is not `Bearer <token>` |
| 401 | `{"error":"invalid or expired token"}` | Token invalid or expired |

---

## Token Details

| Token | Type | Size | Expiry | Storage |
|---|---|---|---|---|
| Access token | JWT (HS256) | ~200 chars | 15 minutes | Client memory only |
| Refresh token | Random hex | 64 chars | 7 days | HttpOnly cookie + DB |
| Verification token | Random hex | 64 chars | 24 hours | DB only, deleted after use |
| Password reset token | Random hex | 64 chars | 30 minutes | DB only, marked used |

## Security

- Passwords hashed with bcrypt (cost factor 12), never logged or returned
- All random tokens generated with `crypto/rand`
- Tokens are single-use (deleted or marked after consumption)
- Login rate-limited: 5 failures per IP per 15 minutes
- Forgot password always returns 200 (prevents email enumeration)
- Refresh token in `HttpOnly; Secure; SameSite=Strict` cookie
- JWT signed with HS256, secret from `JWT_SECRET` env variable
- Assume TLS termination at reverse proxy (all `Secure` cookies)
