package config

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vhysxl/relay/bootstrap/database"
)

const DefaultLeaseDuration = 30 * time.Second

type Config struct {
	Port                 string
	DB                   *sql.DB
	DefaultLeaseDuration time.Duration
	ReaperInterval       time.Duration
	DefaultTimeout       time.Duration
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
		fmt.Printf("Note: Database connection unavailable (%v). Continuing in dev mode without DB.\n", err)
		DB = nil
	} else {
		fmt.Println("Database connected successfully")
	}

	// Port
	port := getEnv("PORT", "8000")
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	// Lease
	lease, err := getIntEnv("DEFAULT_LEASE_DURATION", 30)
	if err != nil {
		return nil, err
	}

	// Reaper Interval
	reaperInterval, err := getIntEnv("REAPER_INTERVAL", 5)
	if err != nil {
		return nil, err
	}

	// Timeout
	defaultTimeout, err := getIntEnv("DEFAULT_TIMEOUT", 15)
	if err != nil {
		return nil, err
	}

	return &Config{
		Port:                 port,
		DB:                   DB,
		DefaultLeaseDuration: time.Duration(lease) * time.Second,
		ReaperInterval:       time.Duration(reaperInterval) * time.Second,
		DefaultTimeout:       time.Duration(defaultTimeout) * time.Second,
	}, nil
}

func parseDuration(val string, fallback time.Duration) time.Duration {
	if val == "" {
		return fallback
	}
	if d, err := time.ParseDuration(val); err == nil && d > 0 {
		return d
	}
	if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return fallback
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getIntEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	trimmed := strings.TrimSuffix(strings.TrimSpace(value), "s")
	integerVal, err := strconv.Atoi(trimmed)
	if err != nil {
		return fallback, nil
	}

	return integerVal, nil
}
