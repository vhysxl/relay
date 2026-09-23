package executor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vhysxl/relay/internal/step"
)

type mockAMQPPublisher struct {
	publishFunc func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

func (m *mockAMQPPublisher) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, exchange, key, mandatory, immediate, msg)
	}
	return nil
}

func TestRabbitMQExecutor_Success(t *testing.T) {
	published := false
	mock := &mockAMQPPublisher{
		publishFunc: func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
			published = true
			if exchange != "orders.exchange" {
				t.Errorf("expected exchange orders.exchange, got %s", exchange)
			}
			if key != "order.created" {
				t.Errorf("expected routing key order.created, got %s", key)
			}
			if msg.ContentType != "application/json" {
				t.Errorf("expected ContentType application/json, got %s", msg.ContentType)
			}
			var payloadMap map[string]any
			_ = json.Unmarshal(msg.Body, &payloadMap)
			if payloadMap["order_id"] != "123" {
				t.Errorf("unexpected payload: %s", string(msg.Body))
			}
			if msg.Headers["x-trace-id"] != "trace-999" {
				t.Errorf("expected header x-trace-id=trace-999, got %v", msg.Headers["x-trace-id"])
			}
			if msg.DeliveryMode != amqp.Persistent {
				t.Errorf("expected persistent delivery mode, got %d", msg.DeliveryMode)
			}
			return nil
		},
	}

	exec := NewRabbitMQExecutor(mock)
	configJSON := []byte(`{
		"exchange": "orders.exchange",
		"routing_key": "order.created",
		"content_type": "application/json",
		"headers": {"x-trace-id": "trace-999"},
		"payload": {"order_id": "123", "amount": 50000}
	}`)

	s := &step.Step{ID: "step-rmq", Type: step.StepTypeMessage}
	se := &step.StepExecution{ID: "exec-rmq", Config: configJSON}

	output, err := exec.Execute(context.Background(), s, se)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !published {
		t.Fatalf("expected message to be published")
	}

	var parsed map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	if parsed["status"] != "published" || parsed["exchange"] != "orders.exchange" || parsed["routing_key"] != "order.created" {
		t.Errorf("unexpected output payload: %+v", parsed)
	}
}

func TestRabbitMQExecutor_NilPublisher(t *testing.T) {
	exec := NewRabbitMQExecutor(nil)
	s := &step.Step{ID: "step-nil", Type: step.StepTypeMessage}
	se := &step.StepExecution{ID: "exec-nil", Config: []byte(`{"payload":{}}`)}

	_, err := exec.Execute(context.Background(), s, se)
	if !errors.Is(err, ErrNilRabbitMQPublisher) {
		t.Fatalf("expected ErrNilRabbitMQPublisher, got %v", err)
	}
}

func TestRabbitMQExecutor_MissingPayload(t *testing.T) {
	mock := &mockAMQPPublisher{}
	exec := NewRabbitMQExecutor(mock)

	configJSON := []byte(`{
		"exchange": "test",
		"routing_key": "test"
	}`)

	s := &step.Step{ID: "step-nopayload", Type: step.StepTypeMessage}
	se := &step.StepExecution{ID: "exec-nopayload", Config: configJSON}

	_, err := exec.Execute(context.Background(), s, se)
	if !errors.Is(err, ErrEmptyRabbitMQPayload) {
		t.Fatalf("expected ErrEmptyRabbitMQPayload, got %v", err)
	}
}

func TestRabbitMQExecutor_PublishError(t *testing.T) {
	expectedErr := errors.New("channel closed unexpectedly")
	mock := &mockAMQPPublisher{
		publishFunc: func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
			return expectedErr
		},
	}

	exec := NewRabbitMQExecutor(mock)
	configJSON := []byte(`{
		"exchange": "test",
		"routing_key": "test",
		"payload": {"hello": "world"}
	}`)

	s := &step.Step{ID: "step-fail", Type: step.StepTypeMessage}
	se := &step.StepExecution{ID: "exec-fail", Config: configJSON}

	_, err := exec.Execute(context.Background(), s, se)
	if err == nil {
		t.Fatalf("expected publish error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestRabbitMQExecutor_InvalidConfig(t *testing.T) {
	mock := &mockAMQPPublisher{}
	exec := NewRabbitMQExecutor(mock)

	s := &step.Step{ID: "step-inv", Type: step.StepTypeMessage}
	se := &step.StepExecution{ID: "exec-inv", Config: []byte(`{invalid-json`)}

	_, err := exec.Execute(context.Background(), s, se)
	if !errors.Is(err, ErrInvalidStepConfig) {
		t.Fatalf("expected ErrInvalidStepConfig, got %v", err)
	}
}
