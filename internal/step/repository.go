package step

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

type stepRepository struct {
	db *sql.DB
}

// NewStepRepository creates a StepRepository with database connection and optional injected config.
func NewStepRepository(db *sql.DB, _ ...*config.Config) StepRepository {
	return &stepRepository{
		db: db,
	}
}

func (r *stepRepository) CreateStepsTx(ctx context.Context, tx *sql.Tx, workflowID string, steps []Step) error {
	if len(steps) == 0 {
		return nil
	}

	// 1000 steps * 12 params = 12.000 placeholders (limit batch mysql 65.535) -> rarely change so hardcoded
	const batchSize = 1000
	now := time.Now().UTC()

	for start := 0; start < len(steps); start += batchSize {
		end := start + batchSize
		if end > len(steps) {
			end = len(steps)
		}
		batch := steps[start:end]

		var (
			b    strings.Builder
			args = make([]any, 0, len(batch)*12)
		)

		b.WriteString(`
			INSERT INTO step_definitions (
				id, workflow_id, name, type, config, depends_on,
				timeout, retry, compensation, irreversible, created_at, updated_at
			) VALUES 
		`)

		for i, s := range batch {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")

			stepCreatedAt := s.CreatedAt
			if stepCreatedAt.IsZero() {
				stepCreatedAt = now
			}
			stepUpdatedAt := s.UpdatedAt
			if stepUpdatedAt.IsZero() {
				stepUpdatedAt = now
			}

			var configVal any
			if len(s.Config) > 0 {
				configVal = string(s.Config)
			}

			var dependsOnVal any
			if len(s.DependsOn) > 0 {
				bytes, err := json.Marshal(s.DependsOn)
				if err != nil {
					return err
				}
				dependsOnVal = string(bytes)
			}

			var retryVal any
			if s.Retry != nil {
				bytes, err := json.Marshal(s.Retry)
				if err != nil {
					return err
				}
				retryVal = string(bytes)
			}

			var compVal any
			if s.Compensation != nil {
				bytes, err := json.Marshal(s.Compensation)
				if err != nil {
					return err
				}
				compVal = string(bytes)
			}

			args = append(args,
				s.ID,
				workflowID,
				s.Name,
				string(s.Type),
				configVal,
				dependsOnVal,
				s.Timeout,
				retryVal,
				compVal,
				s.Irreversible,
				stepCreatedAt,
				stepUpdatedAt,
			)
		}

		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
	}

	return nil
}

func (r *stepRepository) GetStepsByWorkflowID(ctx context.Context, workflowID string) ([]Step, error) {
	stepsMap, err := r.GetStepsByWorkflowIDs(ctx, []string{workflowID})
	if err != nil {
		return nil, err
	}
	return stepsMap[workflowID], nil
}

func (r *stepRepository) GetStepsByWorkflowIDs(ctx context.Context, workflowIDs []string) (map[string][]Step, error) {
	result := make(map[string][]Step)

	if len(workflowIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(workflowIDs))
	args := make([]any, len(workflowIDs))

	for i, id := range workflowIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, workflow_id, name, type, config, depends_on,
		       timeout, retry, compensation, irreversible, created_at, updated_at
		FROM step_definitions
		WHERE workflow_id IN (%s)
		ORDER BY created_at ASC
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			s            Step
			typeStr      string
			configStr    string
			dependsOnStr sql.NullString
			retryStr     sql.NullString
			compStr      sql.NullString
		)

		err := rows.Scan(
			&s.ID,
			&s.WorkflowID,
			&s.Name,
			&typeStr,
			&configStr,
			&dependsOnStr,
			&s.Timeout,
			&retryStr,
			&compStr,
			&s.Irreversible,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		s.Type = StepType(typeStr)
		if len(configStr) > 0 {
			s.Config = json.RawMessage(configStr)
		}

		if dependsOnStr.Valid && len(dependsOnStr.String) > 0 {
			var deps []string
			if err := json.Unmarshal([]byte(dependsOnStr.String), &deps); err == nil {
				s.DependsOn = deps
			}
		}

		if retryStr.Valid && len(retryStr.String) > 0 {
			var rp RetryPolicy
			if err := json.Unmarshal([]byte(retryStr.String), &rp); err == nil {
				s.Retry = &rp
			}
		}

		if compStr.Valid && len(compStr.String) > 0 {
			var comp Compensation
			if err := json.Unmarshal([]byte(compStr.String), &comp); err == nil {
				s.Compensation = &comp
			}
		}

		result[s.WorkflowID] = append(result[s.WorkflowID], s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *stepRepository) GetStepByID(ctx context.Context, workflowID string, stepID string) (*Step, error) {
	query := `
		SELECT id, workflow_id, name, type, config, depends_on,
		       timeout, retry, compensation, irreversible, created_at, updated_at
		FROM step_definitions
		WHERE workflow_id = ? AND id = ?
	`

	var (
		s            Step
		typeStr      string
		configStr    string
		dependsOnStr sql.NullString
		retryStr     sql.NullString
		compStr      sql.NullString
	)

	err := r.db.QueryRowContext(ctx, query, workflowID, stepID).Scan(
		&s.ID,
		&s.WorkflowID,
		&s.Name,
		&typeStr,
		&configStr,
		&dependsOnStr,
		&s.Timeout,
		&retryStr,
		&compStr,
		&s.Irreversible,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errdefs.ErrStepNotFound
		}
		return nil, err
	}

	s.Type = StepType(typeStr)
	if len(configStr) > 0 {
		s.Config = json.RawMessage(configStr)
	}

	if dependsOnStr.Valid && len(dependsOnStr.String) > 0 {
		var deps []string
		if err := json.Unmarshal([]byte(dependsOnStr.String), &deps); err == nil {
			s.DependsOn = deps
		}
	}

	if retryStr.Valid && len(retryStr.String) > 0 {
		var rp RetryPolicy
		if err := json.Unmarshal([]byte(retryStr.String), &rp); err == nil {
			s.Retry = &rp
		}
	}

	if compStr.Valid && len(compStr.String) > 0 {
		var comp Compensation
		if err := json.Unmarshal([]byte(compStr.String), &comp); err == nil {
			s.Compensation = &comp
		}
	}

	return &s, nil
}

