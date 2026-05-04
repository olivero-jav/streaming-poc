package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const defaultTestDSN = "postgres://streaming_user:streaming_pass@localhost:5432/streaming?sslmode=disable"

// newTestDB returns a *sql.DB scoped to a unique temporary schema in the
// configured Postgres instance. The schema is dropped via t.Cleanup. If
// Postgres is unreachable the test is skipped so `go test ./...` does not
// fail when no database is running.
//
// The schema is embedded in the DSN via libpq's `options=-c search_path=...`,
// so every connection in the pool sees the same search_path without having
// to serialize tests to a single connection.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDSN
	}

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bootstrap, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("postgres unavailable (open): %v", err)
	}
	if err := bootstrap.PingContext(ctx); err != nil {
		_ = bootstrap.Close()
		t.Skipf("postgres unavailable (ping): %v", err)
	}
	if _, err := bootstrap.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)); err != nil {
		_ = bootstrap.Close()
		t.Fatalf("create schema %q: %v", schema, err)
	}
	_ = bootstrap.Close()

	schemaDSN, err := dsnWithSearchPath(dsn, schema)
	if err != nil {
		dropSchema(dsn, schema)
		t.Fatalf("build schema-scoped dsn: %v", err)
	}

	db, err := sql.Open("pgx", schemaDSN)
	if err != nil {
		dropSchema(dsn, schema)
		t.Fatalf("open schema-scoped db: %v", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)

	if err := applySchema(ctx, db); err != nil {
		_ = db.Close()
		dropSchema(dsn, schema)
		t.Fatalf("apply schema in %q: %v", schema, err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		dropSchema(dsn, schema)
	})

	return db
}

// dsnWithSearchPath returns dsn with libpq option `-c search_path=<schema>`
// added. Only URL-form DSNs (postgres://...) are supported because that is
// the form used in this project.
func dsnWithSearchPath(dsn, schema string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse dsn: %w", err)
	}
	q := u.Query()
	q.Set("options", "-c search_path="+schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// dropSchema is best-effort cleanup; failures are silent because the test has
// already finished by the time it runs.
func dropSchema(dsn, schema string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return
	}
	defer db.Close()
	_, _ = db.ExecContext(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
}
