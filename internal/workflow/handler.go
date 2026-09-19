package workflow

import (
	"encoding/json"
	"errors"
	"net/http"
	"task/queue/pkg/util"
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
		case errors.Is(err, ErrInvalidWorkflowName), errors.Is(err, ErrInvalidWorkflowContext):
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
