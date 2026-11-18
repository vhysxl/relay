package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"strings"
	"time"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type workerLogic struct {
	execRepo           Repository
	leaseDuration      time.Duration
	defaultWaitTimeout time.Duration
	maxLongPollTimeout time.Duration
	defaultTimeout     time.Duration
	heartbeatInterval  time.Duration
	notifier           QueueNotifier
}

// NewWorkerLogic creates a new WorkerLogic for external worker operations.
func NewWorkerLogic(execRepo Repository, cfg *config.Config, notifier QueueNotifier) WorkerLogic {
	leaseDuration := 30 * time.Second
	maxLongPollTimeout := 25 * time.Second
	defaultTimeout := 15 * time.Second
	heartbeatInterval := 10 * time.Second
	if cfg != nil {
		if cfg.DefaultLeaseDuration > 0 {
			leaseDuration = cfg.DefaultLeaseDuration
		}
		if cfg.WorkerLongPollTimeout > 0 {
			maxLongPollTimeout = cfg.WorkerLongPollTimeout
		}
		if cfg.DefaultTimeout > 0 {
			defaultTimeout = cfg.DefaultTimeout
		}
		if cfg.DefaultHeartBeatInterval > 0 {
			heartbeatInterval = cfg.DefaultHeartBeatInterval
		}
	}
	if notifier == nil {
		notifier = NewQueueNotifier()
	}
	defaultWaitTimeout := 25 * time.Second
	if defaultWaitTimeout > maxLongPollTimeout {
		defaultWaitTimeout = maxLongPollTimeout
	}
	return &workerLogic{
		execRepo:           execRepo,
		leaseDuration:      leaseDuration,
		defaultWaitTimeout: defaultWaitTimeout,
		maxLongPollTimeout: maxLongPollTimeout,
		defaultTimeout:     defaultTimeout,
		heartbeatInterval:  heartbeatInterval,
		notifier:           notifier,
	}
}

func (l *workerLogic) NotifyQueue(queue string) {
	if l.notifier != nil {
		l.notifier.Notify(queue)
	}
}

func (l *workerLogic) GetStepTaskQueue(ctx context.Context, stepExecID string) (string, error) {
	return l.execRepo.GetStepTaskQueue(ctx, stepExecID)
}

func (l *workerLogic) ClaimTask(ctx context.Context, serviceID string, req ClaimRequest) (*ClaimTaskResponse, error) {
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, errdefs.ErrEmptyWorkerID
	}

	queue := strings.TrimSpace(req.Queue)
	if err := util.ValidateQueueName(queue); err != nil {
		return nil, err
	}

	// Validate wait_timeout_ms
	waitTimeout := l.defaultWaitTimeout
	if req.WaitTimeoutMS != nil {
		ms := *req.WaitTimeoutMS
		if ms < 0 || time.Duration(ms)*time.Millisecond > l.maxLongPollTimeout {
			return nil, errdefs.ErrInvalidWaitTimeout
		}
		waitTimeout = time.Duration(ms) * time.Millisecond
	} else if waitTimeout > l.maxLongPollTimeout {
		waitTimeout = l.maxLongPollTimeout
	}

	claimFilter := execution.ClaimFilter{
		ServiceID:      serviceID,
		WorkerID:       workerID,
		Queue:          queue,
		ExecutorTypes:  req.ExecutorTypes,
		Activities:     req.Activities,
		LeaseDuration:  l.leaseDuration,
		DefaultTimeout: l.defaultTimeout,
	}

	// Subscribe for in-memory queue wakeups
	notifyCh, cancelSub := l.notifier.Subscribe(queue)
	defer cancelSub()

	deadline := time.Now().Add(waitTimeout)

	for {
		// Attempt claim
		res, err := l.execRepo.ClaimTask(ctx, claimFilter)
		if err != nil {
			return nil, err
		}
		if res != nil {
			var actName string
			if res.StepExecution.ActivityName != nil {
				actName = *res.StepExecution.ActivityName
			}
			stepType := string(res.StepExecution.Type)
			payload := res.StepExecution.Config
			timeoutSec := res.StepExecution.TimeoutSeconds
			if timeoutSec <= 0 && l.defaultTimeout > 0 {
				timeoutSec = int((l.defaultTimeout-1)/time.Second) + 1
			}
			if timeoutSec <= 0 {
				timeoutSec = 15
			}
			return &ClaimTaskResponse{
				TaskID:              res.StepExecution.ID,
				WorkflowExecutionID: res.StepExecution.WorkflowExecutionID,
				StepID:              res.StepExecution.StepID,
				Type:                stepType,
				TaskQueue:           res.StepExecution.TaskQueue,
				ActivityName:        actName,
				Attempt:             res.StepExecution.Attempt,
				IsCompensation:      res.StepExecution.IsCompensation,
				TimeoutSeconds:      timeoutSec,
				Deadline:            res.Deadline.UTC().Format(time.RFC3339Nano),
				HeartbeatIntervalMS: l.heartbeatInterval.Milliseconds(),
				LeaseToken:          res.LeaseToken,
				LeaseExpiresAt:      res.LeaseExpiresAt.UTC().Format(time.RFC3339Nano),
				LeaseDurationMS:     res.LeaseDuration.Milliseconds(),
				Payload:             payload,
			}, nil
		}

		if waitTimeout == 0 || time.Now().After(deadline) {
			return nil, nil // HTTP 204
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil // HTTP 204
		}

		// Interval wait: 500ms + random jitter (0-200ms), capped at remaining
		tickDuration := 500*time.Millisecond + time.Duration(rand.Intn(200))*time.Millisecond
		if tickDuration > remaining {
			tickDuration = remaining
		}

		timer := time.NewTimer(tickDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-notifyCh:
			timer.Stop()
			// Immediate wakeup on queue activity! Loop around and claim.
		case <-timer.C:
			// Interval elapsed, loop and try claiming.
		}
	}
}

func (l *workerLogic) HeartbeatTask(ctx context.Context, serviceID, taskID string, req HeartbeatRequest) (*HeartbeatResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errdefs.ErrEmptyStepExecutionID
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, errdefs.ErrEmptyWorkerID
	}
	leaseToken := strings.TrimSpace(req.LeaseToken)
	if leaseToken == "" {
		return nil, errdefs.ErrInvalidLeaseToken
	}

	leaseDuration := l.leaseDuration
	if req.LeaseDurationMS > 0 {
		requested := time.Duration(req.LeaseDurationMS) * time.Millisecond
		if requested > 0 && requested < leaseDuration {
			leaseDuration = requested
		}
	}

	expiresAt, duration, err := l.execRepo.HeartbeatTask(ctx, serviceID, taskID, workerID, leaseToken, leaseDuration)
	if err != nil {
		return nil, err
	}

	return &HeartbeatResponse{
		LeaseExpiresAt:  expiresAt.UTC().Format(time.RFC3339Nano),
		LeaseDurationMS: duration.Milliseconds(),
	}, nil
}

func (l *workerLogic) CompleteTask(ctx context.Context, serviceID, taskID string, req CompleteRequest) (*CompleteResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errdefs.ErrEmptyStepExecutionID
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, errdefs.ErrEmptyWorkerID
	}
	leaseToken := strings.TrimSpace(req.LeaseToken)
	if leaseToken == "" {
		return nil, errdefs.ErrInvalidLeaseToken
	}

	output := bytes.TrimSpace(req.Output)
	if len(output) == 0 || bytes.Equal(output, []byte("null")) {
		output = json.RawMessage(`{}`)
	}
	if !json.Valid(output) {
		return nil, errdefs.ErrInvalidStepOutput
	}

	if err := l.execRepo.CompleteTask(ctx, serviceID, taskID, workerID, leaseToken, output); err != nil {
		return nil, err
	}

	return &CompleteResponse{Accepted: true}, nil
}

func (l *workerLogic) FailTask(ctx context.Context, serviceID, taskID string, req FailRequest) (*FailResponse, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errdefs.ErrEmptyStepExecutionID
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, errdefs.ErrEmptyWorkerID
	}
	leaseToken := strings.TrimSpace(req.LeaseToken)
	if leaseToken == "" {
		return nil, errdefs.ErrInvalidLeaseToken
	}

	errMsg := strings.TrimSpace(req.Error.Message)
	if errMsg == "" {
		errMsg = "task failed without specific error message"
	}

	retryable := true
	if req.Error.Retryable != nil {
		retryable = *req.Error.Retryable
	}

	if err := l.execRepo.FailTask(ctx, serviceID, taskID, workerID, leaseToken, errMsg, retryable); err != nil {
		return nil, err
	}

	return &FailResponse{Accepted: true}, nil
}
