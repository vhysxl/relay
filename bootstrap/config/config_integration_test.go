//go:build integration

package config

import (
	"testing"
	"time"
)

func TestInit_DefaultLeaseDuration_Env(t *testing.T) {
	// 1. When DEFAULT_LEASE_DURATION is set
	t.Setenv("DEFAULT_LEASE_DURATION", "45s")
	cfg, err := Init()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLeaseDuration != 45*time.Second {
		t.Errorf("expected 45s, got %v", cfg.DefaultLeaseDuration)
	}

	// 2. When plain integer seconds is used
	t.Setenv("DEFAULT_LEASE_DURATION", "90")
	cfg, err = Init()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLeaseDuration != 90*time.Second {
		t.Errorf("expected 90s, got %v", cfg.DefaultLeaseDuration)
	}

	// 3. Error when invalid
	t.Setenv("DEFAULT_LEASE_DURATION", "not-a-duration")
	_, err = Init()
	if err == nil {
		t.Fatalf("expected error on invalid DEFAULT_LEASE_DURATION, got nil")
	}

	// 4. Fallback when unset
	t.Setenv("DEFAULT_LEASE_DURATION", "")
	cfg, err = Init()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLeaseDuration != 30*time.Second {
		t.Errorf("expected default 30s, got %v", cfg.DefaultLeaseDuration)
	}
}

func TestInit_IntEnvConfigs(t *testing.T) {
	// Test custom values
	t.Setenv("REAPER_INTERVAL", "10")
	t.Setenv("DEFAULT_TIMEOUT", "25")
	t.Setenv("MAX_PAYLOAD_SIZE", "10")

	cfg, err := Init()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ReaperInterval != 10*time.Second {
		t.Errorf("expected ReaperInterval 10s, got %v", cfg.ReaperInterval)
	}
	if cfg.DefaultTimeout != 25*time.Second {
		t.Errorf("expected DefaultTimeout 25s, got %v", cfg.DefaultTimeout)
	}
	if cfg.MaxPayloadSize != 10 {
		t.Errorf("expected MaxPayloadSize 10, got %v", cfg.MaxPayloadSize)
	}

	// Test fail-fast error on invalid values
	t.Setenv("REAPER_INTERVAL", "invalid")
	_, err = Init()
	if err == nil {
		t.Fatalf("expected error on invalid REAPER_INTERVAL, got nil")
	}

	t.Setenv("REAPER_INTERVAL", "5")
	t.Setenv("DEFAULT_TIMEOUT", "invalid")
	_, err = Init()
	if err == nil {
		t.Fatalf("expected error on invalid DEFAULT_TIMEOUT, got nil")
	}

	t.Setenv("DEFAULT_TIMEOUT", "15")
	t.Setenv("MAX_PAYLOAD_SIZE", "invalid")
	_, err = Init()
	if err == nil {
		t.Fatalf("expected error on invalid MAX_PAYLOAD_SIZE, got nil")
	}
}
