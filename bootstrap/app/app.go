package app

import (
	"fmt"
	"log"
	"net/url"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/executor"
	"github.com/vhysxl/relay/internal/reaper"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/validator"
	"github.com/vhysxl/relay/internal/worker"
	"github.com/vhysxl/relay/internal/workflow"
)

type Application struct {
	Config *config.Config

	// Services
	WorkflowLogic  workflow.WorkflowLogic
	ExecutionLogic execution.ExecutionLogic
	WorkerLogic    worker.WorkerLogic

	// Background services (started by main.go via g.Go)
	Pool   *worker.WorkerPool
	Reaper reaper.Reaper

	// Validator
	Validation validator.Validator

	// Cleanup
	closeRMQ func() // no-op when RabbitMQ unused
}

func (a *Application) Close() error {
	if a.closeRMQ != nil {
		a.closeRMQ()
	}
	if a.Config != nil && a.Config.DB != nil {
		return a.Config.DB.Close()
	}
	return nil
}

func InitApp() (*Application, error) {
	conf, err := config.Init()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	reg := executor.NewRegistry()
	reg.Register(step.StepTypeHTTP, executor.NewHTTPExecutor(nil))

	closeRMQ, err := registerRabbitMQ(reg)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: %w", err)
	}

	validation := validator.NewValidator()
	stepRepo := step.NewStepRepository(conf.DB)
	executionRepo := execution.NewExecutionRepository(conf.DB, stepRepo)
	workflowRepo := workflow.NewWorkflowRepository(conf.DB, stepRepo)

	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, validation, conf.ConcurrentRetryLimit)
	executionLogic := execution.NewExecutionLogic(executionRepo)
	workerLogic := worker.NewWorkerLogic(executionRepo, conf.DefaultLeaseDuration)

	pool := worker.NewWorkerPool(
		workerLogic,
		reg,
		worker.WithBaseID("server-worker"),
		worker.WithWorkerCount(conf.WorkerCount),
		worker.WithPollInterval(conf.DefaultPoolInterval),
		worker.WithHeartbeatInterval(conf.DefaultHeartBeatInterval),
	)
	reaperSvc := reaper.New(executionRepo, conf.ReaperInterval)

	return &Application{
		Config:         conf,
		WorkflowLogic:  workflowLogic,
		ExecutionLogic: executionLogic,
		WorkerLogic:    workerLogic,
		Pool:           pool,
		Reaper:         reaperSvc,
		Validation:     validation,
		closeRMQ:       closeRMQ,
	}, nil
}

// registerRabbitMQ wires the message executor if RABBITMQ_URL is set.
// Returns (no-op closer, nil) when URL is empty.
// Returns error (with redacted URL) if URL is set but connection fails.
func registerRabbitMQ(reg executor.Registry) (func(), error) {
	rmqURL := os.Getenv("RABBITMQ_URL")
	if rmqURL == "" {
		return func() {}, nil
	}

	// Redact credentials before using URL in error messages
	redacted := rmqURL
	if u, err := url.Parse(rmqURL); err == nil {
		redacted = u.Redacted()
	}

	conn, err := amqp.Dial(rmqURL)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", redacted, err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open channel at %s: %w", redacted, err)
	}

	reg.Register(step.StepTypeMessage, executor.NewRabbitMQExecutor(ch))
	log.Println("RabbitMQ message executor registered successfully")
	return func() {
		ch.Close()
		conn.Close()
	}, nil
}
