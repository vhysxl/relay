package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/pkg/errdefs"
	"github.com/vhysxl/relay/pkg/util"
)

type executionRepository struct {
	db                  *sql.DB
	stepRepo            step.StepRepository
	defaultLockDuration time.Duration
}

func NewExecutionRepository(db *sql.DB, stepRepo step.StepRepository, cfg ...*config.Config) ExecutionRepository {
	leaseDuration := config.DefaultLeaseDuration
	if len(cfg) > 0 && cfg[0] != nil && cfg[0].DefaultLeaseDuration > 0 {
		leaseDuration = cfg[0].DefaultLeaseDuration
	}
	return &executionRepository{
		db:                  db,
		stepRepo:            stepRepo,
		defaultLockDuration: leaseDuration,
	}
}

func (r *executionRepository) ExecuteWorkflowTx(ctx context.Context, name string, initialContext json.RawMessage) (*WorkflowExecution, error) {
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
	err := r.db.QueryRowContext(ctx, checkQuery, name, name).Scan(&realWfID, &defaultContext)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("failed to query workflow definition: %w", err)
	}

	// 2. Merge initial context on top of default workflow context
	var effectiveContext []byte
	if defaultContext.Valid && len(defaultContext.String) > 0 {
		effectiveContext = []byte(defaultContext.String)
	}
	if len(initialContext) > 0 {
		trimmed := strings.TrimSpace(string(initialContext))
		if trimmed != "" && trimmed != "null" {
			if len(effectiveContext) == 0 {
				effectiveContext = initialContext
			} else {
				var baseMap, overrideMap map[string]any
				if err := json.Unmarshal(effectiveContext, &baseMap); err == nil {
					if err := json.Unmarshal(initialContext, &overrideMap); err == nil {
						for k, v := range overrideMap {
							baseMap[k] = v
						}
						if merged, err := json.Marshal(baseMap); err == nil {
							effectiveContext = merged
						}
					}
				}
			}
		}
	}
	if len(effectiveContext) == 0 {
		effectiveContext = []byte("{}")
	}

	// 3. Begin Transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	wfExecID := util.GenerateUUID()
	now := time.Now().UTC()

	// 4. Insert workflow execution
	insertWfExecQuery := `
		INSERT INTO workflow_executions (id, workflow_id, status, context, created_at, updated_at)
		VALUES (?, ?, 'RUNNING', ?, ?, ?)
	`
	_, err = tx.ExecContext(ctx, insertWfExecQuery, wfExecID, realWfID, string(effectiveContext), now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to insert workflow execution: %w", err)
	}

	// 5. Fetch step definitions
	steps, err := r.stepRepo.GetStepsByWorkflowID(ctx, realWfID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch step definitions: %w", err)
	}

	// 6. Insert step executions
	stepExecs := make([]StepExecution, 0, len(steps))

	insertStepExecQuery := `
		INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on,
			status, attempt, max_attempts, timeout_seconds, backoff_seconds,
			multiplier, max_backoff_seconds, is_compensation, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	// Build transpose / reverse dependency map for compensation DAG (Mirror Rollback)
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

		initialStatus := StatusPending
		if len(s.DependsOn) > 0 {
			initialStatus = StatusWaiting
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
			string(initialStatus),
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

		stepExecs = append(stepExecs, StepExecution{
			ID:                  stepExecID,
			WorkflowExecutionID: wfExecID,
			StepID:              s.ID,
			Name:                s.Name,
			Type:                string(s.Type),
			Config:              configJSON,
			DependsOn:           s.DependsOn,
			Status:              initialStatus,
			MaxAttempts:         maxAttempts,
			TimeoutSeconds:      s.Timeout,
			BackoffSeconds:      backoffSeconds,
			Multiplier:          multiplier,
			MaxBackoffSeconds:   maxBackoffSeconds,
			IsCompensation:      false,
			CreatedAt:           now,
			UpdatedAt:           now,
		})

		// 7. Insert compensation step execution if defined
		if s.Compensation != nil {
			compExecID := util.GenerateUUID()
			compMaxAttempts := 1
			compBackoffSeconds := 0
			compMultiplier := 1.0
			compMaxBackoffSeconds := 0

			if s.Compensation.Retry != nil {
				if s.Compensation.Retry.MaxAttempts > 0 {
					compMaxAttempts = s.Compensation.Retry.MaxAttempts
				}
				if s.Compensation.Retry.Backoff > 0 {
					compBackoffSeconds = s.Compensation.Retry.Backoff
				}
				if s.Compensation.Retry.Multiplier > 0 {
					compMultiplier = s.Compensation.Retry.Multiplier
				}
				if s.Compensation.Retry.MaxBackoff > 0 {
					compMaxBackoffSeconds = s.Compensation.Retry.MaxBackoff
				}
			}

			// Reverse dependency for rollback mirror
			compDependsOn := reverseDeps[s.ID]
			if compDependsOn == nil {
				compDependsOn = []string{}
			}
			compDependsOnJSON, err := json.Marshal(compDependsOn)
			if err != nil {
				compDependsOnJSON = []byte("[]")
			}

			compConfigJSON := s.Compensation.Config
			if len(compConfigJSON) == 0 {
				compConfigJSON = []byte("{}")
			} else if len(effectiveContext) > 0 {
				compConfigJSON = util.InterpolateContext(compConfigJSON, effectiveContext)
			}

			_, err = tx.ExecContext(
				ctx,
				insertStepExecQuery,
				compExecID,
				wfExecID,
				s.ID,
				s.Name+" (Compensation)",
				string(s.Compensation.Type),
				string(compConfigJSON),
				string(compDependsOnJSON),
				string(StatusWaiting), // all compensations start in WAITING
				compMaxAttempts,
				s.Compensation.Timeout,
				compBackoffSeconds,
				compMultiplier,
				compMaxBackoffSeconds,
				true, // is_compensation
				now,
				now,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to insert compensation step execution for %s: %w", s.ID, err)
			}

			stepExecs = append(stepExecs, StepExecution{
				ID:                  compExecID,
				WorkflowExecutionID: wfExecID,
				StepID:              s.ID,
				Name:                s.Name + " (Compensation)",
				Type:                string(s.Compensation.Type),
				Config:              compConfigJSON,
				DependsOn:           compDependsOn,
				Status:              StatusWaiting,
				MaxAttempts:         compMaxAttempts,
				TimeoutSeconds:      s.Compensation.Timeout,
				BackoffSeconds:      compBackoffSeconds,
				Multiplier:          compMultiplier,
				MaxBackoffSeconds:   compMaxBackoffSeconds,
				IsCompensation:      true,
				CreatedAt:           now,
				UpdatedAt:           now,
			})
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit execution transaction: %w", err)
	}

	return &WorkflowExecution{
		ID:         wfExecID,
		WorkflowID: realWfID,
		Status:     StatusRunning,
		Context:    effectiveContext,
		CreatedAt:  now,
		UpdatedAt:  now,
		Steps:      stepExecs,
	}, nil
}

func (r *executionRepository) ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error) {
	if r.db == nil {
		return []WorkflowExecution{}, nil
	}

	query := `
		SELECT e.id, e.workflow_id, w.name, e.status, e.context, e.error_message, e.created_at, e.updated_at
		FROM workflow_executions e
		LEFT JOIN workflow_definitions w ON e.workflow_id = w.id
		ORDER BY e.created_at DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var execs []WorkflowExecution
	var execIDs []string
	for rows.Next() {
		var (
			exec      WorkflowExecution
			wfName    sql.NullString
			ctxStr    sql.NullString
			statusStr string
		)
		err := rows.Scan(
			&exec.ID,
			&exec.WorkflowID,
			&wfName,
			&statusStr,
			&ctxStr,
			&exec.ErrorMessage,
			&exec.CreatedAt,
			&exec.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		exec.Status = ExecutionStatus(statusStr)
		if wfName.Valid {
			exec.WorkflowName = wfName.String
		}
		if ctxStr.Valid {
			exec.Context = json.RawMessage(ctxStr.String)
		}
		execs = append(execs, exec)
		execIDs = append(execIDs, exec.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(execs) == 0 {
		return []WorkflowExecution{}, nil
	}

	stepsMap, err := r.getStepExecutionsByWorkflowExecIDs(ctx, execIDs)
	if err != nil {
		return nil, err
	}

	for i := range execs {
		if steps, ok := stepsMap[execs[i].ID]; ok {
			execs[i].Steps = steps
		} else {
			execs[i].Steps = []StepExecution{}
		}
	}

	return execs, nil
}

func (r *executionRepository) GetExecution(ctx context.Context, executionID string) (*WorkflowExecution, error) {
	if r.db == nil {
		return nil, errdefs.ErrExecutionNotFound
	}

	query := `
		SELECT e.id, e.workflow_id, w.name, e.status, e.context, e.error_message, e.created_at, e.updated_at
		FROM workflow_executions e
		LEFT JOIN workflow_definitions w ON e.workflow_id = w.id
		WHERE e.id = ?
	`
	var (
		exec      WorkflowExecution
		wfName    sql.NullString
		ctxStr    sql.NullString
		statusStr string
	)
	err := r.db.QueryRowContext(ctx, query, executionID).Scan(
		&exec.ID,
		&exec.WorkflowID,
		&wfName,
		&statusStr,
		&ctxStr,
		&exec.ErrorMessage,
		&exec.CreatedAt,
		&exec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrExecutionNotFound
		}
		return nil, err
	}
	exec.Status = ExecutionStatus(statusStr)
	if wfName.Valid {
		exec.WorkflowName = wfName.String
	}
	if ctxStr.Valid {
		exec.Context = json.RawMessage(ctxStr.String)
	}

	steps, err := r.getStepExecutions(ctx, exec.ID)
	if err != nil {
		return nil, err
	}
	exec.Steps = steps

	return &exec, nil
}

// Heartbeat extends the lock lease (locked_until) for a step execution currently running and owned by workerID
func (r *executionRepository) Heartbeat(ctx context.Context, stepExecutionID string, workerID string, lockDuration time.Duration) error {
	if lockDuration <= 0 {
		lockDuration = r.defaultLockDuration
	}
	if lockDuration <= 0 {
		lockDuration = config.DefaultLeaseDuration
	}

	query := `
		UPDATE step_executions
		SET locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND),
		    updated_at = NOW()
		WHERE id = ?
		  AND locked_by = ?
		  AND status = 'RUNNING'
	`
	lockSeconds := int(lockDuration.Seconds())
	res, err := r.db.ExecContext(ctx, query, lockSeconds, stepExecutionID, workerID)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows for heartbeat: %w", err)
	}

	if rows == 0 {
		// In MySQL, if an UPDATE is executed within the same second, columns may not change value,
		// causing RowsAffected() to return 0 unless clientFoundRows=true is set in the DSN.
		// Verify if the row exists and is currently owned by this worker in RUNNING state.
		var exists bool
		checkQuery := `
			SELECT EXISTS(
				SELECT 1 FROM step_executions
				WHERE id = ? AND locked_by = ? AND status = 'RUNNING'
			)
		`
		if err := r.db.QueryRowContext(ctx, checkQuery, stepExecutionID, workerID).Scan(&exists); err != nil {
			return fmt.Errorf("failed to verify lock ownership: %w", err)
		}
		if !exists {
			return errdefs.ErrLockLost
		}
		return nil
	}

	return nil
}

// CompleteStep marks a running step execution as COMPLETED (or COMPENSATED), records the output,
// releases the worker lock lease, and checks if the entire workflow execution has finished.
func (r *executionRepository) CompleteStep(ctx context.Context, stepExecutionID string, workerID string, output json.RawMessage) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch and lock step execution to verify ownership and get workflow_execution_id & is_compensation
	var (
		workflowExecID string
		isCompensation bool
		stepID         string
	)
	query := `
		SELECT workflow_execution_id, is_compensation, step_id
		FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING'
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, query, stepExecutionID, workerID).Scan(&workflowExecID, &isCompensation, &stepID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errdefs.ErrLockLost
		}
		return fmt.Errorf("failed to lock step execution for completion: %w", err)
	}

	// 2. Format output
	var outBytes []byte
	if len(output) > 0 && string(output) != "null" {
		outBytes = output
	} else {
		outBytes = []byte("{}")
	}

	// 3. Update step execution status
	targetStatus := StatusCompleted
	if isCompensation {
		targetStatus = StatusCompensated
	}

	updateQuery := `
		UPDATE step_executions
		SET status = ?,
		    output = ?,
		    locked_by = NULL,
		    locked_until = NULL,
		    updated_at = NOW()
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, updateQuery, targetStatus, outBytes, stepExecutionID); err != nil {
		return fmt.Errorf("failed to update step execution to %s: %w", targetStatus, err)
	}

	// 3b. Accumulate step output into workflow_executions context for downstream payload passing
	var wfContextRaw sql.NullString
	getWfCtxQuery := `SELECT context FROM workflow_executions WHERE id = ? FOR UPDATE`
	if err := tx.QueryRowContext(ctx, getWfCtxQuery, workflowExecID).Scan(&wfContextRaw); err != nil {
		return fmt.Errorf("failed to read workflow context: %w", err)
	}

	var ctxMap map[string]interface{}
	if wfContextRaw.Valid && len(wfContextRaw.String) > 0 {
		_ = json.Unmarshal([]byte(wfContextRaw.String), &ctxMap)
	}
	if ctxMap == nil {
		ctxMap = make(map[string]interface{})
	}

	var parsedOutput interface{}
	if err := json.Unmarshal(outBytes, &parsedOutput); err == nil {
		stepsMap, ok := ctxMap["steps"].(map[string]interface{})
		if !ok {
			stepsMap = make(map[string]interface{})
			ctxMap["steps"] = stepsMap
		}

		if !isCompensation {
			stepObj, ok := stepsMap[stepID].(map[string]interface{})
			if !ok {
				stepObj = make(map[string]interface{})
				stepsMap[stepID] = stepObj
			}
			stepObj["output"] = parsedOutput

			// Add direct key aliases if parsedOutput is an object
			if outMap, ok := parsedOutput.(map[string]interface{}); ok {
				for k, v := range outMap {
					if k != "output" {
						stepObj[k] = v
					}
				}
			}

			// Alias at root level for shorthand {{<stepID>.key}} or {{<stepID>.output.key}}
			ctxMap[stepID] = stepObj
		} else {
			// For compensation steps
			stepObj, ok := stepsMap[stepID].(map[string]interface{})
			if !ok {
				stepObj = make(map[string]interface{})
				stepsMap[stepID] = stepObj
			}
			compObj, ok := stepObj["compensation"].(map[string]interface{})
			if !ok {
				compObj = make(map[string]interface{})
				stepObj["compensation"] = compObj
			}
			compObj["output"] = parsedOutput
		}

		updatedCtxBytes, err := json.Marshal(ctxMap)
		if err == nil {
			updateWfCtxQuery := `
				UPDATE workflow_executions
				SET context = ?,
				    updated_at = NOW()
				WHERE id = ?
			`
			if _, err := tx.ExecContext(ctx, updateWfCtxQuery, string(updatedCtxBytes), workflowExecID); err != nil {
				return fmt.Errorf("failed to update workflow context: %w", err)
			}
		}
	}

	// 4. Wake up downstream dependent steps whose dependencies are now all satisfied
	if err := r.WakeUpDependentsTx(ctx, tx, workflowExecID, isCompensation); err != nil {
		return fmt.Errorf("failed to wake up dependent steps: %w", err)
	}

	// 5. Check if workflow execution is complete
	if !isCompensation {
		// Check if there are any remaining non-compensation steps not COMPLETED
		var remainingNonComp int
		remQuery := `
			SELECT COUNT(*)
			FROM step_executions
			WHERE workflow_execution_id = ?
			  AND is_compensation = FALSE
			  AND status != 'COMPLETED'
		`
		if err := tx.QueryRowContext(ctx, remQuery, workflowExecID).Scan(&remainingNonComp); err != nil {
			return fmt.Errorf("failed to check remaining steps: %w", err)
		}

		if remainingNonComp == 0 {
			// All regular steps are completed! Update workflow to COMPLETED if it is currently RUNNING
			updateWfQuery := `
				UPDATE workflow_executions
				SET status = 'COMPLETED',
				    updated_at = NOW()
				WHERE id = ? AND status = 'RUNNING'
			`
			res, err := tx.ExecContext(ctx, updateWfQuery, workflowExecID)
			if err != nil {
				return fmt.Errorf("failed to mark workflow completed: %w", err)
			}
			rowsAffected, _ := res.RowsAffected()

			if rowsAffected > 0 {
				// Clean up / cancel unused compensation steps since workflow completed successfully
				cancelUnusedCompQuery := `
					UPDATE step_executions
					SET status = 'CANCELLED',
					    updated_at = NOW()
					WHERE workflow_execution_id = ?
					  AND is_compensation = TRUE
					  AND status IN ('PENDING', 'WAITING')
				`
				if _, err := tx.ExecContext(ctx, cancelUnusedCompQuery, workflowExecID); err != nil {
					return fmt.Errorf("failed to cancel unused compensation steps: %w", err)
				}
			}
		}
	} else {
		// Compensation step completed. Check if all active compensation steps and running normal steps are done
		var remainingWork int
		remCompQuery := `
			SELECT COUNT(*)
			FROM step_executions
			WHERE workflow_execution_id = ?
			  AND (
			      (is_compensation = TRUE AND status NOT IN ('COMPENSATED', 'CANCELLED'))
			      OR (is_compensation = FALSE AND status = 'RUNNING')
			  )
		`
		if err := tx.QueryRowContext(ctx, remCompQuery, workflowExecID).Scan(&remainingWork); err != nil {
			return fmt.Errorf("failed to check remaining compensation steps: %w", err)
		}

		if remainingWork == 0 {
			// All compensation steps are done! Mark workflow as COMPENSATED
			updateWfQuery := `
				UPDATE workflow_executions
				SET status = 'COMPENSATED',
				    updated_at = NOW()
				WHERE id = ? AND status = 'COMPENSATING'
			`
			if _, err := tx.ExecContext(ctx, updateWfQuery, workflowExecID); err != nil {
				return fmt.Errorf("failed to mark workflow compensated: %w", err)
			}
		}
	}

	return tx.Commit()
}

// FailStep handles a step execution failure. If attempts remain under max_attempts,
// it schedules a retry with exponential backoff and returns status to PENDING.
// If retries are exhausted, it marks the step as FAILED (or COMPENSATION_FAILED),
// cancels remaining pending steps, and transitions the workflow to COMPENSATING (if compensations exist) or FAILED.
func (r *executionRepository) FailStep(ctx context.Context, stepExecutionID string, workerID string, errMsg string) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch and lock step execution to verify ownership and get retry & workflow configs
	var (
		workflowExecID    string
		attempt           int
		maxAttempts       int
		backoffSeconds    int
		multiplier        float64
		maxBackoffSeconds int
		isCompensation    bool
	)
	query := `
		SELECT workflow_execution_id, attempt, max_attempts, backoff_seconds, multiplier, max_backoff_seconds, is_compensation
		FROM step_executions
		WHERE id = ? AND locked_by = ? AND status = 'RUNNING'
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, query, stepExecutionID, workerID).Scan(
		&workflowExecID,
		&attempt,
		&maxAttempts,
		&backoffSeconds,
		&multiplier,
		&maxBackoffSeconds,
		&isCompensation,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errdefs.ErrLockLost
		}
		return fmt.Errorf("failed to lock step execution for failure: %w", err)
	}

	// 2. Determine if we should retry
	if attempt < maxAttempts {
		// Calculate backoff
		// attempt was already incremented to 1 on first claim.
		power := float64(attempt - 1)
		if power < 0 {
			power = 0
		}
		if multiplier <= 0 {
			multiplier = 1.0
		}
		delay := float64(backoffSeconds) * math.Pow(multiplier, power)
		if maxBackoffSeconds > 0 && delay > float64(maxBackoffSeconds) {
			delay = float64(maxBackoffSeconds)
		}
		if delay < 0 {
			delay = 0
		}
		delaySeconds := int(math.Round(delay))

		retryQuery := `
			UPDATE step_executions
			SET status = 'PENDING',
			    error_message = ?,
			    next_retry_at = DATE_ADD(NOW(), INTERVAL ? SECOND),
			    locked_by = NULL,
			    locked_until = NULL,
			    updated_at = NOW()
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, retryQuery, errMsg, delaySeconds, stepExecutionID); err != nil {
			return fmt.Errorf("failed to schedule step retry: %w", err)
		}

		return tx.Commit()
	}

	// 3. Retries exhausted -> Permanently failed
	if err := r.failStepPermanentlyTx(ctx, tx, stepExecutionID, workflowExecID, isCompensation, errMsg); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *executionRepository) failStepPermanentlyTx(
	ctx context.Context,
	tx *sql.Tx,
	stepExecutionID string,
	workflowExecID string,
	isCompensation bool,
	errMsg string,
) error {
	if isCompensation {
		failCompQuery := `
			UPDATE step_executions
			SET status = 'COMPENSATION_FAILED',
			    error_message = ?,
			    locked_by = NULL,
			    locked_until = NULL,
			    updated_at = NOW()
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, failCompQuery, errMsg, stepExecutionID); err != nil {
			return fmt.Errorf("failed to mark compensation step as COMPENSATION_FAILED: %w", err)
		}

		failWfQuery := `
			UPDATE workflow_executions
			SET status = 'COMPENSATION_FAILED',
			    error_message = ?,
			    updated_at = NOW()
			WHERE id = ?
		`
		if _, err := tx.ExecContext(ctx, failWfQuery, fmt.Sprintf("compensation step %s failed: %s", stepExecutionID, errMsg), workflowExecID); err != nil {
			return fmt.Errorf("failed to mark workflow as COMPENSATION_FAILED: %w", err)
		}

		return nil
	}

	// Regular step permanently failed
	failStepQuery := `
		UPDATE step_executions
		SET status = 'FAILED',
		    error_message = ?,
		    locked_by = NULL,
		    locked_until = NULL,
		    updated_at = NOW()
		WHERE id = ?
	`
	if _, err := tx.ExecContext(ctx, failStepQuery, errMsg, stepExecutionID); err != nil {
		return fmt.Errorf("failed to mark step execution as FAILED: %w", err)
	}

	// Cancel any remaining PENDING or WAITING normal steps for this workflow execution
	cancelPendingQuery := `
		UPDATE step_executions
		SET status = 'CANCELLED',
		    updated_at = NOW()
		WHERE workflow_execution_id = ?
		  AND is_compensation = FALSE
		  AND status IN ('PENDING', 'WAITING')
	`
	if _, err := tx.ExecContext(ctx, cancelPendingQuery, workflowExecID); err != nil {
		return fmt.Errorf("failed to cancel pending steps: %w", err)
	}

	// Cancel any PENDING or WAITING compensation steps whose original step has failed or was cancelled
	cancelUnneededCompQuery := `
		UPDATE step_executions comp
		JOIN step_executions normal 
		    ON normal.workflow_execution_id = comp.workflow_execution_id
		   AND normal.step_id = comp.step_id
		   AND normal.is_compensation = FALSE
		SET comp.status = 'CANCELLED',
		    comp.updated_at = NOW()
		WHERE comp.workflow_execution_id = ?
		  AND comp.is_compensation = TRUE
		  AND comp.status IN ('PENDING', 'WAITING')
		  AND normal.status IN ('FAILED', 'CANCELLED')
	`
	if _, err := tx.ExecContext(ctx, cancelUnneededCompQuery, workflowExecID); err != nil {
		return fmt.Errorf("failed to cancel unneeded compensation steps: %w", err)
	}

	// Check if this workflow has any active compensation steps remaining to execute
	var hasCompensation bool
	checkCompQuery := `
		SELECT EXISTS(
			SELECT 1 FROM step_executions
			WHERE workflow_execution_id = ? 
			  AND is_compensation = TRUE
			  AND status IN ('PENDING', 'WAITING')
		)
	`
	if err := tx.QueryRowContext(ctx, checkCompQuery, workflowExecID).Scan(&hasCompensation); err != nil {
		return fmt.Errorf("failed to check for compensation steps: %w", err)
	}

	if hasCompensation {
		// Wake up initial leaf compensation steps (those whose dependencies are already met)
		if err := r.WakeUpDependentsTx(ctx, tx, workflowExecID, true); err != nil {
			return fmt.Errorf("failed to wake up initial compensation steps: %w", err)
		}

		updateWfQuery := `
			UPDATE workflow_executions
			SET status = 'COMPENSATING',
			    error_message = ?,
			    updated_at = NOW()
			WHERE id = ? AND status = 'RUNNING'
		`
		if _, err := tx.ExecContext(ctx, updateWfQuery, fmt.Sprintf("step %s failed: %s", stepExecutionID, errMsg), workflowExecID); err != nil {
			return fmt.Errorf("failed to transition workflow to COMPENSATING: %w", err)
		}
	} else {
		updateWfQuery := `
			UPDATE workflow_executions
			SET status = 'FAILED',
			    error_message = ?,
			    updated_at = NOW()
			WHERE id = ? AND status = 'RUNNING'
		`
		if _, err := tx.ExecContext(ctx, updateWfQuery, fmt.Sprintf("step %s failed: %s", stepExecutionID, errMsg), workflowExecID); err != nil {
			return fmt.Errorf("failed to mark workflow as FAILED: %w", err)
		}
	}

	return nil
}

// WakeUpDependentsTx checks all WAITING steps in workflowExecID (matching isCompensation),
// and transitions any step whose dependencies are now all satisfied to PENDING.
func (r *executionRepository) WakeUpDependentsTx(ctx context.Context, tx *sql.Tx, workflowExecID string, isCompensation bool) error {
	query := `
		SELECT id, step_id, depends_on, status
		FROM step_executions
		WHERE workflow_execution_id = ?
		  AND is_compensation = ?
	`
	rows, err := tx.QueryContext(ctx, query, workflowExecID, isCompensation)
	if err != nil {
		return fmt.Errorf("failed to fetch step executions for wake up: %w", err)
	}
	defer rows.Close()

	type stepSummary struct {
		id        string
		stepID    string
		dependsOn []string
		status    string
	}

	var allSteps []stepSummary
	completedMap := make(map[string]bool)
	activeBlockers := make(map[string]bool)

	for rows.Next() {
		var (
			s            stepSummary
			dependsOnStr sql.NullString
		)
		if err := rows.Scan(&s.id, &s.stepID, &dependsOnStr, &s.status); err != nil {
			return fmt.Errorf("failed to scan step execution for wake up: %w", err)
		}
		if dependsOnStr.Valid && len(dependsOnStr.String) > 0 {
			_ = json.Unmarshal([]byte(dependsOnStr.String), &s.dependsOn)
		}
		allSteps = append(allSteps, s)

		if !isCompensation {
			if s.status == string(StatusCompleted) {
				completedMap[s.stepID] = true
			}
		} else {
			if s.status == string(StatusWaiting) || s.status == string(StatusPending) || s.status == string(StatusRunning) {
				activeBlockers[s.stepID] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error in wake up: %w", err)
	}

	var toWakeUp []string
	for _, s := range allSteps {
		if s.status != string(StatusWaiting) {
			continue
		}

		allMet := true
		if !isCompensation {
			for _, depID := range s.dependsOn {
				if !completedMap[depID] {
					allMet = false
					break
				}
			}
		} else {
			for _, depID := range s.dependsOn {
				if depID != s.stepID && activeBlockers[depID] {
					allMet = false
					break
				}
			}
		}

		if allMet {
			toWakeUp = append(toWakeUp, s.id)
		}
	}

	if len(toWakeUp) == 0 {
		return nil
	}

	placeholders := make([]string, len(toWakeUp))
	args := make([]any, len(toWakeUp))
	for i, id := range toWakeUp {
		placeholders[i] = "?"
		args[i] = id
	}

	updateQuery := fmt.Sprintf(`
		UPDATE step_executions
		SET status = 'PENDING',
		    updated_at = NOW()
		WHERE id IN (%s) AND status = 'WAITING'
	`, strings.Join(placeholders, ","))

	if _, err := tx.ExecContext(ctx, updateQuery, args...); err != nil {
		return fmt.Errorf("failed to wake up dependent steps: %w", err)
	}

	return nil
}

// ReapExpiredSteps resets locks for running steps whose leases have expired and attempts remain,
// and fails steps (cascading cancellations / triggering saga compensation) whose attempts have exhausted max_attempts.
func (r *executionRepository) ReapExpiredSteps(ctx context.Context) (int64, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction for reaping: %w", err)
	}
	defer tx.Rollback()

	// 1. Find expired steps whose attempts have exhausted max_attempts
	type exhaustedStep struct {
		id             string
		workflowExecID string
		isCompensation bool
	}

	selectExhaustedQuery := `
		SELECT id, workflow_execution_id, is_compensation
		FROM step_executions
		WHERE status = 'RUNNING'
		  AND locked_until < NOW()
		  AND attempt >= max_attempts
		FOR UPDATE
	`
	rows, err := tx.QueryContext(ctx, selectExhaustedQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to query exhausted expired steps: %w", err)
	}

	var exhaustedSteps []exhaustedStep
	for rows.Next() {
		var s exhaustedStep
		if err := rows.Scan(&s.id, &s.workflowExecID, &s.isCompensation); err != nil {
			rows.Close()
			return 0, fmt.Errorf("failed to scan exhausted expired step: %w", err)
		}
		exhaustedSteps = append(exhaustedSteps, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("error iterating exhausted expired steps: %w", err)
	}
	rows.Close()

	// 2. Permanently fail exhausted steps and cascade cancellations / compensations
	for _, s := range exhaustedSteps {
		if err := r.failStepPermanentlyTx(ctx, tx, s.id, s.workflowExecID, s.isCompensation, "lease expired: max attempts reached"); err != nil {
			return 0, fmt.Errorf("failed to permanently fail exhausted step %s: %w", s.id, err)
		}
	}

	// 3. Reset expired steps that still have attempts remaining back to PENDING
	resetQuery := `
		UPDATE step_executions
		SET status = 'PENDING',
		    locked_by = NULL,
		    locked_until = NULL,
		    updated_at = NOW()
		WHERE status = 'RUNNING'
		  AND locked_until < NOW()
		  AND attempt < max_attempts
	`
	res, err := tx.ExecContext(ctx, resetQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to reset retryable expired steps: %w", err)
	}
	resetCount, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read affected rows from reap reset: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit reap transaction: %w", err)
	}

	return int64(len(exhaustedSteps)) + resetCount, nil
}

// ClaimPending atomically selects, locks, and returns the next eligible PENDING step execution ticket.
func (r *executionRepository) ClaimPending(ctx context.Context, workerID string, lockDuration time.Duration) (*step.Step, *StepExecution, error) {
	if lockDuration <= 0 {
		lockDuration = r.defaultLockDuration
	}
	if lockDuration <= 0 {
		lockDuration = config.DefaultLeaseDuration
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	excludeTestClause := ""
	if strings.HasPrefix(workerID, "server-") {
		excludeTestClause = "\n\t\t  AND we.workflow_id NOT IN (SELECT id FROM workflow_definitions WHERE name LIKE 'test-%')"
	}

	selectQuery := fmt.Sprintf(`
		SELECT 
			se.id, se.workflow_execution_id, se.step_id, se.name, se.type, se.config, se.depends_on,
			se.status, se.output, se.error_message, se.attempt, se.max_attempts,
			se.timeout_seconds, se.backoff_seconds, se.multiplier, se.max_backoff_seconds,
			se.locked_by, se.locked_until, se.is_compensation, se.executed_at, se.next_retry_at,
			se.created_at, se.updated_at,
			sd.id, sd.workflow_id, sd.name, sd.type, sd.config, sd.depends_on,
			sd.timeout, sd.retry, sd.compensation, sd.irreversible, sd.created_at, sd.updated_at,
			we.context, orig.output
		FROM step_executions se
		JOIN workflow_executions we ON se.workflow_execution_id = we.id
		LEFT JOIN step_definitions sd ON sd.workflow_id = we.workflow_id AND sd.id = se.step_id
		LEFT JOIN step_executions orig 
		    ON orig.workflow_execution_id = se.workflow_execution_id 
		   AND orig.step_id = se.step_id 
		   AND orig.is_compensation = FALSE
		WHERE se.status = 'PENDING'
		  AND (se.locked_until IS NULL OR se.locked_until < NOW())
		  AND (se.next_retry_at IS NULL OR se.next_retry_at <= NOW())%s
		  AND (
		      (we.status = 'RUNNING' AND se.is_compensation = FALSE)
		      OR
		      (we.status = 'COMPENSATING' AND se.is_compensation = TRUE)
		  )
		ORDER BY 
		    CASE WHEN we.status = 'COMPENSATING' THEN 1 ELSE 2 END ASC,
		    we.created_at ASC,
		    se.created_at ASC
		LIMIT 1
		FOR UPDATE OF se SKIP LOCKED
	`, excludeTestClause)

	var (
		se             StepExecution
		seName         sql.NullString
		seConfigStr    string
		seDependsOnStr sql.NullString
		seOutputStr    sql.NullString
		seErrorMsg     sql.NullString
		seLockedBy     sql.NullString
		seLockedUntil  sql.NullTime
		seExecutedAt   sql.NullTime
		seNextRetryAt  sql.NullTime

		sdID           sql.NullString
		sdWorkflowID   sql.NullString
		sdName         sql.NullString
		sdType         sql.NullString
		sdConfigStr    sql.NullString
		sdDependsOnStr sql.NullString
		sdTimeout      sql.NullInt64
		sdRetryStr     sql.NullString
		sdCompStr      sql.NullString
		sdIrreversible sql.NullBool
		sdCreatedAt    sql.NullTime
		sdUpdatedAt    sql.NullTime

		weContextStr  sql.NullString
		origOutputStr sql.NullString
	)

	row := tx.QueryRowContext(ctx, selectQuery)
	err = row.Scan(
		&se.ID,
		&se.WorkflowExecutionID,
		&se.StepID,
		&seName,
		&se.Type,
		&seConfigStr,
		&seDependsOnStr,
		&se.Status,
		&seOutputStr,
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
		&sdID,
		&sdWorkflowID,
		&sdName,
		&sdType,
		&sdConfigStr,
		&sdDependsOnStr,
		&sdTimeout,
		&sdRetryStr,
		&sdCompStr,
		&sdIrreversible,
		&sdCreatedAt,
		&sdUpdatedAt,
		&weContextStr,
		&origOutputStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to scan candidate step: %w", err)
	}

	var workflowCtxMap map[string]interface{}
	if weContextStr.Valid && len(weContextStr.String) > 0 {
		_ = json.Unmarshal([]byte(weContextStr.String), &workflowCtxMap)
	}
	if workflowCtxMap == nil {
		workflowCtxMap = make(map[string]interface{})
	}

	if se.IsCompensation && origOutputStr.Valid && len(origOutputStr.String) > 0 {
		var origOutput interface{}
		if err := json.Unmarshal([]byte(origOutputStr.String), &origOutput); err == nil {
			origObj := make(map[string]interface{})
			origObj["output"] = origOutput
			if m, ok := origOutput.(map[string]interface{}); ok {
				for k, v := range m {
					if k != "output" {
						origObj[k] = v
					}
				}
			}
			workflowCtxMap["original"] = origObj
		}
	}

	rawConfigBytes := []byte(seConfigStr)
	if len(rawConfigBytes) == 0 && !se.IsCompensation && sdConfigStr.Valid && len(sdConfigStr.String) > 0 {
		rawConfigBytes = []byte(sdConfigStr.String)
	}
	if len(rawConfigBytes) == 0 {
		rawConfigBytes = []byte("{}")
	}

	var resolvedConfigBytes []byte
	if len(workflowCtxMap) > 0 {
		ctxBytes, err := json.Marshal(workflowCtxMap)
		if err == nil {
			resolvedConfigBytes = util.InterpolateContext(rawConfigBytes, ctxBytes)
		} else {
			resolvedConfigBytes = rawConfigBytes
		}
	} else {
		resolvedConfigBytes = rawConfigBytes
	}

	updateQuery := `
		UPDATE step_executions
		SET locked_by = ?,
		    locked_until = DATE_ADD(NOW(), INTERVAL ? SECOND),
		    status = 'RUNNING',
		    attempt = attempt + 1,
		    config = ?,
		    executed_at = NOW(),
		    updated_at = NOW()
		WHERE id = ?
	`
	lockSeconds := int(lockDuration.Seconds())
	_, err = tx.ExecContext(ctx, updateQuery, workerID, lockSeconds, string(resolvedConfigBytes), se.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to update locked step execution: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to commit claim transaction: %w", err)
	}

	se.LockedBy = &workerID
	lockedUntilTime := time.Now().UTC().Add(lockDuration)
	se.LockedUntil = &lockedUntilTime
	se.Status = StatusRunning
	se.Attempt++
	now := time.Now().UTC()
	se.ExecutedAt = &now
	se.UpdatedAt = now
	se.Config = json.RawMessage(resolvedConfigBytes)

	if seName.Valid {
		se.Name = seName.String
	}
	if seDependsOnStr.Valid && len(seDependsOnStr.String) > 0 {
		var deps []string
		if err := json.Unmarshal([]byte(seDependsOnStr.String), &deps); err == nil {
			se.DependsOn = deps
		}
	}
	if seOutputStr.Valid && len(seOutputStr.String) > 0 {
		se.Output = json.RawMessage(seOutputStr.String)
	}
	if seErrorMsg.Valid {
		se.ErrorMessage = &seErrorMsg.String
	}
	if seNextRetryAt.Valid {
		se.NextRetryAt = &seNextRetryAt.Time
	}

	s := step.Step{
		ID:         se.StepID,
		WorkflowID: se.WorkflowExecutionID,
		Name:       se.Name,
		Type:       step.StepType(se.Type),
		Config:     se.Config,
		DependsOn:  se.DependsOn,
		Timeout:    se.TimeoutSeconds,
	}

	if sdID.Valid {
		s.ID = sdID.String
		if sdWorkflowID.Valid {
			s.WorkflowID = sdWorkflowID.String
		}
		if !se.IsCompensation {
			if sdName.Valid {
				s.Name = sdName.String
			}
			if sdType.Valid {
				s.Type = step.StepType(sdType.String)
			}
		}
		if sdDependsOnStr.Valid && len(sdDependsOnStr.String) > 0 {
			var deps []string
			if err := json.Unmarshal([]byte(sdDependsOnStr.String), &deps); err == nil {
				s.DependsOn = deps
			}
		}
		if sdTimeout.Valid {
			s.Timeout = int(sdTimeout.Int64)
		}
		if sdRetryStr.Valid && len(sdRetryStr.String) > 0 {
			var rp step.RetryPolicy
			if err := json.Unmarshal([]byte(sdRetryStr.String), &rp); err == nil {
				s.Retry = &rp
			}
		}
		if sdCompStr.Valid && len(sdCompStr.String) > 0 {
			var comp step.Compensation
			if err := json.Unmarshal([]byte(sdCompStr.String), &comp); err == nil {
				s.Compensation = &comp
			}
		}
		if sdIrreversible.Valid {
			s.Irreversible = sdIrreversible.Bool
		}
		if sdCreatedAt.Valid {
			s.CreatedAt = sdCreatedAt.Time
		}
		if sdUpdatedAt.Valid {
			s.UpdatedAt = sdUpdatedAt.Time
		}
	}

	return &s, &se, nil
}

func (r *executionRepository) getStepExecutions(ctx context.Context, workflowExecID string) ([]StepExecution, error) {
	stepsMap, err := r.getStepExecutionsByWorkflowExecIDs(ctx, []string{workflowExecID})
	if err != nil {
		return nil, err
	}
	return stepsMap[workflowExecID], nil
}

func (r *executionRepository) getStepExecutionsByWorkflowExecIDs(ctx context.Context, workflowExecIDs []string) (map[string][]StepExecution, error) {
	result := make(map[string][]StepExecution)
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
			se            StepExecution
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
		se.Status = ExecutionStatus(statusStr)
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
