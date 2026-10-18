package worker

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/vhysxl/relay/internal/execution"
	"github.com/vhysxl/relay/internal/executor"
	"github.com/vhysxl/relay/internal/step"
)

type workerPool struct {
	baseID            string
	workerCount       int
	pollInterval      time.Duration
	heartbeatInterval time.Duration
	logic             WorkerLogic
	registry          executor.Registry
}

// NewWorkerPool creates a new WorkerPool with the provided dependencies and configuration.
func NewWorkerPool(
	logic WorkerLogic,
	registry executor.Registry,
	baseID string,
	workerCount int,
	pollInterval time.Duration,
	heartbeatInterval time.Duration,
) WorkerPool {
	return &workerPool{
		baseID:            baseID,
		workerCount:       workerCount,
		pollInterval:      pollInterval,
		heartbeatInterval: heartbeatInterval,
		logic:             logic,
		registry:          registry,
	}
}

// Start launches worker goroutines and blocks until ctx is cancelled.
func (p *workerPool) Start(ctx context.Context) error {
	var wg sync.WaitGroup

	for i := 1; i <= p.workerCount; i++ {
		workerID := fmt.Sprintf("%s-%d", p.baseID, i)
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			p.runWorker(ctx, id)
		}(workerID)
	}

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

func (p *workerPool) runWorker(ctx context.Context, workerID string) {
	timer := time.NewTimer(p.pollInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		processed, err := p.ProcessOne(ctx, workerID)
		if err != nil && ctx.Err() == nil {
			log.Printf("[worker %s] error processing step: %v", workerID, err)
		}

		if !processed || err != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(p.pollInterval)

			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
	}
}

// ProcessOne claims and executes a single task for the given workerID.
func (p *workerPool) ProcessOne(ctx context.Context, workerID string) (bool, error) {
	s, exec, err := p.logic.ClaimNext(ctx, workerID)
	if err != nil {
		return false, err
	}
	if exec == nil {
		return false, nil
	}

	p.executeTask(ctx, workerID, s, exec)
	return true, nil
}

func (p *workerPool) executeTask(ctx context.Context, workerID string, s *step.Step, exec *execution.StepExecution) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[worker %s] panic during step execution %s: %v\n%s", workerID, exec.ID, r, debug.Stack())
			cleanupCtx, cancel := cleanupContext(ctx, 5*time.Second)
			defer cancel()
			_ = p.logic.FailStep(cleanupCtx, exec.ID, workerID, fmt.Sprintf("panic in step execution: %v", r))
		}
	}()

	// Determine timeout if configured on ticket or step definition
	timeoutSeconds := exec.TimeoutSeconds
	if timeoutSeconds <= 0 && s != nil && s.Timeout > 0 {
		timeoutSeconds = s.Timeout
	}

	var (
		taskCtx    context.Context
		taskCancel context.CancelFunc
	)
	if timeoutSeconds > 0 {
		taskCtx, taskCancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	} else {
		taskCtx, taskCancel = context.WithCancel(ctx)
	}
	defer taskCancel()

	// Start background lease renewal heartbeat
	stopHB, hbErrChan := p.logic.StartHeartbeat(ctx, exec.ID, workerID, p.heartbeatInterval)
	defer stopHB()

	var (
		lockLostMu sync.Mutex
		lockLost   bool
	)

	// Monitor heartbeat failures (e.g. lease expired or lost to reaper/another worker)
	go func() {
		select {
		case err, ok := <-hbErrChan:
			if ok && err != nil {
				lockLostMu.Lock()
				lockLost = true
				lockLostMu.Unlock()
				taskCancel()
			}
		case <-taskCtx.Done():
		}
	}()

	stepType := step.StepType(exec.Type)
	if stepType == "" && s != nil {
		stepType = s.Type
	}

	execHandler, err := p.registry.Get(stepType)
	if err != nil {
		cleanupCtx, cancel := cleanupContext(ctx, 5*time.Second)
		defer cancel()
		_ = p.logic.FailStep(cleanupCtx, exec.ID, workerID, fmt.Sprintf("unsupported step type: %v", err))
		return
	}

	output, err := execHandler.Execute(taskCtx, s, exec)
	stopHB()

	lockLostMu.Lock()
	isLockLost := lockLost
	lockLostMu.Unlock()

	// If lock was lost during execution, do not attempt to complete or overwrite step state
	if isLockLost {
		log.Printf("[worker %s] step %s execution cancelled or completed after lock was lost", workerID, exec.ID)
		return
	}

	cleanupCtx, cancel := cleanupContext(ctx, 5*time.Second)
	defer cancel()

	if err != nil {
		_ = p.logic.FailStep(cleanupCtx, exec.ID, workerID, err.Error())
	} else {
		_ = p.logic.CompleteStep(cleanupCtx, exec.ID, workerID, output)
	}
}

func cleanupContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx.Err() != nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithTimeout(ctx, timeout)
}
