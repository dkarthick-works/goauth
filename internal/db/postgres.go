package db

import (
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

const appDatabase = "goauth"

func Connect(databaseURL string) (*sql.DB, error) {
	databaseURL, err := ensureAppDatabase(databaseURL)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return db, nil
}

// ensureAppDatabase creates the app database if the URL points to "postgres",
// then returns a URL pointing to the app database.
func ensureAppDatabase(databaseURL string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse database URL: %w", err)
	}

	dbName := strings.TrimPrefix(u.Path, "/")
	if dbName == appDatabase {
		return databaseURL, nil
	}

	bootstrap, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return "", fmt.Errorf("open bootstrap connection: %w", err)
	}
	defer bootstrap.Close()

	if err := bootstrap.Ping(); err != nil {
		return "", fmt.Errorf("ping bootstrap connection: %w", err)
	}

	var exists bool
	err = bootstrap.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", appDatabase).Scan(&exists)
	if err != nil {
		return "", fmt.Errorf("check database existence: %w", err)
	}

	if !exists {
		if _, err := bootstrap.Exec("CREATE DATABASE " + appDatabase); err != nil {
			return "", fmt.Errorf("create database %s: %w", appDatabase, err)
		}
	}

	u.Path = "/" + appDatabase
	return u.String(), nil
}

func RunMigrations(database *sql.DB) error {
	data, err := migrationsFS.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}
	_, err = database.Exec(string(data))
	if err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}
	return nil
}
