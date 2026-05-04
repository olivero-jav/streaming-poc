package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Streams lifecycle for the current POC:
// - pending: stream key created, waiting for publisher (OBS) to connect
// - live: active stream
// - ended: finished stream, pending post-processing

// InitPostgres opens a PostgreSQL connection and ensures required tables exist.
func InitPostgres(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if err := applySchema(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// applySchema creates the tables and indexes required by the storage layer.
// Idempotent: safe to call against an already-initialized database. Used by
// InitPostgres at startup and by tests that bootstrap a temporary schema.
func applySchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS videos (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			source_path TEXT,
			hls_path TEXT,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS streams (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			stream_key TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'live', 'ended')),
			hls_path TEXT,
			started_at TIMESTAMPTZ,
			ended_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE streams ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE streams ADD COLUMN IF NOT EXISTS hls_path TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_videos_status ON videos(status)`,
		`CREATE INDEX IF NOT EXISTS idx_videos_created_at ON videos(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_streams_stream_key ON streams(stream_key)`,
		`CREATE INDEX IF NOT EXISTS idx_streams_created_at ON streams(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_streams_status ON streams(status)`,
	}

	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize postgres schema: %w", err)
		}
	}

	return nil
}
