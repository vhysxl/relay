package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not found, using system environment variables")
	}

	host := getEnv("DB_HOST", "127.0.0.1")
	port := getEnv("DB_PORT", "3306")
	user := getEnv("DB_USER", "relay")
	pass := getEnv("DB_PASSWORD", "relay")
	dbName := getEnv("DB_NAME", "relay_taskqueue")

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?parseTime=true&clientFoundRows=true&tls=preferred&multiStatements=true",
		user, pass, host, port, dbName,
	)

	fmt.Printf("Connecting to %s:%s database %s...\n", host, port, dbName)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	fmt.Println("Database connection verified successfully.")

	// Create schema_migrations table if not exists
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(255) NOT NULL PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`)
	if err != nil {
		log.Fatalf("Failed to initialize schema_migrations table: %v", err)
	}

	// Find migrations directory
	migrationDir := "migrations"
	if _, err := os.Stat(migrationDir); os.IsNotExist(err) {
		migrationDir = filepath.Join("..", "migrations")
	}

	files, err := filepath.Glob(filepath.Join(migrationDir, "*.up.sql"))
	if err != nil || len(files) == 0 {
		log.Fatalf("No migration files found in %s: %v", migrationDir, err)
	}
	sort.Strings(files)

	fmt.Printf("Found %d migration files in %s.\n", len(files), migrationDir)

	for _, file := range files {
		baseName := filepath.Base(file)

		var exists int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", baseName).Scan(&exists)
		if err != nil {
			log.Fatalf("Failed to check migration status for %s: %v", baseName, err)
		}

		if exists > 0 {
			fmt.Printf("  [SKIP] %s (already applied)\n", baseName)
			continue
		}

		fmt.Printf("  [APPLYING] %s ... ", baseName)
		content, err := os.ReadFile(file)
		if err != nil {
			log.Fatalf("\nFailed to read file %s: %v", file, err)
		}

		// Execute migration
		if _, err := db.ExecContext(ctx, string(content)); err != nil {
			log.Fatalf("\nFailed executing migration %s: %v", baseName, err)
		}

		// Record applied migration
		_, err = db.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", baseName)
		if err != nil {
			log.Fatalf("\nFailed to record migration version %s: %v", baseName, err)
		}

		fmt.Println("OK")
	}

	fmt.Println("\nAll migrations applied successfully! Database is ready.")
}
