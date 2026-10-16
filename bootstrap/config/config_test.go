package config

import (
	"errors"
	"testing"
	"time"

	"github.com/vhysxl/relay/pkg/errdefs"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name        string
		envKey      string
		envVal      string
		setEnv      bool
		fallback    time.Duration
		expected    time.Duration
		expectError bool
	}{
		{
			name:        "unset env returns fallback",
			envKey:      "TEST_DURATION_UNSET",
			setEnv:      false,
			fallback:    30 * time.Second,
			expected:    30 * time.Second,
			expectError: false,
		},
		{
			name:        "empty string returns fallback",
			envKey:      "TEST_DURATION_EMPTY",
			envVal:      "",
			setEnv:      true,
			fallback:    30 * time.Second,
			expected:    30 * time.Second,
			expectError: false,
		},
		{
			name:        "whitespace string returns fallback",
			envKey:      "TEST_DURATION_WS",
			envVal:      "   ",
			setEnv:      true,
			fallback:    30 * time.Second,
			expected:    30 * time.Second,
			expectError: false,
		},
		{
			name:        "valid duration string seconds",
			envKey:      "TEST_DURATION_SECS",
			envVal:      "45s",
			setEnv:      true,
			fallback:    30 * time.Second,
			expected:    45 * time.Second,
			expectError: false,
		},
		{
			name:        "valid duration string minutes",
			envKey:      "TEST_DURATION_MINS",
			envVal:      "2m",
			setEnv:      true,
			fallback:    30 * time.Second,
			expected:    2 * time.Minute,
			expectError: false,
		},
		{
			name:        "valid integer seconds",
			envKey:      "TEST_DURATION_INT",
			envVal:      "60",
			setEnv:      true,
			fallback:    30 * time.Second,
			expected:    60 * time.Second,
			expectError: false,
		},
		{
			name:        "invalid string returns error",
			envKey:      "TEST_DURATION_INVALID",
			envVal:      "invalid_time",
			setEnv:      true,
			fallback:    30 * time.Second,
			expectError: true,
		},
		{
			name:        "zero integer returns error",
			envKey:      "TEST_DURATION_ZERO_INT",
			envVal:      "0",
			setEnv:      true,
			fallback:    30 * time.Second,
			expectError: true,
		},
		{
			name:        "zero duration returns error",
			envKey:      "TEST_DURATION_ZERO_DUR",
			envVal:      "0s",
			setEnv:      true,
			fallback:    30 * time.Second,
			expectError: true,
		},
		{
			name:        "negative integer returns error",
			envKey:      "TEST_DURATION_NEG_INT",
			envVal:      "-10",
			setEnv:      true,
			fallback:    30 * time.Second,
			expectError: true,
		},
		{
			name:        "negative duration returns error",
			envKey:      "TEST_DURATION_NEG_DUR",
			envVal:      "-10s",
			setEnv:      true,
			fallback:    30 * time.Second,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.envKey, tt.envVal)
			}
			got, err := parseDuration(tt.envKey, tt.fallback)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, errdefs.ErrInvalidEnvValue) {
					t.Errorf("expected ErrInvalidEnvValue, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.expected {
					t.Errorf("got %v, want %v", got, tt.expected)
				}
			}
		})
	}
}

func TestGetIntEnv(t *testing.T) {
	tests := []struct {
		name        string
		envKey      string
		envVal      string
		setEnv      bool
		fallback    int
		expected    int
		expectError bool
	}{
		{
			name:        "unset env returns fallback",
			envKey:      "TEST_INT_UNSET",
			setEnv:      false,
			fallback:    5,
			expected:    5,
			expectError: false,
		},
		{
			name:        "empty string returns fallback",
			envKey:      "TEST_INT_EMPTY",
			envVal:      "",
			setEnv:      true,
			fallback:    5,
			expected:    5,
			expectError: false,
		},
		{
			name:        "whitespace string returns fallback",
			envKey:      "TEST_INT_WS",
			envVal:      "   ",
			setEnv:      true,
			fallback:    5,
			expected:    5,
			expectError: false,
		},
		{
			name:        "valid positive integer",
			envKey:      "TEST_INT_VALID",
			envVal:      "10",
			setEnv:      true,
			fallback:    5,
			expected:    10,
			expectError: false,
		},
		{
			name:        "valid positive integer with trailing s",
			envKey:      "TEST_INT_SUFFIX_S",
			envVal:      "30s",
			setEnv:      true,
			fallback:    5,
			expected:    30,
			expectError: false,
		},
		{
			name:        "zero returns error",
			envKey:      "TEST_INT_ZERO",
			envVal:      "0",
			setEnv:      true,
			fallback:    5,
			expectError: true,
		},
		{
			name:        "negative returns error",
			envKey:      "TEST_INT_NEG",
			envVal:      "-3",
			setEnv:      true,
			fallback:    5,
			expectError: true,
		},
		{
			name:        "non-numeric string returns error",
			envKey:      "TEST_INT_NONNUM",
			envVal:      "abc",
			setEnv:      true,
			fallback:    5,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(tt.envKey, tt.envVal)
			}
			got, err := getIntEnv(tt.envKey, tt.fallback)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, errdefs.ErrInvalidEnvValue) {
					t.Errorf("expected ErrInvalidEnvValue, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.expected {
					t.Errorf("got %v, want %v", got, tt.expected)
				}
			}
		})
	}
}
