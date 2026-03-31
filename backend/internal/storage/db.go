package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const initSchema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS videos (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    source_path TEXT,
    hls_path TEXT,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS streams (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    stream_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'live', 'ended')),
    hls_path TEXT,
    started_at DATETIME,
    ended_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

// Streams lifecycle for the current POC:
// - pending: stream key created, waiting for publisher (OBS) to connect
// - live: active stream, video_id can be NULL
// - ended: finished stream, pending post-processing
// InitSQLite opens the SQLite database and ensures required tables exist.
func InitSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite connection: %w", err)
	}

	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if _, err := db.ExecContext(ctx, initSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize sqlite schema: %w", err)
	}

	// Migrate streams table for databases created before title/hls_path were added.
	// ALTER TABLE ADD COLUMN has no IF NOT EXISTS in SQLite; ignore duplicate-column errors.
	for _, m := range []string{
		`ALTER TABLE streams ADD COLUMN title TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE streams ADD COLUMN hls_path TEXT`,
	} {
		if _, err := db.ExecContext(ctx, m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				_ = db.Close()
				return nil, fmt.Errorf("migrate streams schema: %w", err)
			}
		}
	}

	return db, nil
}
