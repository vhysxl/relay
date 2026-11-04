package api

import (
	"net/http"

	"github.com/vhysxl/relay/bootstrap/app"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/system"
	"github.com/vhysxl/relay/internal/web"
	"github.com/vhysxl/relay/internal/worker"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/util"
)

func RegisterRoutes(mux *http.ServeMux, app *app.Application) {

	workflowHandler := workflow.NewWorkflowHandler(app.WorkflowLogic, app.Config)
	executionHandler := execution.NewExecutionHandler(app.ExecutionLogic, app.Config)
	workerHandler := worker.NewWorkerHandler(app.WorkerLogic, app.Config)

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
	mux.HandleFunc("GET /workflows/{name}/versions", workflowHandler.ListWorkflowVersions)
	mux.HandleFunc("GET /workflows/{name}/versions/{version}", workflowHandler.GetWorkflowVersion)
	mux.HandleFunc("GET /workflows/{name}/versions/{version}/changes", workflowHandler.GetWorkflowVersionChanges)

	// Workflow Executions
	mux.HandleFunc("POST /workflows/{name}/execute", executionHandler.ExecuteWorkflow)
	mux.HandleFunc("GET /executions", executionHandler.ListExecutions)
	mux.HandleFunc("GET /executions/{id}", executionHandler.GetExecution)

	// Worker Tasks Protocol
	mux.HandleFunc("POST /worker/tasks/claim", workerHandler.Claim)
	mux.HandleFunc("POST /worker/tasks/{id}/heartbeat", workerHandler.Heartbeat)
	mux.HandleFunc("POST /worker/tasks/{id}/complete", workerHandler.Complete)
	mux.HandleFunc("POST /worker/tasks/{id}/fail", workerHandler.Fail)
}
