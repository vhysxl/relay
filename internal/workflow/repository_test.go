package workflow_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/vhysxl/relay/internal/step"
	"github.com/vhysxl/relay/internal/workflow"
)

// A small database/sql connector keeps these I/O contract tests independent of MySQL.
type summaryConnector struct {
	query func(context.Context, string, []driver.NamedValue) (driver.Rows, error)
}

func (c *summaryConnector) Connect(context.Context) (driver.Conn, error) {
	return &summaryConn{connector: c}, nil
}
func (c *summaryConnector) Driver() driver.Driver { return summaryDriver{} }

type summaryDriver struct{}

func (summaryDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type summaryConn struct{ connector *summaryConnector }

func (*summaryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepared statement")
}
func (*summaryConn) Close() error { return nil }
func (*summaryConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *summaryConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.connector.query(ctx, query, args)
}

type summaryRows struct {
	values [][]driver.Value
	err    error
}

func (*summaryRows) Columns() []string {
	return []string{"id", "name", "version", "created_at", "updated_at", "step_count"}
}
func (*summaryRows) Close() error { return nil }
func (r *summaryRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		if r.err != nil {
			return r.err
		}
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

type unexpectedStepReader struct {
	step.StepRepository
	t *testing.T
}

func (r unexpectedStepReader) GetStepsByWorkflowID(context.Context, string) ([]step.Step, error) {
	r.t.Fatal("list must not load step definitions")
	return nil, nil
}
func (r unexpectedStepReader) GetStepsByWorkflowIDs(context.Context, []string) (map[string][]step.Step, error) {
	r.t.Fatal("list must not load step definitions")
	return nil, nil
}

func TestWorkflowRepository_ListWorkflows_MetadataOnly(t *testing.T) {
	now := time.Now().UTC()
	queries := 0
	db := sql.OpenDB(&summaryConnector{query: func(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
		queries++
		for _, field := range []string{"context", "config", "depends_on", "compensation"} {
			if strings.Contains(strings.ToLower(query), field) {
				t.Errorf("list query must not load definition payload field %q", field)
			}
		}
		if len(args) != 2 || args[0].Value != int64(20) || args[1].Value != int64(10) {
			t.Fatalf("unexpected pagination arguments: %+v", args)
		}
		return &summaryRows{values: [][]driver.Value{
			{"wf-1", "orders", int64(3), now, now, int64(12000)},
			{"wf-2", "empty", int64(1), now, now, int64(0)},
		}}, nil
	}})
	t.Cleanup(func() { db.Close() })
	repo := workflow.NewWorkflowRepository(db, unexpectedStepReader{t: t})
	items, err := repo.ListWorkflows(context.Background(), 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	if queries != 1 {
		t.Fatalf("expected one metadata/count query, got %d", queries)
	}
	if len(items) != 2 || items[0].Name != "orders" || items[0].Version != 3 || items[0].StepCount != 12000 || items[1].StepCount != 0 {
		t.Fatalf("unexpected summaries: %+v", items)
	}
	if !items[0].CreatedAt.Equal(now) || !items[0].UpdatedAt.Equal(now) {
		t.Fatalf("timestamps not preserved: %+v", items[0])
	}
}

func TestWorkflowRepository_ListWorkflows_EmptyAndErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	rowErr := errors.New("row read failed")
	for _, tc := range []struct {
		name     string
		rows     *summaryRows
		queryErr error
		wantErr  error
	}{
		{name: "empty", rows: &summaryRows{}},
		{name: "query error", queryErr: dbErr, wantErr: dbErr},
		{name: "row error", rows: &summaryRows{err: rowErr}, wantErr: rowErr},
		{name: "scan error", rows: &summaryRows{values: [][]driver.Value{{"wf-1", "orders", "invalid-version", time.Now(), time.Now(), int64(1)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := sql.OpenDB(&summaryConnector{query: func(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
				if tc.queryErr != nil {
					return nil, tc.queryErr
				}
				return tc.rows, nil
			}})
			t.Cleanup(func() { db.Close() })
			repo := workflow.NewWorkflowRepository(db, unexpectedStepReader{t: t})
			items, err := repo.ListWorkflows(context.Background(), 10, 0)
			if tc.name == "scan error" {
				if err == nil {
					t.Fatal("expected malformed metadata to return a scan error")
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
			if len(items) != 0 {
				t.Fatalf("expected no summaries, got %+v", items)
			}
		})
	}
}
