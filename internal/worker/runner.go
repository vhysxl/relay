package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vhysxl/relay/internal/executor"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

const (
	DefaultWorkerCount = 3
	DefaultPollInterval = 1 * time.Second
)

type WorkerOption func(*WorkerPool)

type WorkerPool struct {
	baseID            string
	workerCount       int
	pollInterval      time.Duration
	heartbeatInterval time.Duration
	logic             WorkerLogic
	registry          executor.Registry
}

// NewWorkerPool creates a worker pool with a fixed number of workers (default: 3)
func NewWorkerPool(logic WorkerLogic, registry executor.Registry, opts ...WorkerOption) *WorkerPool {
	p := &WorkerPool{
		baseID:            "worker-" + util.GenerateUUID()[:8],
		workerCount:       DefaultWorkerCount,
		pollInterval:      DefaultPollInterval,
		heartbeatInterval: DefaultHeartbeatInterval,
		logic:             logic,
		registry:          registry,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func WithWorkerCount(count int) WorkerOption {
	return func(p *WorkerPool) {
		if count > 0 {
			p.workerCount = count
		}
	}
}

func WithBaseID(baseID string) WorkerOption {
	return func(p *WorkerPool) {
		if baseID != "" {
			p.baseID = baseID
		}
	}
}

func WithPollInterval(d time.Duration) WorkerOption {
	return func(p *WorkerPool) {
		if d > 0 {
			p.pollInterval = d
		}
	}
}

func WithHeartbeatInterval(d time.Duration) WorkerOption {
	return func(p *WorkerPool) {
		if d > 0 {
			p.heartbeatInterval = d
		}
	}
}

// Start launches fixed worker goroutines and blocks until ctx is cancelled
func (p *WorkerPool) Start(ctx context.Context) error {
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

func (p *WorkerPool) runWorker(ctx context.Context, workerID string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			processed, err := p.ProcessOne(ctx, workerID)
			if err != nil || !processed {
				select {
				case <-ctx.Done():
					return
				case <-time.After(p.pollInterval):
					continue
				}
			}
		}
	}
}

// ProcessOne claims and executes a single task for the given workerID (useful for deterministic tests)
func (p *WorkerPool) ProcessOne(ctx context.Context, workerID string) (bool, error) {
	if p.logic == nil {
		return false, fmt.Errorf("worker logic is not configured")
	}

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

func (p *WorkerPool) executeTask(ctx context.Context, workerID string, s *step.Step, exec *step.StepExecution) {
	defer func() {
		if r := recover(); r != nil {
			cleanupCtx := ctx
			if ctx.Err() != nil {
				var cancel context.CancelFunc
				cleanupCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
			}
			_ = p.logic.FailStep(cleanupCtx, exec.ID, workerID, fmt.Sprintf("panic in step execution: %v", r))
		}
	}()

	stopHB, _ := p.logic.StartHeartbeat(ctx, exec.ID, workerID, p.heartbeatInterval)
	defer stopHB()

	stepType := step.StepType(exec.Type)
	if stepType == "" && s != nil {
		stepType = s.Type
	}

	if p.registry == nil {
		_ = p.logic.FailStep(ctx, exec.ID, workerID, "executor registry is not configured")
		return
	}

	execHandler, err := p.registry.Get(stepType)
	if err != nil {
		_ = p.logic.FailStep(ctx, exec.ID, workerID, fmt.Sprintf("unsupported step type: %v", err))
		return
	}

	output, err := execHandler.Execute(ctx, s, exec)
	stopHB()

	cleanupCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		cleanupCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}

	if err != nil {
		_ = p.logic.FailStep(cleanupCtx, exec.ID, workerID, err.Error())
	} else {
		_ = p.logic.CompleteStep(cleanupCtx, exec.ID, workerID, output)
	}
}
