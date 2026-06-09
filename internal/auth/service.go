package auth

import (
	"fmt"
	"log"
	"net/mail"
	"time"

	"goauth/internal/mailer"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type Service struct {
	repo      *Repository
	mailer    *mailer.ResendMailer
	jwtSecret string
}

func NewService(repo *Repository, m *mailer.ResendMailer, jwtSecret string) *Service {
	return &Service{
		repo:      repo,
		mailer:    m,
		jwtSecret: jwtSecret,
	}
}

func (s *Service) Signup(email, password string) error {
	start := time.Now()
	log.Printf("signup: start email=%s", email)

	if _, err := mail.ParseAddress(email); err != nil {
		log.Printf("signup: invalid email email=%s", email)
		return ErrInvalidEmail
	}
	if len(password) < 8 {
		log.Printf("signup: password too short email=%s", email)
		return ErrPasswordTooShort
	}
	log.Printf("signup: validation ok email=%s elapsed=%s", email, time.Since(start))

	step := time.Now()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		log.Printf("signup: bcrypt failed email=%s err=%v", email, err)
		return fmt.Errorf("hash password: %w", err)
	}
	log.Printf("signup: bcrypt done email=%s step=%s elapsed=%s", email, time.Since(step), time.Since(start))

	step = time.Now()
	user, err := s.repo.CreateUser(email, string(hash))
	if err != nil {
		log.Printf("signup: create user failed email=%s step=%s err=%v", email, time.Since(step), err)
		return err
	}
	log.Printf("signup: user created email=%s user_id=%s step=%s elapsed=%s", email, user.ID, time.Since(step), time.Since(start))

	verificationToken, err := GenerateRandomToken()
	if err != nil {
		log.Printf("signup: generate token failed user_id=%s err=%v", user.ID, err)
		return err
	}

	step = time.Now()
	err = s.repo.CreateVerificationToken(user.ID, verificationToken, time.Now().Add(VerificationDuration))
	if err != nil {
		log.Printf("signup: create verification token failed user_id=%s step=%s err=%v", user.ID, time.Since(step), err)
		return fmt.Errorf("create verification token: %w", err)
	}
	log.Printf("signup: verification token stored user_id=%s step=%s elapsed=%s", user.ID, time.Since(step), time.Since(start))

	step = time.Now()
	if err := s.mailer.SendVerificationEmail(email, verificationToken); err != nil {
		log.Printf("signup: send email failed user_id=%s step=%s err=%v", user.ID, time.Since(step), err)
		return fmt.Errorf("send verification email: %w", err)
	}
	log.Printf("signup: email sent user_id=%s step=%s elapsed=%s", user.ID, time.Since(step), time.Since(start))

	log.Printf("signup: complete email=%s user_id=%s total=%s", email, user.ID, time.Since(start))
	return nil
}

func (s *Service) Login(email, password string) (*TokenPair, error) {
	user, err := s.repo.FindUserByEmail(email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.IsVerified {
		return nil, ErrUserNotVerified
	}

	accessToken, err := GenerateAccessToken(user.ID, user.Email, s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := GenerateRandomToken()
	if err != nil {
		return nil, err
	}

	err = s.repo.CreateRefreshToken(user.ID, refreshToken, time.Now().Add(RefreshTokenDuration))
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *Service) VerifyEmail(token string) error {
	vt, err := s.repo.FindVerificationToken(token)
	if err != nil {
		return err
	}

	if time.Now().After(vt.ExpiresAt) {
		return ErrTokenExpired
	}

	if err := s.repo.SetUserVerified(vt.UserID); err != nil {
		return fmt.Errorf("set user verified: %w", err)
	}

	if err := s.repo.DeleteVerificationToken(token); err != nil {
		return fmt.Errorf("delete verification token: %w", err)
	}

	return nil
}

func (s *Service) ResendVerification(email string) error {
	user, err := s.repo.FindUserByEmail(email)
	if err != nil {
		return nil
	}

	if user.IsVerified {
		return nil
	}

	if err := s.repo.DeleteUserVerificationTokens(user.ID); err != nil {
		return fmt.Errorf("delete old verification tokens: %w", err)
	}

	token, err := GenerateRandomToken()
	if err != nil {
		return err
	}

	if err := s.repo.CreateVerificationToken(user.ID, token, time.Now().Add(VerificationDuration)); err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}

	if err := s.mailer.SendVerificationEmail(email, token); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}

	return nil
}

func (s *Service) RefreshToken(refreshToken string) (*TokenPair, error) {
	rt, err := s.repo.FindRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	if time.Now().After(rt.ExpiresAt) {
		_ = s.repo.DeleteRefreshToken(refreshToken)
		return nil, ErrTokenExpired
	}

	if err := s.repo.DeleteRefreshToken(refreshToken); err != nil {
		return nil, fmt.Errorf("delete old refresh token: %w", err)
	}

	user, err := s.repo.FindUserByID(rt.UserID)
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	accessToken, err := GenerateAccessToken(user.ID, user.Email, s.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	newRefreshToken, err := GenerateRandomToken()
	if err != nil {
		return nil, err
	}

	err = s.repo.CreateRefreshToken(user.ID, newRefreshToken, time.Now().Add(RefreshTokenDuration))
	if err != nil {
		return nil, fmt.Errorf("create new refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (s *Service) Logout(refreshToken string) error {
	return s.repo.DeleteRefreshToken(refreshToken)
}

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
