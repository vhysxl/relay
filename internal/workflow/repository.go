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
		INSERT INTO workflow_definitions (id, name, version, context, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
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
		SELECT id, name, version, context, created_at, updated_at
		FROM workflow_definitions
		WHERE id = ? OR name = ?
		ORDER BY version DESC LIMIT 1
	`

	var (
		wf          Workflow
		contextData sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, workflowID, workflowID).Scan(
		&wf.ID,
		&wf.Name,
		&wf.Version,
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
	// 1. Verify workflow definition exists (by ID or Name) and get default context
	var (
		realWfID       string
		defaultContext sql.NullString
	)
	checkQuery := `
		SELECT id, context
		FROM workflow_definitions
		WHERE id = ? OR name = ?
		ORDER BY version DESC LIMIT 1
	`
	err := r.db.QueryRowContext(ctx, checkQuery, workflowID, workflowID).Scan(&realWfID, &defaultContext)
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
	_, err = tx.ExecContext(ctx, insertWfExecQuery, wfExecID, realWfID, string(effectiveContext), now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert workflow execution: %w", err)
	}

	// 5. Fetch step definitions
	if r.stepRepo == nil {
		return nil, errors.New("step repository is not configured")
	}
	steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, realWfID)
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

	// Build transpose / reverse dependency map for compensation DAG (Mirror Rollback)
	// If step B depends on step A, then in compensation DAG, compensation A depends on compensation B.
	reverseDeps := make(map[string][]string, len(steps))
	for _, s := range steps {
		for _, depID := range s.DependsOn {
			reverseDeps[depID] = append(reverseDeps[depID], s.ID)
		}
	}

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
		} else if len(effectiveContext) > 0 {
			configJSON = util.InterpolateContext(configJSON, effectiveContext)
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
			} else if len(effectiveContext) > 0 {
				compConfigJSON = util.InterpolateContext(compConfigJSON, effectiveContext)
			}

			compName := "Compensation for " + s.Name
			if s.Name == "" {
				compName = "Compensation for " + s.ID
			}

			compDependsOn := reverseDeps[s.ID]
			if compDependsOn == nil {
				compDependsOn = []string{}
			}
			compDependsOnJSON, err := json.Marshal(compDependsOn)
			if err != nil {
				compDependsOnJSON = []byte("[]")
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
				string(compDependsOnJSON),
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
				DependsOn:           compDependsOn,
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

func (r *workflowRepository) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	if r.db == nil {
		return []Workflow{}, nil
	}
	query := `
		SELECT id, name, version, context, created_at, updated_at
		FROM workflow_definitions
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wfs []Workflow
	for rows.Next() {
		var (
			wf      Workflow
			ctxData sql.NullString
		)
		if err := rows.Scan(
			&wf.ID,
			&wf.Name,
			&wf.Version,
			&ctxData,
			&wf.CreatedAt,
			&wf.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if ctxData.Valid && len(ctxData.String) > 0 {
			wf.Context = json.RawMessage(ctxData.String)
		}
		if r.stepRepo != nil {
			steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, wf.ID)
			if err == nil {
				wf.Steps = steps
			}
		}
		wfs = append(wfs, wf)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if r.stepRepo != nil && len(wfs) > 0 {
		wfIDs := make([]string, len(wfs))
		for i, wf := range wfs {
			wfIDs[i] = wf.ID
		}
		stepsMap, err := r.stepRepo.GetStepsByWorkflowIDs(ctx, wfIDs)
		if err == nil {
			for i := range wfs {
				if steps, ok := stepsMap[wfs[i].ID]; ok {
					wfs[i].Steps = steps
				} else {
					wfs[i].Steps = []step.Step{}
				}
			}
		}
	}

	return wfs, nil
}

func (r *workflowRepository) ListExecutions(ctx context.Context) ([]step.WorkflowExecution, error) {
	if r.db == nil {
		return []step.WorkflowExecution{}, nil
	}
	query := `
		SELECT we.id, we.workflow_id, COALESCE(wd.name, ''), we.status, we.context, we.error_message, we.created_at, we.updated_at
		FROM workflow_executions we
		LEFT JOIN workflow_definitions wd ON we.workflow_id = wd.id
		ORDER BY we.created_at DESC
		LIMIT 100
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []step.WorkflowExecution
	for rows.Next() {
		var (
			exec      step.WorkflowExecution
			statusStr string
			ctxData   sql.NullString
		)
		if err := rows.Scan(
			&exec.ID,
			&exec.WorkflowID,
			&exec.WorkflowName,
			&statusStr,
			&ctxData,
			&exec.ErrorMessage,
			&exec.CreatedAt,
			&exec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		exec.Status = step.ExecutionStatus(statusStr)
		if ctxData.Valid && len(ctxData.String) > 0 {
			exec.Context = json.RawMessage(ctxData.String)
		}
		execs = append(execs, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(execs) > 0 {
		execIDs := make([]string, len(execs))
		for i, exec := range execs {
			execIDs[i] = exec.ID
		}
		stepsMap, err := r.getStepExecutionsByWorkflowExecIDs(ctx, execIDs)
		if err == nil {
			for i := range execs {
				if steps, ok := stepsMap[execs[i].ID]; ok {
					execs[i].Steps = steps
				} else {
					execs[i].Steps = []step.StepExecution{}
				}
			}
		}
	}

	return execs, nil
}

func (r *workflowRepository) GetExecution(ctx context.Context, executionID string) (*step.WorkflowExecution, error) {
	if r.db == nil {
		return nil, errors.New("execution not found")
	}
	query := `
		SELECT we.id, we.workflow_id, COALESCE(wd.name, ''), we.status, we.context, we.error_message, we.created_at, we.updated_at
		FROM workflow_executions we
		LEFT JOIN workflow_definitions wd ON we.workflow_id = wd.id
		WHERE we.id = ? OR wd.name = ? OR we.workflow_id = ?
		ORDER BY we.created_at DESC LIMIT 1
	`
	var (
		exec      step.WorkflowExecution
		statusStr string
		ctxData   sql.NullString
	)
	err := r.db.QueryRowContext(ctx, query, executionID, executionID, executionID).Scan(
		&exec.ID,
		&exec.WorkflowID,
		&exec.WorkflowName,
		&statusStr,
		&ctxData,
		&exec.ErrorMessage,
		&exec.CreatedAt,
		&exec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("execution not found")
		}
		return nil, err
	}
	exec.Status = step.ExecutionStatus(statusStr)
	if ctxData.Valid && len(ctxData.String) > 0 {
		exec.Context = json.RawMessage(ctxData.String)
	}

	steps, err := r.getStepExecutions(ctx, exec.ID)
	if err == nil {
		exec.Steps = steps
	}
	return &exec, nil
}

func (r *workflowRepository) getStepExecutions(ctx context.Context, workflowExecID string) ([]step.StepExecution, error) {
	stepsMap, err := r.getStepExecutionsByWorkflowExecIDs(ctx, []string{workflowExecID})
	if err != nil {
		return nil, err
	}
	return stepsMap[workflowExecID], nil
}

func (r *workflowRepository) getStepExecutionsByWorkflowExecIDs(ctx context.Context, workflowExecIDs []string) (map[string][]step.StepExecution, error) {
	result := make(map[string][]step.StepExecution)
	if r.db == nil || len(workflowExecIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(workflowExecIDs))
	args := make([]any, len(workflowExecIDs))
	for i, id := range workflowExecIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT 
			id, workflow_execution_id, step_id, name, type, config, depends_on,
			status, output, error_message, attempt, max_attempts, timeout_seconds,
			backoff_seconds, multiplier, max_backoff_seconds, locked_by, locked_until,
			is_compensation, executed_at, next_retry_at, created_at, updated_at
		FROM step_executions
		WHERE workflow_execution_id IN (%s)
		ORDER BY is_compensation ASC, created_at ASC
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			se            step.StepExecution
			statusStr     string
			seName        sql.NullString
			seConfig      string
			seDependsOn   sql.NullString
			seOutput      sql.NullString
			seErrorMsg    sql.NullString
			seLockedBy    sql.NullString
			seLockedUntil sql.NullTime
			seExecutedAt  sql.NullTime
			seNextRetryAt sql.NullTime
		)
		err := rows.Scan(
			&se.ID,
			&se.WorkflowExecutionID,
			&se.StepID,
			&seName,
			&se.Type,
			&seConfig,
			&seDependsOn,
			&statusStr,
			&seOutput,
			&seErrorMsg,
			&se.Attempt,
			&se.MaxAttempts,
			&se.TimeoutSeconds,
			&se.BackoffSeconds,
			&se.Multiplier,
			&se.MaxBackoffSeconds,
			&seLockedBy,
			&seLockedUntil,
			&se.IsCompensation,
			&seExecutedAt,
			&seNextRetryAt,
			&se.CreatedAt,
			&se.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		se.Status = step.ExecutionStatus(statusStr)
		if seName.Valid {
			se.Name = seName.String
		}
		if len(seConfig) > 0 {
			se.Config = json.RawMessage(seConfig)
		}
		if seDependsOn.Valid && len(seDependsOn.String) > 0 {
			var deps []string
			if err := json.Unmarshal([]byte(seDependsOn.String), &deps); err == nil {
				se.DependsOn = deps
			}
		}
		if seOutput.Valid && len(seOutput.String) > 0 {
			se.Output = json.RawMessage(seOutput.String)
		}
		if seErrorMsg.Valid {
			se.ErrorMessage = &seErrorMsg.String
		}
		if seLockedBy.Valid {
			se.LockedBy = &seLockedBy.String
		}
		if seLockedUntil.Valid {
			se.LockedUntil = &seLockedUntil.Time
		}
		if seExecutedAt.Valid {
			se.ExecutedAt = &seExecutedAt.Time
		}
		if seNextRetryAt.Valid {
			se.NextRetryAt = &seNextRetryAt.Time
		}
		result[se.WorkflowExecutionID] = append(result[se.WorkflowExecutionID], se)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
