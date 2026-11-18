package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBaselineRoundTrip(t *testing.T) {
	db := OpenMySQL(t)
	ctx := context.Background()
	for _, name := range []string{"000001_init_schema.down.sql", "000001_init_schema.up.sql"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(data)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()`).Scan(&count); err != nil || count != 6 {
		t.Fatalf("baseline tables: %d %v", count, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name IN ('step_definitions','step_executions') AND column_name IN ('queue','activity','input')`).Scan(&count); err != nil || count != 6 {
		t.Fatalf("generic columns: %d %v", count, err)
	}
}
