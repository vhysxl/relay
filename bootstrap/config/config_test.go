package config

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fallback time.Duration
		expected time.Duration
	}{
		{"empty string returns fallback", "", 30 * time.Second, 30 * time.Second},
		{"valid duration string seconds", "45s", 30 * time.Second, 45 * time.Second},
		{"valid duration string minutes", "2m", 30 * time.Second, 2 * time.Minute},
		{"valid integer seconds", "60", 30 * time.Second, 60 * time.Second},
		{"invalid string returns fallback", "invalid_time", 30 * time.Second, 30 * time.Second},
		{"negative integer returns fallback", "-10", 30 * time.Second, 30 * time.Second},
		{"negative duration returns fallback", "-10s", 30 * time.Second, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDuration(tt.input, tt.fallback)
			if got != tt.expected {
				t.Errorf("parseDuration(%q, %v) = %v; want %v", tt.input, tt.fallback, got, tt.expected)
			}
		})
	}
}

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

	// 3. Fallback when invalid
	t.Setenv("DEFAULT_LEASE_DURATION", "not-a-duration")
	cfg, err = Init()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLeaseDuration != DefaultLeaseDuration {
		t.Errorf("expected default 30s, got %v", cfg.DefaultLeaseDuration)
	}

	// 4. Fallback when unset
	t.Setenv("DEFAULT_LEASE_DURATION", "")
	cfg, err = Init()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultLeaseDuration != DefaultLeaseDuration {
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

	// Test fallback on invalid values (should not error or panic)
	t.Setenv("REAPER_INTERVAL", "invalid")
	t.Setenv("DEFAULT_TIMEOUT", "invalid")
	t.Setenv("MAX_PAYLOAD_SIZE", "invalid")

	cfg, err = Init()
	if err != nil {
		t.Fatalf("unexpected error on invalid env: %v", err)
	}
	if cfg.ReaperInterval != 5*time.Second {
		t.Errorf("expected default ReaperInterval 5s, got %v", cfg.ReaperInterval)
	}
	if cfg.DefaultTimeout != 15*time.Second {
		t.Errorf("expected default DefaultTimeout 15s, got %v", cfg.DefaultTimeout)
	}
	if cfg.MaxPayloadSize != 5 {
		t.Errorf("expected default MaxPayloadSize 5, got %v", cfg.MaxPayloadSize)
	}
}
