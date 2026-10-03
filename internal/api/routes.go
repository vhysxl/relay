package api

import (
	"database/sql"
	"net/http"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/web"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/util"
)

func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	stepRepo := step.NewStepRepository(db)
	stepValidator := step.NewStepValidator()

	workflowRepo := workflow.NewWorkflowRepository(db, stepRepo)
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo, stepValidator)
	workflowHandler := workflow.NewWorkflowHandler(workflowLogic)

	// Web Dashboard UI
	mux.HandleFunc("GET /{$}", web.Handler())
	mux.HandleFunc("GET /dashboard", web.Handler())
	mux.HandleFunc("GET /history", web.Handler())

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		util.WriteJSON(w, http.StatusOK, "Task Queue Server Online")
	})

	mux.HandleFunc("GET /workflows", workflowHandler.ListWorkflows)
	mux.HandleFunc("POST /workflows", workflowHandler.CreateWorkflow)
	mux.HandleFunc("GET /workflows/{id}", workflowHandler.GetWorkflow)
	mux.HandleFunc("POST /workflows/{id}/execute", workflowHandler.ExecuteWorkflow)

	mux.HandleFunc("GET /executions", workflowHandler.ListExecutions)
	mux.HandleFunc("GET /executions/{id}", workflowHandler.GetExecution)
}

