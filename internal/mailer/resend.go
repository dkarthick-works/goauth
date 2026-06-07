package mailer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const resendTimeout = 30 * time.Second

type ResendMailer struct {
	apiKey     string
	fromEmail  string
	appBaseURL string
	client     *http.Client
}

type emailPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

func NewResendMailer(apiKey, fromEmail, appBaseURL string) *ResendMailer {
	return &ResendMailer{
		apiKey:     apiKey,
		fromEmail:  fromEmail,
		appBaseURL: appBaseURL,
		client:     &http.Client{Timeout: resendTimeout},
	}
}

func (m *ResendMailer) send(to, subject, html string) error {
	payload := emailPayload{
		From:    m.fromEmail,
		To:      to,
		Subject: subject,
		HTML:    html,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create email request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("resend api returned status %d", resp.StatusCode)
	}
	return nil
}

func (m *ResendMailer) SendVerificationEmail(to, token string) error {
	link := fmt.Sprintf("%s/auth/verify?token=%s", m.appBaseURL, token)
	html := fmt.Sprintf(
		`<p>Please verify your email address by clicking the link below:</p><p><a href="%s">Verify Email</a></p><p>This link expires in 24 hours.</p>`,
		link,
	)
	return m.send(to, "Verify your email", html)
}

func (m *ResendMailer) SendPasswordResetEmail(to, token string) error {
	link := fmt.Sprintf("%s/auth/reset-password?token=%s", m.appBaseURL, token)
	html := fmt.Sprintf(
		`<p>You requested a password reset. Click the link below to reset your password:</p><p><a href="%s">Reset Password</a></p><p>This link expires in 30 minutes. If you did not request this, ignore this email.</p>`,
		link,
	)
	return m.send(to, "Reset your password", html)
}
