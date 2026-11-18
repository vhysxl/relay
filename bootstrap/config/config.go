package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vhysxl/relay/bootstrap/database"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type WorkerCredential struct {
	ID     string   `json:"id"`
	Token  string   `json:"token"`
	Queues []string `json:"queues"`
}

func (c WorkerCredential) HasQueueAccess(queue string) bool {
	if queue == "" {
		return false
	}
	for _, q := range c.Queues {
		if q == "*" || q == queue {
			return true
		}
	}
	return false
}

type Config struct {
	Port                     string
	DB                       *sql.DB
	DefaultLeaseDuration     time.Duration
	DefaultHeartBeatInterval time.Duration
	ReaperInterval           time.Duration
	DefaultTimeout           time.Duration
	MaxPayloadSize           int
	ConcurrentRetryLimit     int
	ConditionMaxDepth        int
	ConditionMaxNodes        int
	WorkerAPIEnabled         bool
	WorkerCredentials        []WorkerCredential
	WorkerLongPollTimeout    time.Duration
}

func Init() (*Config, error) {
	port := getEnv("PORT", "8000")
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	lease, err := parseDuration("DEFAULT_LEASE_DURATION", 30*time.Second)
	if err != nil {
		return nil, err
	}

	heartbeat, err := parseDuration("DEFAULT_HEARTBEAT_INTERVAL", 10*time.Second)
	if err != nil {
		return nil, err
	}

	reaperInterval, err := parseDuration("REAPER_INTERVAL", 5*time.Second)
	if err != nil {
		return nil, err
	}

	defaultTimeout, err := parseDuration("DEFAULT_TIMEOUT", 15*time.Second)
	if err != nil {
		return nil, err
	}

	maxPayloadSize, err := getIntEnv("MAX_PAYLOAD_SIZE", 5)
	if err != nil {
		return nil, err
	}

	concurrentRetryLimit, err := getIntEnv("CONCURRENT_RETRY_LIMIT", 3)
	if err != nil {
		return nil, err
	}

	conditionMaxDepth, err := getIntEnv("CONDITION_MAX_DEPTH", 8)
	if err != nil {
		return nil, err
	}

	conditionMaxNodes, err := getIntEnv("CONDITION_MAX_NODES", 64)
	if err != nil {
		return nil, err
	}

	workerAPIEnabled, workerCreds, workerLongPollTimeout, err := parseWorkerConfig()
	if err != nil {
		return nil, err
	}

	// DB Init
	dbconf := &database.DBConfig{
		Host:     getEnv("DB_HOST", "mysql"),
		Port:     getEnv("DB_PORT", "3306"),
		User:     getEnv("DB_USER", "relay"),
		Password: getEnv("DB_PASSWORD", "relay"),
		Database: getEnv("DB_NAME", "relay_taskqueue"),
	}
	DB, err := database.NewMySql(*dbconf)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	fmt.Println("Database connected successfully")

	return &Config{
		Port:                     port,
		DB:                       DB,
		DefaultLeaseDuration:     lease,
		DefaultHeartBeatInterval: heartbeat,
		ReaperInterval:           reaperInterval,
		DefaultTimeout:           defaultTimeout,
		MaxPayloadSize:           maxPayloadSize,
		ConcurrentRetryLimit:     concurrentRetryLimit,
		ConditionMaxDepth:        conditionMaxDepth,
		ConditionMaxNodes:        conditionMaxNodes,
		WorkerAPIEnabled:         workerAPIEnabled,
		WorkerCredentials:        workerCreds,
		WorkerLongPollTimeout:    workerLongPollTimeout,
	}, nil
}

// parseDuration: empty → fallback, invalid → error, <=0 → error
func parseDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	if d, err := time.ParseDuration(v); err == nil {
		if d <= 0 {
			return 0, fmt.Errorf("%w: %s=%q must be positive", errdefs.ErrInvalidEnvValue, key, v)
		}
		return d, nil
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0, fmt.Errorf("%w: %s=%q must be positive", errdefs.ErrInvalidEnvValue, key, v)
		}
		return time.Duration(secs) * time.Second, nil
	}
	return 0, fmt.Errorf("%w: %s=%q is not a valid duration or integer seconds", errdefs.ErrInvalidEnvValue, key, v)
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getIntEnv(key string, fallback int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	// strip trailing 's' to accept e.g. "30s" for integer-second fields
	v = strings.TrimSuffix(v, "s")
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%w: %s=%q must be a positive integer", errdefs.ErrInvalidEnvValue, key, os.Getenv(key))
	}
	return n, nil
}

func parseWorkerConfig() (bool, []WorkerCredential, time.Duration, error) {
	workerAPIEnabledStr := strings.TrimSpace(os.Getenv("RELAY_WORKER_API_ENABLED"))
	workerCredsJSON := strings.TrimSpace(os.Getenv("RELAY_WORKER_CREDENTIALS"))

	workerAPIEnabled := false
	if workerAPIEnabledStr != "" {
		b, err := strconv.ParseBool(workerAPIEnabledStr)
		if err != nil {
			return false, nil, 0, fmt.Errorf("%w: RELAY_WORKER_API_ENABLED must be a boolean", errdefs.ErrInvalidEnvValue)
		}
		workerAPIEnabled = b
	}

	var workerCreds []WorkerCredential
	if workerCredsJSON != "" {
		if err := json.Unmarshal([]byte(workerCredsJSON), &workerCreds); err != nil {
			return false, nil, 0, fmt.Errorf("%w: RELAY_WORKER_CREDENTIALS is not valid JSON: %v", errdefs.ErrInvalidEnvValue, err)
		}
	}

	if workerAPIEnabled {
		if len(workerCreds) == 0 {
			return false, nil, 0, fmt.Errorf("%w: RELAY_WORKER_CREDENTIALS must be configured and non-empty when RELAY_WORKER_API_ENABLED is true", errdefs.ErrInvalidEnvValue)
		}
		for i, cred := range workerCreds {
			if strings.TrimSpace(cred.ID) == "" || strings.TrimSpace(cred.Token) == "" {
				return false, nil, 0, fmt.Errorf("%w: RELAY_WORKER_CREDENTIALS[%d] must have non-empty id and token", errdefs.ErrInvalidEnvValue, i)
			}
		}
	}

	workerLongPollTimeout, err := parseDuration("WORKER_LONG_POLL_TIMEOUT", 25*time.Second)
	if err != nil {
		return false, nil, 0, err
	}

	return workerAPIEnabled, workerCreds, workerLongPollTimeout, nil
}
