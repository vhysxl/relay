package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
)

var (
	ErrEmptyRabbitMQPayload = errors.New("rabbitmq payload cannot be empty")
	ErrNilRabbitMQPublisher = errors.New("rabbitmq publisher is not configured")
)

// AMQPPublisher abstracts the AMQP channel publish method for testability and flexibility
type AMQPPublisher interface {
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

type rabbitMQExecutor struct {
	publisher AMQPPublisher
}

// NewRabbitMQExecutor creates a StepExecutor for StepTypeMessage
func NewRabbitMQExecutor(publisher AMQPPublisher) StepExecutor {
	return &rabbitMQExecutor{publisher: publisher}
}

// Execute publishes the message specified in the step configuration to RabbitMQ
func (e *rabbitMQExecutor) Execute(ctx context.Context, s *step.Step, exec *execution.StepExecution) (json.RawMessage, error) {
	if e.publisher == nil {
		return nil, ErrNilRabbitMQPublisher
	}

	// extract config from execution ticket, fallback to step definition
	configRaw := exec.Config
	if len(configRaw) == 0 && s != nil {
		configRaw = s.Config
	}
	if len(configRaw) == 0 {
		return nil, ErrInvalidStepConfig
	}

	var cfg step.RabbitMQConfig
	if err := json.Unmarshal(configRaw, &cfg); err != nil {
		return nil, fmt.Errorf("%w: failed to parse RabbitMQConfig: %v", ErrInvalidStepConfig, err)
	}

	if len(cfg.Payload) == 0 || string(cfg.Payload) == "null" {
		return nil, ErrEmptyRabbitMQPayload
	}

	contentType := cfg.ContentType
	if contentType == "" {
		contentType = "application/json"
	}

	// resolve step timeout if specified
	timeoutSeconds := 0
	if exec.TimeoutSeconds > 0 {
		timeoutSeconds = exec.TimeoutSeconds
	} else if s != nil && s.Timeout > 0 {
		timeoutSeconds = s.Timeout
	}

	pubCtx := ctx
	if timeoutSeconds > 0 {
		var cancel context.CancelFunc
		pubCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
	}

	now := time.Now().UTC()
	var headers amqp.Table
	if len(cfg.Headers) > 0 {
		headers = amqp.Table(cfg.Headers)
	}

	msg := amqp.Publishing{
		ContentType:  contentType,
		Headers:      headers,
		Body:         cfg.Payload,
		Timestamp:    now,
		DeliveryMode: amqp.Persistent,
	}

	if err := e.publisher.PublishWithContext(pubCtx, cfg.Exchange, cfg.RoutingKey, false, false, msg); err != nil {
		return nil, fmt.Errorf("failed to publish rabbitmq message: %w", err)
	}

	// return structured publication confirmation
	result, err := json.Marshal(map[string]any{
		"status":      "published",
		"exchange":    cfg.Exchange,
		"routing_key": cfg.RoutingKey,
		"timestamp":   now.Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to encode rabbitmq output: %w", err)
	}

	return json.RawMessage(result), nil
}
