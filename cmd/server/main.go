//	@title			Goauth API
//	@version		1.0
//	@description	A JWT-based authentication service with email verification and secure token management.
//	@host			localhost:8090
//	@BasePath		/
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				JWT access token — prefix with "Bearer "

package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"goauth/config"
	_ "goauth/docs"
	"goauth/internal/auth"
	"goauth/internal/db"
	"goauth/internal/mailer"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	httpSwagger "github.com/swaggo/http-swagger"
)

var database *sql.DB

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	var err error
	database, err = db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	repo := auth.NewRepository(database)
	mlr := mailer.NewResendMailer(cfg.ResendAPIKey, cfg.FromEmail, cfg.AppBaseURLForMailer)
	svc := auth.NewService(repo, mlr, cfg.JWTSecret)
	h := auth.NewHandler(svc, cfg.JWTSecret)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)

	r.Get("/health", healthHandler)
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	r.Post("/auth/signup", h.Signup)
	r.Post("/auth/login", h.Login)
	r.Get("/auth/verify", h.VerifyEmail)
	r.Post("/auth/resend-verification", h.ResendVerification)
	r.Post("/auth/refresh", h.RefreshToken)
	r.Post("/auth/logout", h.Logout)
	r.Post("/auth/forgot-password", h.ForgotPassword)
	r.Post("/auth/reset-password", h.ResetPassword)

	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(cfg.JWTSecret))
		r.Get("/auth/me", meHandler)
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("server starting on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// healthHandler godoc
//
//	@Summary		Health check
//	@Description	Returns the server and database health status.
//	@Tags			system
//	@Produce		json
//	@Success		200	{object}	map[string]string	"healthy"
//	@Failure		503	{object}	map[string]string	"unhealthy"
//	@Router			/health [get]
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if err := database.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// meHandler godoc
//
//	@Summary		Get current user
//	@Description	Return the authenticated user's ID and email extracted from the JWT.
//	@Tags			auth
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	auth.MeResponse
//	@Failure		401	{object}	auth.ErrorResponse
//	@Router			/auth/me [get]
func meHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(auth.MeResponse{
		UserID: auth.GetUserID(r.Context()),
		Email:  auth.GetUserEmail(r.Context()),
	})
}
