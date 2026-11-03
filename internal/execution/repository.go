package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vhysxl/relay/bootstrap/config"
	"github.com/vhysxl/relay/pkg/errdefs"
)

type executionRepository struct {
	db                   *sql.DB
	defaultLeaseDuration time.Duration
}

func NewExecutionRepository(db *sql.DB, cfg *config.Config) Repository {
	lease := 30 * time.Second
	if cfg != nil && cfg.DefaultLeaseDuration > 0 {
		lease = cfg.DefaultLeaseDuration
	}
	return &executionRepository{db: db, defaultLeaseDuration: lease}
}

func (r *executionRepository) ListExecutions(ctx context.Context, limit, offset int) ([]WorkflowExecution, error) {
	if r.db == nil {
		return []WorkflowExecution{}, nil
	}

	query := `
		SELECT e.id, e.workflow_id, w.name, e.status, e.context, e.error_message, e.created_at, e.updated_at
		FROM workflow_executions e
		LEFT JOIN workflow_definitions w ON e.workflow_id = w.id
		ORDER BY e.created_at DESC, e.id DESC
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
		SELECT %s
		FROM step_executions
		WHERE workflow_execution_id IN (%s)
		ORDER BY is_compensation ASC, created_at ASC
	`, stepColumns, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		se, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		result[se.WorkflowExecutionID] = append(result[se.WorkflowExecutionID], se)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
