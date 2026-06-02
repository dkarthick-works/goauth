package auth

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	IsVerified   bool
	CreatedAt    time.Time
}

type VerificationToken struct {
	ID        string
	UserID    string
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type RefreshToken struct {
	ID        string
	UserID    string
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type PasswordResetToken struct {
	ID        string
	UserID    string
	Token     string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(email, passwordHash string) (*User, error) {
	user := &User{}
	err := r.db.QueryRow(
		`INSERT INTO users (email, password_hash) VALUES ($1, $2)
		 RETURNING id, email, password_hash, is_verified, created_at`,
		email, passwordHash,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.IsVerified, &user.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailExists
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (r *Repository) FindUserByEmail(email string) (*User, error) {
	user := &User{}
	err := r.db.QueryRow(
		`SELECT id, email, password_hash, is_verified, created_at FROM users WHERE email = $1`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.IsVerified, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by email: %w", err)
	}
	return user, nil
}

func (r *Repository) FindUserByID(userID string) (*User, error) {
	user := &User{}
	err := r.db.QueryRow(
		`SELECT id, email, password_hash, is_verified, created_at FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.IsVerified, &user.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

func (r *Repository) SetUserVerified(userID string) error {
	_, err := r.db.Exec(`UPDATE users SET is_verified = true WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("set user verified: %w", err)
	}
	return nil
}

func (r *Repository) UpdateUserPassword(userID, passwordHash string) error {
	_, err := r.db.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	return nil
}

func (r *Repository) CreateVerificationToken(userID, token string, expiresAt time.Time) error {
	_, err := r.db.Exec(
		`INSERT INTO verification_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, token, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	return nil
}

func (r *Repository) FindVerificationToken(token string) (*VerificationToken, error) {
	vt := &VerificationToken{}
	err := r.db.QueryRow(
		`SELECT id, user_id, token, expires_at, created_at FROM verification_tokens WHERE token = $1`,
		token,
	).Scan(&vt.ID, &vt.UserID, &vt.Token, &vt.ExpiresAt, &vt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find verification token: %w", err)
	}
	return vt, nil
}

func (r *Repository) DeleteVerificationToken(token string) error {
	_, err := r.db.Exec(`DELETE FROM verification_tokens WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete verification token: %w", err)
	}
	return nil
}

func (r *Repository) CreateRefreshToken(userID, token string, expiresAt time.Time) error {
	_, err := r.db.Exec(
		`INSERT INTO refresh_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, token, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *Repository) FindRefreshToken(token string) (*RefreshToken, error) {
	rt := &RefreshToken{}
	err := r.db.QueryRow(
		`SELECT id, user_id, token, expires_at, created_at FROM refresh_tokens WHERE token = $1`,
		token,
	).Scan(&rt.ID, &rt.UserID, &rt.Token, &rt.ExpiresAt, &rt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	return rt, nil
}

func (r *Repository) DeleteRefreshToken(token string) error {
	_, err := r.db.Exec(`DELETE FROM refresh_tokens WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

func (r *Repository) DeleteAllRefreshTokens(userID string) error {
	_, err := r.db.Exec(`DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete all refresh tokens: %w", err)
	}
	return nil
}

func (r *Repository) DeleteUserVerificationTokens(userID string) error {
	_, err := r.db.Exec(`DELETE FROM verification_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user verification tokens: %w", err)
	}
	return nil
}

func (r *Repository) CreatePasswordResetToken(userID, token string, expiresAt time.Time) error {
	_, err := r.db.Exec(
		`INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		userID, token, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return nil
}

func (r *Repository) FindPasswordResetToken(token string) (*PasswordResetToken, error) {
	prt := &PasswordResetToken{}
	err := r.db.QueryRow(
		`SELECT id, user_id, token, expires_at, used, created_at FROM password_reset_tokens WHERE token = $1`,
		token,
	).Scan(&prt.ID, &prt.UserID, &prt.Token, &prt.ExpiresAt, &prt.Used, &prt.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find password reset token: %w", err)
	}
	return prt, nil
}

func (r *Repository) MarkPasswordResetTokenUsed(token string) error {
	_, err := r.db.Exec(`UPDATE password_reset_tokens SET used = true WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("mark password reset token used: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "unique") ||
		strings.Contains(errStr, "duplicate") ||
		strings.Contains(errStr, "23505")
}
