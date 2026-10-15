package api

import (
	"net/http"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/system"
	"github.com/vhysxl/relay/internal/validator"
	"github.com/vhysxl/relay/internal/web"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/util"
)

func RegisterRoutes(mux *http.ServeMux, conf *config.Config) {
	stepRepo := step.NewStepRepository(conf.DB)
	v := validator.NewValidator()

	workflowRepo := workflow.NewWorkflowRepository(conf.DB, stepRepo)
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, v)
	workflowHandler := workflow.NewWorkflowHandler(workflowLogic, conf)

	executionRepo := execution.NewExecutionRepository(conf.DB, stepRepo)
	executionLogic := execution.NewExecutionLogic(executionRepo)
	executionHandler := execution.NewExecutionHandler(executionLogic)

	// Web Dashboard UI & Static Assets (SPA Fallback)
	mux.HandleFunc("GET /", web.Handler())

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		util.WriteJSON(w, http.StatusOK, "Task Queue Server Online")
	})

	mux.HandleFunc("GET /system/metrics", func(w http.ResponseWriter, r *http.Request) {
		util.WriteJSON(w, http.StatusOK, system.GetMetrics())
	})

	// Workflow Definitions
	mux.HandleFunc("GET /workflows", workflowHandler.ListWorkflows)
	mux.HandleFunc("POST /workflows", workflowHandler.CreateWorkflow)
	mux.HandleFunc("GET /workflows/{name}", workflowHandler.GetWorkflow)

	// Workflow Executions
	mux.HandleFunc("POST /workflows/{name}/execute", executionHandler.ExecuteWorkflow)
	mux.HandleFunc("GET /executions", executionHandler.ListExecutions)
	mux.HandleFunc("GET /executions/{id}", executionHandler.GetExecution)
}
