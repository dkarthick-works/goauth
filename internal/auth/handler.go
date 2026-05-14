package auth

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

const refreshTokenCookie = "refresh_token"

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

func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.Signup(req.Email, req.Password); err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"message": "registration successful, please check your email to verify your account",
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	ip := getClientIP(r)

	if !h.rateLimiter.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
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
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": pair.AccessToken,
	})
}

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

	http.Redirect(w, r, "/login", http.StatusFound)
}

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
	writeJSON(w, http.StatusOK, map[string]string{
		"access_token": pair.AccessToken,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshTokenCookie)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
		return
	}

	_ = h.service.Logout(cookie.Value)

	clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	_ = h.service.ForgotPassword(req.Email)

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "if the email is registered, a password reset link has been sent",
	})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.ResetPassword(req.Token, req.NewPassword); err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "password reset successful",
	})
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

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
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
