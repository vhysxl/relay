// Package testutil contains isolated integration-test fixtures, not application wiring.
package testutil

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

// OpenMySQL creates and migrates a fresh schema per test. TEST_DB_DSN must name
// a test-server account with CREATE/DROP DATABASE privileges. No existing schema
// is modified. An explicitly configured but unavailable server fails the test.
func OpenMySQL(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	configured := dsn != ""
	if !configured {
		t.Skip("MySQL integration requires TEST_DB_DSN (test-server account with CREATE/DROP DATABASE privileges)")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName, cfg.ParseTime, cfg.MultiStatements = "", true, true
	// Existing large-payload repository tests sort full JSON rows on MySQL 8.4.
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	cfg.Params["sort_buffer_size"] = "16777216"
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		err = admin.PingContext(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("configured MySQL unavailable: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	name := "relay_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("cannot find migration root")
		}
		root = parent
	}
	migrations, err := filepath.Glob(filepath.Join(root, "migrations", "*.up.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, path := range migrations {
		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("migrate %s: %v", filepath.Base(path), err)
		}
	}
	return db
}
