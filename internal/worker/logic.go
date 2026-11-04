package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
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
func NewWorkerLogic(
	execRepo Repository,
	leaseDuration time.Duration,
	extra ...any,
) WorkerLogic {
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	maxLongPollTimeout := 25 * time.Second
	defaultTimeout := 15 * time.Second
	heartbeatInterval := 10 * time.Second
	var notifier QueueNotifier

	for _, a := range extra {
		switch v := a.(type) {
		case *config.Config:
			if v != nil {
				if v.DefaultLeaseDuration > 0 {
					leaseDuration = v.DefaultLeaseDuration
				}
				if v.WorkerLongPollTimeout > 0 {
					maxLongPollTimeout = v.WorkerLongPollTimeout
				}
				if v.DefaultTimeout > 0 {
					defaultTimeout = v.DefaultTimeout
				}
				if v.DefaultHeartBeatInterval > 0 {
					heartbeatInterval = v.DefaultHeartBeatInterval
				}
			}
		case time.Duration:
			if v > 0 {
				maxLongPollTimeout = v
			}
		case QueueNotifier:
			notifier = v
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
	if queue == "" {
		queue = "legacy"
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
			if res.StepDefinition != nil {
				stepType = string(res.StepDefinition.Type)
				payload = res.StepDefinition.Config
			}
			timeoutSec := res.StepExecution.TimeoutSeconds
			if timeoutSec <= 0 && l.defaultTimeout > 0 {
				timeoutSec = int((l.defaultTimeout - 1) / time.Second) + 1
			}
			if timeoutSec <= 0 {
				timeoutSec = 15
			}
			return &ClaimTaskResponse{
				StepExecutionID:     res.StepExecution.ID,
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

func (l *workerLogic) ClaimNext(ctx context.Context, workerID string) (*step.Step, *execution.StepExecution, error) {
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return nil, nil, errdefs.ErrEmptyWorkerID
	}
	return l.execRepo.ClaimPending(ctx, trimmedWorkerID, l.leaseDuration)
}

func (l *workerLogic) Heartbeat(ctx context.Context, stepExecutionID string, workerID string) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return errdefs.ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return errdefs.ErrEmptyWorkerID
	}
	return l.execRepo.Heartbeat(ctx, trimmedExecID, trimmedWorkerID, l.leaseDuration)
}

func (l *workerLogic) StartHeartbeat(ctx context.Context, stepExecutionID string, workerID string, interval time.Duration) (func(), <-chan error) {
	stopChan := make(chan struct{})
	errChan := make(chan error, 1)
	var once sync.Once

	stop := func() {
		once.Do(func() {
			close(stopChan)
		})
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := l.Heartbeat(ctx, stepExecutionID, workerID); err != nil {
					select {
					case errChan <- err:
					default:
					}
					return
				}
			}
		}
	}()

	return stop, errChan
}

func (l *workerLogic) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return errdefs.ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return errdefs.ErrEmptyWorkerID
	}
	if len(output) > 0 {
		trimmedOutput := bytes.TrimSpace(output)
		if len(trimmedOutput) > 0 && !bytes.Equal(trimmedOutput, []byte("null")) && !json.Valid(output) {
			return errdefs.ErrInvalidStepOutput
		}
	}
	return l.execRepo.CompleteStep(ctx, trimmedExecID, trimmedWorkerID, output)
}

func (l *workerLogic) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	trimmedExecID := strings.TrimSpace(stepExecutionID)
	if trimmedExecID == "" {
		return errdefs.ErrEmptyStepExecutionID
	}
	trimmedWorkerID := strings.TrimSpace(workerID)
	if trimmedWorkerID == "" {
		return errdefs.ErrEmptyWorkerID
	}
	return l.execRepo.FailStep(ctx, trimmedExecID, trimmedWorkerID, errMsg)
}
