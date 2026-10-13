package execution

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type ExecutionHandler struct {
	logic ExecutionLogic
}

func NewExecutionHandler(logic ExecutionLogic) *ExecutionHandler {
	return &ExecutionHandler{
		logic: logic,
	}
}

func (h *ExecutionHandler) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "workflow name is required"})
		return
	}

	var req ExecuteWorkflowRequest
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			trimmed := strings.TrimSpace(string(bodyBytes))
			if trimmed != "" && trimmed != "null" {
				var wrapper map[string]json.RawMessage
				if err := json.Unmarshal(bodyBytes, &wrapper); err == nil {
					if ctxRaw, ok := wrapper["context"]; ok {
						req.Context = ctxRaw
					} else {
						req.Context = json.RawMessage(bodyBytes)
					}
				} else {
					util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid json request body"})
					return
				}
			}
		}
	}

	exec, err := h.logic.ExecuteWorkflow(r.Context(), name, &req)
	if err != nil {
		switch {
		case errors.Is(err, errdefs.ErrWorkflowNotFound):
			util.WriteJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		}
		return
	}

	resp := ExecuteWorkflowResponse{
		ID:         exec.ID,
		WorkflowID: exec.WorkflowID,
		Status:     string(exec.Status),
		Context:    exec.Context,
		TotalSteps: len(exec.Steps),
		CreatedAt:  exec.CreatedAt,
	}

	util.WriteJSON(w, http.StatusCreated, resp)
}

func (h *ExecutionHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	pg := util.ParsePagination(r, 50, 100)
	execs, err := h.logic.ListExecutions(r.Context(), pg.Limit, pg.Offset)
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if execs == nil {
		execs = []step.WorkflowExecution{}
	}
	util.WriteJSON(w, http.StatusOK, execs)
}

func (h *ExecutionHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "execution id is required"})
		return
	}

	exec, err := h.logic.GetExecution(r.Context(), id)
	if err != nil {
		util.WriteJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		return
	}
	util.WriteJSON(w, http.StatusOK, exec)
}
