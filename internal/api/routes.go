package api

import (
	"database/sql"
	"net/http"
	"github.com/vhysxl/relay/internal/workflow"
	"github.com/vhysxl/relay/pkg/util"
)

func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	workflowRepo := workflow.NewWorkflowRepository(db)
	workflowLogic := workflow.NewWorkflowLogic(workflowRepo)
	workflowHandler := workflow.NewWorkflowHandler(workflowLogic)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		util.WriteJSON(w, http.StatusOK, "Task Queue Server Online")
	})

	mux.HandleFunc("POST /workflows", workflowHandler.CreateWorkflow)
	mux.HandleFunc("GET /workflows/{id}", workflowHandler.GetWorkflow)
}
