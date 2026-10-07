package workflow

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

type WorkflowHandler struct {
	logic WorkflowLogic
}

func NewWorkflowHandler(logic WorkflowLogic) *WorkflowHandler {
	return &WorkflowHandler{logic: logic}
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func (h *WorkflowHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var req WorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	wf, err := h.logic.CreateWorkflow(r.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidWorkflowName),
			errors.Is(err, ErrInvalidWorkflowContext),
			errors.Is(err, step.ErrEmptyStepType),
			errors.Is(err, step.ErrInvalidStepType),
			errors.Is(err, step.ErrInvalidStepConfig),
			errors.Is(err, step.ErrDuplicateStepID),
			errors.Is(err, step.ErrDependencyNotFound),
			errors.Is(err, step.ErrSelfDependency),
			errors.Is(err, step.ErrCyclicDependency),
			errors.Is(err, step.ErrInvalidRetryPolicy),
			errors.Is(err, step.ErrInvalidCompensation),
			errors.Is(err, step.ErrIrreversibleCompensation):
			util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrWorkflowAlreadyExists):
			util.WriteJSON(w, http.StatusConflict, ErrorResponse{Error: err.Error()})
		default:
			util.WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		}
		return
	}

	util.WriteJSON(w, http.StatusCreated, wf)
}

func (h *WorkflowHandler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "workflow id is required"})
		return
	}

	wf, err := h.logic.GetWorkflow(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkflowNotFound):
			util.WriteJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrInvalidWorkflowID):
			util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		default:
			util.WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		}
		return
	}

	util.WriteJSON(w, http.StatusOK, wf)
}

func (h *WorkflowHandler) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "workflow id is required"})
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

	exec, err := h.logic.ExecuteWorkflow(r.Context(), id, &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrWorkflowNotFound):
			util.WriteJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		case errors.Is(err, ErrInvalidWorkflowID),
			errors.Is(err, ErrInvalidWorkflowContext):
			util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		default:
			util.WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
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

func (h *WorkflowHandler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	wfs, err := h.logic.ListWorkflows(r.Context())
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if wfs == nil {
		wfs = []Workflow{}
	}
	util.WriteJSON(w, http.StatusOK, wfs)
}

func (h *WorkflowHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	execs, err := h.logic.ListExecutions(r.Context())
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if execs == nil {
		execs = []step.WorkflowExecution{}
	}
	util.WriteJSON(w, http.StatusOK, execs)
}

func (h *WorkflowHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
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


