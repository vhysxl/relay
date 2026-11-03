package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/vhysxl/relay/pkg/errdefs"
)

func insertWorkflowExecutionTx(ctx context.Context, tx *sql.Tx, exec *WorkflowExecution) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO workflow_executions
		(id, workflow_id, status, context, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		exec.ID, exec.WorkflowID, exec.Status, string(exec.Context), exec.CreatedAt, exec.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert execution: %w", err)
	}
	return nil
}

func insertStepExecutionsTx(ctx context.Context, tx *sql.Tx, executionID string, steps []StepExecution) error {
	for _, s := range steps {
		deps, err := json.Marshal(s.DependsOn)
		if err != nil {
			return err
		}
		var condVal any
		if len(s.When) > 0 {
			condVal = string(s.When)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO step_executions (
			id, workflow_execution_id, step_id, name, type, config, depends_on, status,
			attempt, max_attempts, timeout_seconds, backoff_seconds, multiplier, max_backoff_seconds,
			is_compensation, `+"`condition`"+`, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, executionID, s.StepID, s.Name, s.Type, string(s.Config), string(deps), s.Status,
			s.Attempt, s.MaxAttempts, s.TimeoutSeconds, s.BackoffSeconds, s.Multiplier, s.MaxBackoffSeconds,
			s.IsCompensation, condVal, s.CreatedAt, s.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert step %s: %w", s.StepID, err)
		}
	}
	return nil
}

func insertIdempotencyRequestTx(ctx context.Context, tx *sql.Tx, req *ExecutionRequestRecord) error {
	query := `INSERT INTO workflow_execution_requests
		(workflow_name, idempotency_key, workflow_execution_id, input_context, response_receipt, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`
	_, err := tx.ExecContext(ctx, query,
		req.WorkflowName,
		[]byte(req.IdempotencyKey),
		req.WorkflowExecutionID,
		[]byte(req.InputContext),
		[]byte(req.ResponseReceipt),
		req.CreatedAt,
	)
	return err
}

func isMySQLDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}

func (r *executionRepository) CreateExecutionWithIdempotency(
	ctx context.Context,
	exec *WorkflowExecution,
	reqRecord *ExecutionRequestRecord,
	targetIsExplicitID bool,
	targetID string,
) (*ExecuteWorkflowResponse, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, fmt.Errorf("begin execution: %w", err)
	}
	defer tx.Rollback()

	if err := insertWorkflowExecutionTx(ctx, tx, exec); err != nil {
		return nil, false, err
	}

	if err := insertIdempotencyRequestTx(ctx, tx, reqRecord); err != nil {
		if isMySQLDuplicateKey(err) {
			_ = tx.Rollback()
			winner, readErr := r.GetExecutionRequest(ctx, reqRecord.WorkflowName, reqRecord.IdempotencyKey)
			if readErr != nil {
				return nil, false, fmt.Errorf("read winning idempotency record: %w", readErr)
			}
			receipt, evalErr := evaluateReplay(winner, reqRecord.InputContext, targetIsExplicitID, targetID)
			if evalErr != nil {
				return nil, false, evalErr
			}
			return receipt, true, nil
		}
		return nil, false, fmt.Errorf("insert idempotency record: %w", err)
	}

	if err := insertStepExecutionsTx(ctx, tx, exec.ID, exec.Steps); err != nil {
		return nil, false, err
	}

	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit execution: %w", err)
	}

	var receipt ExecuteWorkflowResponse
	if err := json.Unmarshal(reqRecord.ResponseReceipt, &receipt); err != nil {
		return nil, false, fmt.Errorf("unmarshal stored receipt: %w", err)
	}
	return &receipt, false, nil
}

func (r *executionRepository) CreateExecution(ctx context.Context, exec *WorkflowExecution) (*WorkflowExecution, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin execution: %w", err)
	}
	defer tx.Rollback()

	if err := insertWorkflowExecutionTx(ctx, tx, exec); err != nil {
		return nil, err
	}
	if err := insertStepExecutionsTx(ctx, tx, exec.ID, exec.Steps); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit execution: %w", err)
	}
	return exec, nil
}

func (r *executionRepository) GetExecutionRequest(ctx context.Context, workflowName string, idempotencyKey string) (*ExecutionRequestRecord, error) {
	if r.db == nil {
		return nil, errdefs.ErrExecutionRequestNotFound
	}
	query := `
		SELECT workflow_name, idempotency_key, workflow_execution_id, input_context, response_receipt, created_at
		FROM workflow_execution_requests
		WHERE workflow_name = ? AND idempotency_key = ?
	`
	var (
		rec      ExecutionRequestRecord
		keyBytes []byte
		inputRaw []byte
		respRaw  []byte
	)
	err := r.db.QueryRowContext(ctx, query, workflowName, []byte(idempotencyKey)).Scan(
		&rec.WorkflowName,
		&keyBytes,
		&rec.WorkflowExecutionID,
		&inputRaw,
		&respRaw,
		&rec.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrExecutionRequestNotFound
		}
		return nil, fmt.Errorf("get execution request: %w", err)
	}
	rec.IdempotencyKey = string(keyBytes)
	rec.InputContext = json.RawMessage(inputRaw)
	rec.ResponseReceipt = json.RawMessage(respRaw)
	return &rec, nil
}
