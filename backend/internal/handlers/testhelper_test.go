package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	_ "github.com/jackc/pgx/v5/stdlib"

	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/config"
	"streaming-poc/backend/internal/process"
	"streaming-poc/backend/internal/storage"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const defaultTestDSN = "postgres://streaming_user:streaming_pass@localhost:5432/streaming?sslmode=disable"

// newTestDB returns a *sql.DB scoped to a unique temporary Postgres schema,
// dropped via t.Cleanup. Skips the test if Postgres is unreachable. Mirrors
// the helper in internal/storage; kept duplicated to avoid promoting an
// internal/storagetest package for a single ~80-line file.
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

	db, err := storage.InitPostgres(ctx, schemaDSN)
	if err != nil {
		dropSchema(dsn, schema)
		t.Fatalf("init postgres in %q: %v", schema, err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		dropSchema(dsn, schema)
	})

	return db
}

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

// newTestDeps builds a Deps suitable for handler tests. db may be nil for
// tests whose handler code path never reaches the database. Cache is fail-soft
// (empty URL → no-op), so handler cache calls succeed without a Redis server.
func newTestDeps(t *testing.T, db *sql.DB) *Deps {
	t.Helper()
	return &Deps{
		DB:       db,
		Cache:    cache.New(context.Background(), ""),
		Registry: process.NewRegistry(),
		Cfg: config.Config{
			MaxUploadBytes: 500 << 20,
			BackendRoot:    t.TempDir(),
		},
		AppCtx:       context.Background(),
		TranscodeSem: make(chan struct{}, 1),
		BgWG:         &sync.WaitGroup{},
	}
}
