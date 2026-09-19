package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrWorkflowNotFound      = errors.New("workflow not found")
	ErrWorkflowAlreadyExists = errors.New("workflow with this name and version already exists")
)

type workflowRepository struct {
	db *sql.DB
}

func NewWorkflowRepository(db *sql.DB) WorkflowRepository {
	return &workflowRepository{db: db}
}

func (r *workflowRepository) CreateWorkflow(ctx context.Context, wf *Workflow) (*Workflow, error) {
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

	_, err := r.db.ExecContext(
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

	return &wf, nil
}
