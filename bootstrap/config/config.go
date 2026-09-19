package config

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"task/queue/bootstrap/database"
)

type Config struct {
	Port string
	DB   *sql.DB
}

func Init() (*Config, error) {
	//DB Init
	dbconf := &database.DBConfig{
		Host:     getEnv("DB_HOST", "mysql"),
		Port:     getEnv("DB_PORT", "3306"),
		User:     getEnv("DB_USER", "relay"),
		Password: getEnv("DB_PASSWORD", "relay"),
		Database: getEnv("DB_NAME", "relay_taskqueue"),
	}
	DB, err := database.NewMySql(*dbconf)
	if err != nil {
		return nil, err
	}

	fmt.Println("Database connected successfully")

	// Port
	port := getEnv("PORT", "8000")
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	return &Config{
		Port: port,
		DB:   DB,
	}, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
