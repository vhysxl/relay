package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/util"
)

var (
	ErrWorkflowNotFound      = errors.New("workflow not found")
	ErrWorkflowAlreadyExists = errors.New("workflow with this name and version already exists")
)

type workflowRepository struct {
	db       *sql.DB
	stepRepo step.StepRepository
}

func NewWorkflowRepository(db *sql.DB, stepRepo step.StepRepository) WorkflowRepository {
	return &workflowRepository{
		db:       db,
		stepRepo: stepRepo,
	}
}

func (r *workflowRepository) CreateWorkflow(ctx context.Context, wf *Workflow) (*Workflow, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO workflow_definitions (id, name, version, status, hash, context, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	var contextVal any
	if len(wf.Context) > 0 {
		contextVal = string(wf.Context)
	} else {
		contextVal = nil
	}

	_, err = tx.ExecContext(
		ctx,
		query,
		wf.ID,
		wf.Name,
		wf.Version,
		string(wf.Status),
		wf.Hash,
		contextVal,
		wf.CreatedAt,
		wf.UpdatedAt,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, ErrWorkflowAlreadyExists
		}
		return nil, err
	}

	if len(wf.Steps) > 0 && r.stepRepo != nil {
		if err := r.stepRepo.CreateStepsTx(ctx, tx, wf.ID, wf.Steps); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return wf, nil
}

func (r *workflowRepository) GetWorkflow(ctx context.Context, workflowID string) (*Workflow, error) {
	query := `
		SELECT id, name, version, status, hash, context, created_at, updated_at
		FROM workflow_definitions
		WHERE id = ?
	`

	var (
		wf          Workflow
		statusStr   string
		contextData sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, workflowID).Scan(
		&wf.ID,
		&wf.Name,
		&wf.Version,
		&statusStr,
		&wf.Hash,
		&contextData,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkflowNotFound
		}
		return nil, err
	}

	wf.Status = WorkflowStatus(statusStr)
	if contextData.Valid && len(contextData.String) > 0 {
		wf.Context = json.RawMessage(contextData.String)
	}

	if r.stepRepo != nil {
		steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, wf.ID)
		if err != nil {
			return nil, err
		}
		wf.Steps = steps
	}

	return &wf, nil
}

func (r *workflowRepository) ExecuteWorkflowTx(ctx context.Context, workflowID string, initialContext json.RawMessage) (*step.WorkflowExecution, error) {
	// 1. Verify workflow definition exists and get default context
	var (
		defaultContext sql.NullString
		wfStatus       string
	)
	checkQuery := `
		SELECT status, context
		FROM workflow_definitions
		WHERE id = ?
	`
	err := r.db.QueryRowContext(ctx, checkQuery, workflowID).Scan(&wfStatus, &defaultContext)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("failed to query workflow definition: %w", err)
	}

	// 2. Resolve effective context
	var effectiveContext []byte
	if len(initialContext) > 0 && strings.TrimSpace(string(initialContext)) != "" && string(initialContext) != "null" {
		effectiveContext = initialContext
	} else if defaultContext.Valid && len(defaultContext.String) > 0 && defaultContext.String != "null" {
		effectiveContext = []byte(defaultContext.String)
	} else {
		effectiveContext = []byte("{}")
	}

	// 3. Begin transaction
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 4. Create workflow execution record
	wfExecID := util.GenerateUUID()
	now := time.Now().UTC()

	insertWfExecQuery := `
		INSERT INTO workflow_executions (id, workflow_id, status, context, created_at, updated_at)
		VALUES (?, ?, 'RUNNING', ?, ?, ?)
	`
	_, err = tx.ExecContext(ctx, insertWfExecQuery, wfExecID, workflowID, string(effectiveContext), now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert workflow execution: %w", err)
	}

	// 5. Fetch step definitions
	if r.stepRepo == nil {
		return nil, errors.New("step repository is not configured")
	}
	steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch step definitions: %w", err)
	}

	// 6. Insert step executions
	stepExecs := make([]step.StepExecution, 0, len(steps))

	insertStepExecQuery := `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on,
			status, attempt, max_attempts, timeout_seconds, backoff_seconds,
			multiplier, max_backoff_seconds, is_compensation, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', 0, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, s := range steps {
		stepExecID := util.GenerateUUID()
		maxAttempts := 1
		backoffSeconds := 0
		multiplier := 1.0
		maxBackoffSeconds := 0

		if s.Retry != nil {
			if s.Retry.MaxAttempts > 0 {
				maxAttempts = s.Retry.MaxAttempts
			}
			if s.Retry.Backoff > 0 {
				backoffSeconds = s.Retry.Backoff
			}
			if s.Retry.Multiplier > 0 {
				multiplier = s.Retry.Multiplier
			}
			if s.Retry.MaxBackoff > 0 {
				maxBackoffSeconds = s.Retry.MaxBackoff
			}
		}

		dependsOnJSON, err := json.Marshal(s.DependsOn)
		if err != nil || len(s.DependsOn) == 0 {
			dependsOnJSON = []byte("[]")
		}

		configJSON := s.Config
		if len(configJSON) == 0 {
			configJSON = []byte("{}")
		}

		_, err = tx.ExecContext(
			ctx,
			insertStepExecQuery,
			stepExecID,
			wfExecID,
			s.ID,
			s.Name,
			string(s.Type),
			string(configJSON),
			string(dependsOnJSON),
			maxAttempts,
			s.Timeout,
			backoffSeconds,
			multiplier,
			maxBackoffSeconds,
			false, // is_compensation
			now,
			now,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert step execution for %s: %w", s.ID, err)
		}

		stepExecs = append(stepExecs, step.StepExecution{
			ID:                  stepExecID,
			WorkflowExecutionID: wfExecID,
			StepID:              s.ID,
			Name:                s.Name,
			Type:                string(s.Type),
			Config:              configJSON,
			DependsOn:           s.DependsOn,
			Status:              step.StatusPending,
			MaxAttempts:         maxAttempts,
			TimeoutSeconds:      s.Timeout,
			BackoffSeconds:      backoffSeconds,
			Multiplier:          multiplier,
			MaxBackoffSeconds:   maxBackoffSeconds,
			IsCompensation:      false,
		})

		// If step has a compensation definition, seed it with is_compensation = TRUE
		if s.Compensation != nil {
			compExecID := util.GenerateUUID()
			compMaxAttempts := 1
			compBackoff := 0
			compMultiplier := 1.0
			compMaxBackoff := 0

			if s.Compensation.Retry != nil {
				if s.Compensation.Retry.MaxAttempts > 0 {
					compMaxAttempts = s.Compensation.Retry.MaxAttempts
				}
				if s.Compensation.Retry.Backoff > 0 {
					compBackoff = s.Compensation.Retry.Backoff
				}
				if s.Compensation.Retry.Multiplier > 0 {
					compMultiplier = s.Compensation.Retry.Multiplier
				}
				if s.Compensation.Retry.MaxBackoff > 0 {
					compMaxBackoff = s.Compensation.Retry.MaxBackoff
				}
			}

			compConfigJSON := s.Compensation.Config
			if len(compConfigJSON) == 0 {
				compConfigJSON = []byte("{}")
			}

			compName := "Compensation for " + s.Name
			if s.Name == "" {
				compName = "Compensation for " + s.ID
			}

			_, err = tx.ExecContext(
				ctx,
				insertStepExecQuery,
				compExecID,
				wfExecID,
				s.ID, // associate with original step_id
				compName,
				string(s.Compensation.Type),
				string(compConfigJSON),
				"[]",
				compMaxAttempts,
				s.Compensation.Timeout,
				compBackoff,
				compMultiplier,
				compMaxBackoff,
				true, // is_compensation = true
				now,
				now,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to insert compensation execution for %s: %w", s.ID, err)
			}

			stepExecs = append(stepExecs, step.StepExecution{
				ID:                  compExecID,
				WorkflowExecutionID: wfExecID,
				StepID:              s.ID,
				Name:                compName,
				Type:                string(s.Compensation.Type),
				Config:              compConfigJSON,
				Status:              step.StatusPending,
				MaxAttempts:         compMaxAttempts,
				TimeoutSeconds:      s.Compensation.Timeout,
				BackoffSeconds:      compBackoff,
				Multiplier:          compMultiplier,
				MaxBackoffSeconds:   compMaxBackoff,
				IsCompensation:      true,
			})
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit workflow execution: %w", err)
	}

	return &step.WorkflowExecution{
		ID:         wfExecID,
		WorkflowID: workflowID,
		Status:     step.StatusRunning,
		Context:    json.RawMessage(effectiveContext),
		CreatedAt:  now,
		UpdatedAt:  now,
		Steps:      stepExecs,
	}, nil
}

