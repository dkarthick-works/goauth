package config

import "os"

type Config struct {
	DatabaseURL         string
	JWTSecret           string
	ResendAPIKey        string
	AppBaseURL          string
	AppBaseURLForMailer string
	FromEmail           string
	Port                string
}

func Load() Config {
	return Config{
		DatabaseURL:         requireEnv("DATABASE_URL"),
		JWTSecret:           requireEnv("JWT_SECRET"),
		ResendAPIKey:        requireEnv("RESEND_API_KEY"),
		AppBaseURL:          requireEnv("APP_BASE_URL"),
		AppBaseURLForMailer: requireEnv("APP_BASE_URL_FOR_MAILER"),
		FromEmail:           requireEnv("FROM_EMAIL"),
		Port:                getEnv("PORT", "8090"),
	}
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required environment variable: " + key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
