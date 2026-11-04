package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type ExecutionHandler struct {
	logic          ExecutionLogic
	maxPayloadSize int64
}

func NewExecutionHandler(logic ExecutionLogic, cfg *config.Config) *ExecutionHandler {
	return &ExecutionHandler{
		logic:          logic,
		maxPayloadSize: int64(cfg.MaxPayloadSize) * 1024 * 1024,
	}
}

func (h *ExecutionHandler) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "workflow name is required"})
		return
	}

	keyHeaders := r.Header.Values("Idempotency-Key")
	if len(keyHeaders) == 0 {
		h.writeError(w, errdefs.ErrIdempotencyKeyRequired)
		return
	}
	if len(keyHeaders) > 1 {
		h.writeError(w, errdefs.ErrInvalidIdempotencyKey)
		return
	}
	idempotencyKey := keyHeaders[0]
	if err := ValidateIdempotencyKey(idempotencyKey); err != nil {
		h.writeError(w, err)
		return
	}

	var req ExecuteWorkflowRequest
	req.IdempotencyKey = idempotencyKey

	if r.Body != nil {
		if h.maxPayloadSize > 0 {
			if r.ContentLength > h.maxPayloadSize {
				h.writeError(w, errdefs.ErrPayloadTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, h.maxPayloadSize)
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				h.writeError(w, errdefs.ErrPayloadTooLarge)
			} else {
				util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "failed to read request body"})
			}
			return
		}
		if len(bodyBytes) > 0 {
			trimmed := strings.TrimSpace(string(bodyBytes))
			if trimmed != "" && trimmed != "null" {
				var wrapper map[string]json.RawMessage
				if err := json.Unmarshal(bodyBytes, &wrapper); err == nil {
					if ctxRaw, ok := wrapper["context"]; ok {
						req.Context = ctxRaw
						if vRaw, ok := wrapper["workflow_version"]; ok {
							var v int
							if err := json.Unmarshal(vRaw, &v); err != nil || v < 1 {
								h.writeError(w, errdefs.ErrInvalidWorkflowVersion)
								return
							}
							req.WorkflowVersion = v
						}
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
	if len(bytes.TrimSpace(req.Context)) > 0 {
		if _, err := contextObject(req.Context); err != nil {
			h.writeError(w, err)
			return
		}
	}

	resp, replayed, err := h.logic.ExecuteWorkflow(r.Context(), name, &req)
	if err != nil {
		h.writeError(w, err)
		return
	}

	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	} else {
		w.Header().Set("Idempotency-Replayed", "false")
	}

	util.WriteJSON(w, http.StatusCreated, resp)
}

func (h *ExecutionHandler) ListExecutions(w http.ResponseWriter, r *http.Request) {
	pg := util.ParsePagination(r, 50, 100)
	execs, err := h.logic.ListExecutions(r.Context(), pg.Limit, pg.Offset)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if execs == nil {
		execs = []WorkflowExecution{}
	}
	util.WriteJSON(w, http.StatusOK, execs)
}

func (h *ExecutionHandler) GetExecution(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "execution id is required"})
		return
	}

	exec, err := h.logic.GetExecution(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}
	util.WriteJSON(w, http.StatusOK, exec)
}

func (h *ExecutionHandler) writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "internal server error"
	switch {
	case errors.Is(err, errdefs.ErrWorkflowNotFound), errors.Is(err, errdefs.ErrExecutionNotFound):
		status, message = http.StatusNotFound, err.Error()
	case errors.Is(err, errdefs.ErrInvalidWorkflowName), errors.Is(err, errdefs.ErrInvalidWorkflowContext), errors.Is(err, errdefs.ErrEmptyExecutionID),
		errors.Is(err, errdefs.ErrIdempotencyKeyRequired), errors.Is(err, errdefs.ErrInvalidIdempotencyKey):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, errdefs.ErrIdempotencyConflict):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, errdefs.ErrPayloadTooLarge):
		status, message = http.StatusRequestEntityTooLarge, err.Error()
	default:
		log.Printf("execution request failed: %v", err)
	}
	util.WriteJSON(w, status, ErrorResponse{Error: message})
}
