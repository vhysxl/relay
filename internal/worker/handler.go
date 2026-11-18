package worker

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type WorkerHandler struct {
	logic          WorkerLogic
	cfg            *config.Config
	credentials    []config.WorkerCredential
	apiEnabled     bool
	maxPayloadSize int64
}

func NewWorkerHandler(logic WorkerLogic, cfg *config.Config) *WorkerHandler {
	apiEnabled := true
	var creds []config.WorkerCredential
	var maxPayloadSize int64 = 5 * 1024 * 1024
	if cfg != nil {
		apiEnabled = cfg.WorkerAPIEnabled
		creds = cfg.WorkerCredentials
		if cfg.MaxPayloadSize > 0 {
			maxPayloadSize = int64(cfg.MaxPayloadSize) * 1024 * 1024
		}
	}
	return &WorkerHandler{
		logic:          logic,
		cfg:            cfg,
		credentials:    creds,
		apiEnabled:     apiEnabled,
		maxPayloadSize: maxPayloadSize,
	}
}

type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func (h *WorkerHandler) writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	code := "INTERNAL_ERROR"

	switch {
	case errors.Is(err, errdefs.ErrWorkerAPIDisabled):
		status = http.StatusForbidden
		code = "WORKER_API_DISABLED"
	case errors.Is(err, errdefs.ErrUnauthorized):
		status = http.StatusUnauthorized
		code = "UNAUTHORIZED"
	case errors.Is(err, errdefs.ErrForbiddenQueue):
		status = http.StatusForbidden
		code = "FORBIDDEN_QUEUE"
	case errors.Is(err, errdefs.ErrForbiddenTask):
		status = http.StatusForbidden
		code = "FORBIDDEN_TASK"
	case errors.Is(err, errdefs.ErrInvalidLeaseToken):
		status = http.StatusUnauthorized
		code = "INVALID_LEASE_TOKEN"
	case errors.Is(err, errdefs.ErrEmptyQueueName),
		errors.Is(err, errdefs.ErrInvalidQueueName),
		errors.Is(err, errdefs.ErrEmptyWorkerID),
		errors.Is(err, errdefs.ErrEmptyStepExecutionID),
		errors.Is(err, errdefs.ErrInvalidWaitTimeout),
		errors.Is(err, errdefs.ErrInvalidStepOutput):
		status = http.StatusBadRequest
		code = "BAD_REQUEST"
	case errors.Is(err, errdefs.ErrStepNotFound):
		status = http.StatusNotFound
		code = "NOT_FOUND"
	case errors.Is(err, errdefs.ErrLockLost):
		status = http.StatusConflict
		code = "LOCK_LOST"
	case errors.Is(err, errdefs.ErrReportConflict):
		status = http.StatusConflict
		code = "REPORT_CONFLICT"
	case errors.Is(err, errdefs.ErrPayloadTooLarge):
		status = http.StatusRequestEntityTooLarge
		code = "PAYLOAD_TOO_LARGE"
	}

	util.WriteJSON(w, status, ErrorResponse{
		Error: err.Error(),
		Code:  code,
	})
}

func (h *WorkerHandler) decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		return true
	}
	if h.maxPayloadSize > 0 {
		if r.ContentLength > h.maxPayloadSize {
			h.writeError(w, errdefs.ErrPayloadTooLarge)
			return false
		}
		r.Body = http.MaxBytesReader(w, r.Body, h.maxPayloadSize)
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeError(w, errdefs.ErrPayloadTooLarge)
			return false
		}
		util.WriteJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid json body", Code: "BAD_REQUEST"})
		return false
	}
	return true
}

func (h *WorkerHandler) authenticate(r *http.Request) (*config.WorkerCredential, error) {
	if !h.apiEnabled {
		return nil, errdefs.ErrWorkerAPIDisabled
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" || !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return nil, errdefs.ErrUnauthorized
	}
	token := strings.TrimSpace(authHeader[7:])
	if token == "" {
		return nil, errdefs.ErrUnauthorized
	}

	for _, cred := range h.credentials {
		if subtle.ConstantTimeCompare([]byte(cred.Token), []byte(token)) == 1 {
			return &cred, nil
		}
	}

	return nil, errdefs.ErrUnauthorized
}

func (h *WorkerHandler) Claim(w http.ResponseWriter, r *http.Request) {
	cred, err := h.authenticate(r)
	if err != nil {
		h.writeError(w, err)
		return
	}

	var req ClaimRequest
	if !h.decodeJSON(w, r, &req) {
		return
	}

	queue := strings.TrimSpace(req.Queue)
	if err := util.ValidateQueueName(queue); err != nil {
		h.writeError(w, err)
		return
	}
	req.Queue = queue
	if !cred.HasQueueAccess(queue) {
		h.writeError(w, errdefs.ErrForbiddenQueue)
		return
	}

	task, err := h.logic.ClaimTask(r.Context(), cred.ID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if task == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	util.WriteJSON(w, http.StatusOK, task)
}

func (h *WorkerHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	cred, err := h.authenticate(r)
	if err != nil {
		h.writeError(w, err)
		return
	}

	taskID := strings.TrimSpace(r.PathValue("id"))
	if taskID == "" {
		h.writeError(w, errdefs.ErrEmptyStepExecutionID)
		return
	}

	var req HeartbeatRequest
	if !h.decodeJSON(w, r, &req) {
		return
	}

	taskQueue, err := h.logic.GetStepTaskQueue(r.Context(), taskID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if !cred.HasQueueAccess(taskQueue) {
		h.writeError(w, errdefs.ErrForbiddenQueue)
		return
	}

	res, err := h.logic.HeartbeatTask(r.Context(), cred.ID, taskID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}

	util.WriteJSON(w, http.StatusOK, res)
}

func (h *WorkerHandler) Complete(w http.ResponseWriter, r *http.Request) {
	cred, err := h.authenticate(r)
	if err != nil {
		h.writeError(w, err)
		return
	}

	taskID := strings.TrimSpace(r.PathValue("id"))
	if taskID == "" {
		h.writeError(w, errdefs.ErrEmptyStepExecutionID)
		return
	}

	var req CompleteRequest
	if !h.decodeJSON(w, r, &req) {
		return
	}

	taskQueue, err := h.logic.GetStepTaskQueue(r.Context(), taskID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if !cred.HasQueueAccess(taskQueue) {
		h.writeError(w, errdefs.ErrForbiddenQueue)
		return
	}

	res, err := h.logic.CompleteTask(r.Context(), cred.ID, taskID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.logic.NotifyQueue(taskQueue)

	util.WriteJSON(w, http.StatusOK, res)
}

func (h *WorkerHandler) Fail(w http.ResponseWriter, r *http.Request) {
	cred, err := h.authenticate(r)
	if err != nil {
		h.writeError(w, err)
		return
	}

	taskID := strings.TrimSpace(r.PathValue("id"))
	if taskID == "" {
		h.writeError(w, errdefs.ErrEmptyStepExecutionID)
		return
	}

	var req FailRequest
	if !h.decodeJSON(w, r, &req) {
		return
	}

	taskQueue, err := h.logic.GetStepTaskQueue(r.Context(), taskID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if !cred.HasQueueAccess(taskQueue) {
		h.writeError(w, errdefs.ErrForbiddenQueue)
		return
	}

	res, err := h.logic.FailTask(r.Context(), cred.ID, taskID, req)
	if err != nil {
		h.writeError(w, err)
		return
	}

	h.logic.NotifyQueue(taskQueue)

	util.WriteJSON(w, http.StatusOK, res)
}
