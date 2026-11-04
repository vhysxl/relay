package app

import (
	"fmt"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
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

	// Background services
	Reaper reaper.Reaper

	// Validator
	Validation validator.Validator
}

func (a *Application) Close() error {
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

	validation := validator.NewValidatorWithLimits(conf.ConditionMaxDepth, conf.ConditionMaxNodes)
	stepRepo := step.NewStepRepository(conf.DB)
	executionRepo := execution.NewExecutionRepository(conf.DB, conf)
	workflowRepo := workflow.NewWorkflowRepository(conf.DB, stepRepo)

	queueNotifier := worker.NewQueueNotifier()
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, validation, conf.ConcurrentRetryLimit)
	executionLogic := execution.NewExecutionLogic(executionRepo, workflowRepo)
	workerLogic := worker.NewWorkerLogic(executionRepo, conf.DefaultLeaseDuration, conf, queueNotifier)

	reaperSvc := reaper.New(executionRepo, conf.ReaperInterval)

	return &Application{
		Config:         conf,
		WorkflowLogic:  workflowLogic,
		ExecutionLogic: executionLogic,
		WorkerLogic:    workerLogic,
		Reaper:         reaperSvc,
		Validation:     validation,
	}, nil
}
