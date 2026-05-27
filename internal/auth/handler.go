package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const refreshTokenCookie = "refresh_token"

const verificationSuccessHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Email Verified</title>
  <style>
    body { font-family: sans-serif; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; background: #f9fafb; }
    .card { text-align: center; padding: 2rem 3rem; background: white; border-radius: 8px; box-shadow: 0 1px 4px rgba(0,0,0,.1); }
    h1 { color: #16a34a; margin-bottom: .5rem; }
    p { color: #6b7280; }
  </style>
</head>
<body>
  <div class="card">
    <h1>&#10003; Email Verified</h1>
    <p>Your email address has been confirmed.<br>You can now close this tab.</p>
  </div>
</body>
</html>`

// SignupRequest is the request body for the signup endpoint.
type SignupRequest struct {
	Email    string `json:"email"    example:"user@example.com"`
	Password string `json:"password" example:"s3cr3tpassword"`
}

// LoginRequest is the request body for the login endpoint.
type LoginRequest struct {
	Email    string `json:"email"    example:"user@example.com"`
	Password string `json:"password" example:"s3cr3tpassword"`
}

// ForgotPasswordRequest is the request body for the forgot-password endpoint.
type ForgotPasswordRequest struct {
	Email string `json:"email" example:"user@example.com"`
}

// ResetPasswordRequest is the request body for the reset-password endpoint.
type ResetPasswordRequest struct {
	Token       string `json:"token"        example:"a3f8c2..."`
	NewPassword string `json:"new_password" example:"newpassword123"`
}

// MessageResponse is a generic message response.
type MessageResponse struct {
	Message string `json:"message" example:"operation successful"`
}

// AccessTokenResponse is returned on successful login or token refresh.
type AccessTokenResponse struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// MeResponse is the response for the /auth/me endpoint.
type MeResponse struct {
	UserID string `json:"user_id" example:"123e4567-e89b-12d3-a456-426614174000"`
	Email  string `json:"email"   example:"user@example.com"`
}

type Handler struct {
	service     *Service
	jwtSecret   string
	rateLimiter *RateLimiter
}

func NewHandler(service *Service, jwtSecret string) *Handler {
	return &Handler{
		service:     service,
		jwtSecret:   jwtSecret,
		rateLimiter: NewRateLimiter(5, 15*time.Minute),
	}
}

// Signup godoc
//
//	@Summary		Register a new user
//	@Description	Create a new account. A verification email is sent after successful registration.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		SignupRequest	true	"Signup request"
//	@Success		201		{object}	MessageResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse	"email already registered"
//	@Failure		422		{object}	ErrorResponse	"invalid email or password too short"
//	@Router			/auth/signup [post]
func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	var req SignupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.Signup(req.Email, req.Password); err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, MessageResponse{
		Message: "registration successful, please check your email to verify your account",
	})
}

// Login godoc
//
//	@Summary		Log in
//	@Description	Authenticate with email and password. Returns a short-lived JWT access token and sets an HttpOnly refresh token cookie.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		LoginRequest	true	"Login request"
//	@Success		200		{object}	AccessTokenResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse	"invalid credentials"
//	@Failure		403		{object}	ErrorResponse	"email not verified"
//	@Failure		429		{object}	ErrorResponse	"too many login attempts"
//	@Router			/auth/login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	ip := getClientIP(r)

	if !h.rateLimiter.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pair, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		h.rateLimiter.Record(ip)
		writeServiceError(w, err)
		return
	}

	h.rateLimiter.Reset(ip)
	setRefreshCookie(w, pair.RefreshToken)
	writeJSON(w, http.StatusOK, AccessTokenResponse{AccessToken: pair.AccessToken})
}

// VerifyEmail godoc
//
//	@Summary		Verify email address
//	@Description	Confirm a user's email using the token sent in the verification email. Redirects to /login on success.
//	@Tags			auth
//	@Produce		json
//	@Param			token	query	string	true	"Email verification token"
//	@Success		200		"HTML confirmation page"
//	@Failure		400		{object}	ErrorResponse	"missing or invalid token"
//	@Router			/auth/verify [get]
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing token")
		return
	}

	if err := h.service.VerifyEmail(token); err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, verificationSuccessHTML)
}

// RefreshToken godoc
//
//	@Summary		Refresh access token
//	@Description	Issue a new JWT access token using the HttpOnly refresh token cookie. Also rotates the refresh token cookie.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	AccessTokenResponse
//	@Failure		400	{object}	ErrorResponse	"token expired, already used, or not found"
//	@Failure		401	{object}	ErrorResponse	"missing refresh token cookie"
//	@Router			/auth/refresh [post]
func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookie)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	pair, err := h.service.RefreshToken(cookie.Value)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	setRefreshCookie(w, pair.RefreshToken)
	writeJSON(w, http.StatusOK, AccessTokenResponse{AccessToken: pair.AccessToken})
}

// Logout godoc
//
//	@Summary		Log out
//	@Description	Invalidate the refresh token and clear the refresh token cookie.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	MessageResponse
//	@Router			/auth/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookie)
	if err != nil {
		writeJSON(w, http.StatusOK, MessageResponse{Message: "logged out"})
		return
	}

	_ = h.service.Logout(cookie.Value)

	clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, MessageResponse{Message: "logged out"})
}

// ForgotPassword godoc
//
//	@Summary		Request password reset
//	@Description	Send a password reset link to the provided email. Always responds with 200 to avoid email enumeration.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ForgotPasswordRequest	true	"Forgot password request"
//	@Success		200		{object}	MessageResponse
//	@Failure		400		{object}	ErrorResponse
//	@Router			/auth/forgot-password [post]
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req ForgotPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_ = h.service.ForgotPassword(req.Email)

	writeJSON(w, http.StatusOK, MessageResponse{
		Message: "if the email is registered, a password reset link has been sent",
	})
}

// ResetPassword godoc
//
//	@Summary		Reset password
//	@Description	Set a new password using a valid password reset token.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ResetPasswordRequest	true	"Reset password request"
//	@Success		200		{object}	MessageResponse
//	@Failure		400		{object}	ErrorResponse	"invalid, expired, or already-used token"
//	@Failure		422		{object}	ErrorResponse	"password too short"
//	@Router			/auth/reset-password [post]
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.ResetPassword(req.Token, req.NewPassword); err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, MessageResponse{Message: "password reset successful"})
}

func setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    token,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(RefreshTokenDuration.Seconds()),
	})
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshTokenCookie,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ErrorResponse is the standard error envelope.
type ErrorResponse struct {
	Error string `json:"error" example:"something went wrong"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, ErrUserNotVerified):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrTokenExpired):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrTokenNotFound):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrTokenUsed):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, ErrEmailExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidEmail):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrPasswordTooShort):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrUserNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
