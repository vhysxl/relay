package app

import (
	"strings"
	"testing"

	"github.com/vhysxl/relay/internal/executor"
)

func TestRegisterRabbitMQ_Empty(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "")
	reg := executor.NewRegistry()
	closer, err := registerRabbitMQ(reg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closer == nil {
		t.Fatalf("expected non-nil closer function")
	}
	closer()
}

func TestRegisterRabbitMQ_RedactedError(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "amqp://user:secretpass@127.0.0.1:59999/")
	reg := executor.NewRegistry()
	_, err := registerRabbitMQ(reg)
	if err == nil {
		t.Fatalf("expected error when rabbitmq is unreachable, got nil")
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "secretpass") {
		t.Errorf("expected secretpass to be redacted, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "amqp://user:xxxxx@127.0.0.1:59999/") {
		t.Errorf("expected redacted URL in error, got: %s", errMsg)
	}
}
