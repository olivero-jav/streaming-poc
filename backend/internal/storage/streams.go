package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Stream is the API-facing model for a live stream entry.
type Stream struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	StreamKey string `json:"stream_key"`
	Status    string `json:"status"`
	HLSPath   string `json:"hls_path,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CreateStreamInput defines the fields accepted when creating a stream.
type CreateStreamInput struct {
	ID        string
	Title     string
	StreamKey string
}

const streamCols = `id, title, stream_key, status, hls_path, started_at, ended_at, created_at, updated_at`

// CreateStream inserts a new stream record and returns it.
func CreateStream(ctx context.Context, db *sql.DB, input CreateStreamInput) (Stream, error) {
	const q = `INSERT INTO streams (id, title, stream_key) VALUES ($1, $2, $3) RETURNING ` + streamCols + `;`
	s, err := scanStreamRow(db.QueryRowContext(ctx, q, input.ID, input.Title, input.StreamKey))
	if err != nil {
		return Stream{}, fmt.Errorf("insert stream: %w", err)
	}
	return s, nil
}

// ListStreams returns all streams ordered by newest first.
func ListStreams(ctx context.Context, db *sql.DB) ([]Stream, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+streamCols+" FROM streams ORDER BY created_at DESC;")
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	defer rows.Close()

	streams := make([]Stream, 0)
	for rows.Next() {
		s, err := scanStreamRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scan stream row: %w", err)
		}
		streams = append(streams, s)
	}
	return streams, rows.Err()
}

// GetStreamByID returns a single stream by its ID.
func GetStreamByID(ctx context.Context, db *sql.DB, id string) (Stream, error) {
	s, err := scanStreamRow(db.QueryRowContext(ctx, "SELECT "+streamCols+" FROM streams WHERE id = $1;", id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Stream{}, fmt.Errorf("stream not found: %w", sql.ErrNoRows)
		}
		return Stream{}, fmt.Errorf("get stream by id: %w", err)
	}
	return s, nil
}

// GetStreamByKey returns a stream by its RTMP stream key.
func GetStreamByKey(ctx context.Context, db *sql.DB, streamKey string) (Stream, error) {
	s, err := scanStreamRow(db.QueryRowContext(ctx, "SELECT "+streamCols+" FROM streams WHERE stream_key = $1;", streamKey))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Stream{}, fmt.Errorf("stream not found: %w", sql.ErrNoRows)
		}
		return Stream{}, fmt.Errorf("get stream by key: %w", err)
	}
	return s, nil
}

// MarkStreamLive sets hls_path and transitions the stream to live status.
func MarkStreamLive(ctx context.Context, db *sql.DB, id, hlsPath string) error {
	const q = `
UPDATE streams
SET status = 'live', hls_path = $1, started_at = NOW(), updated_at = NOW()
WHERE id = $2;`
	result, err := db.ExecContext(ctx, q, hlsPath, id)
	if err != nil {
		return fmt.Errorf("mark stream live: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("stream not found: %w", sql.ErrNoRows)
	}
	return nil
}

// MarkStreamEnded transitions the stream to ended status. Returns a wrapped
// sql.ErrNoRows when no row matches the given id, mirroring MarkStreamLive.
func MarkStreamEnded(ctx context.Context, db *sql.DB, id string) error {
	const q = `
UPDATE streams
SET status = 'ended', ended_at = NOW(), updated_at = NOW()
WHERE id = $1;`
	result, err := db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark stream ended: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("stream not found: %w", sql.ErrNoRows)
	}
	return nil
}

// ResetStaleStreams moves any 'live' streams to 'ended'.
// Called on server startup to clean up state from a previous run.
func ResetStaleStreams(ctx context.Context, db *sql.DB) error {
	const q = `
UPDATE streams
SET status = 'ended', ended_at = NOW(), updated_at = NOW()
WHERE status = 'live';`
	_, err := db.ExecContext(ctx, q)
	return err
}

func scanStreamRow(row *sql.Row) (Stream, error) {
	var s Stream
	var hlsPath sql.NullString
	var startedAt, endedAt sql.NullTime
	var createdAt, updatedAt time.Time

	if err := row.Scan(&s.ID, &s.Title, &s.StreamKey, &s.Status, &hlsPath, &startedAt, &endedAt, &createdAt, &updatedAt); err != nil {
		return Stream{}, err
	}

	s.HLSPath = nullStringToString(hlsPath)
	if startedAt.Valid {
		s.StartedAt = startedAt.Time.UTC().Format(time.RFC3339)
	}
	if endedAt.Valid {
		s.EndedAt = endedAt.Time.UTC().Format(time.RFC3339)
	}
	s.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	s.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return s, nil
}

func scanStreamRows(rows *sql.Rows) (Stream, error) {
	var s Stream
	var hlsPath sql.NullString
	var startedAt, endedAt sql.NullTime
	var createdAt, updatedAt time.Time

	if err := rows.Scan(&s.ID, &s.Title, &s.StreamKey, &s.Status, &hlsPath, &startedAt, &endedAt, &createdAt, &updatedAt); err != nil {
		return Stream{}, err
	}

	s.HLSPath = nullStringToString(hlsPath)
	if startedAt.Valid {
		s.StartedAt = startedAt.Time.UTC().Format(time.RFC3339)
	}
	if endedAt.Valid {
		s.EndedAt = endedAt.Time.UTC().Format(time.RFC3339)
	}
	s.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	s.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return s, nil
}
