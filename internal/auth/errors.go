package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotVerified    = errors.New("email not verified")
	ErrTokenExpired       = errors.New("token expired")
	ErrTokenNotFound      = errors.New("token not found")
	ErrTokenUsed          = errors.New("token already used")
	ErrRateLimited        = errors.New("too many attempts")
	ErrEmailExists        = errors.New("email already registered")
	ErrInvalidEmail       = errors.New("invalid email format")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
	ErrUserNotFound       = errors.New("user not found")
)
