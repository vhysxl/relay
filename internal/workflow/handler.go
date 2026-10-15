package workflow

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type WorkflowHandler struct {
	logic          WorkflowLogic
	maxPayloadSize int64
}

func NewWorkflowHandler(logic WorkflowLogic, cfg *config.Config) *WorkflowHandler {
	return &WorkflowHandler{
		logic:          logic,
		maxPayloadSize: int64(cfg.MaxPayloadSize) * 1024 * 1024,
	}
}

func (h *WorkflowHandler) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	if h.maxPayloadSize > 0 && r.Body != nil {
		if r.ContentLength > h.maxPayloadSize {
			util.WriteJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{Error: errdefs.ErrPayloadTooLarge.Error()})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, h.maxPayloadSize)
	}

	var req WorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			util.WriteJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{Error: errdefs.ErrPayloadTooLarge.Error()})
			return
		}
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	wf, err := h.logic.CreateWorkflow(r.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, errdefs.ErrWorkflowAlreadyExists):
			util.WriteJSON(w, http.StatusConflict, ErrorResponse{Error: err.Error()})
		default:
			util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		}
		return
	}

	util.WriteJSON(w, http.StatusCreated, wf)
}

func (h *WorkflowHandler) GetWorkflow(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "workflow name is required"})
		return
	}

	wf, err := h.logic.GetWorkflow(r.Context(), name)
	if err != nil {
		switch {
		case errors.Is(err, errdefs.ErrWorkflowNotFound):
			util.WriteJSON(w, http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		}
		return
	}

	util.WriteJSON(w, http.StatusOK, wf)
}

func (h *WorkflowHandler) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	pg := util.ParsePagination(r, 50, 100)
	wfs, err := h.logic.ListWorkflows(r.Context(), pg.Limit, pg.Offset)
	if err != nil {
		util.WriteJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if wfs == nil {
		wfs = []Workflow{}
	}
	util.WriteJSON(w, http.StatusOK, wfs)
}
