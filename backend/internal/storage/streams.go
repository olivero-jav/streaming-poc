package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	const q = `INSERT INTO streams (id, title, stream_key) VALUES (?, ?, ?);`
	if _, err := db.ExecContext(ctx, q, input.ID, input.Title, input.StreamKey); err != nil {
		return Stream{}, fmt.Errorf("insert stream: %w", err)
	}
	return GetStreamByID(ctx, db, input.ID)
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
	s, err := scanStreamRow(db.QueryRowContext(ctx, "SELECT "+streamCols+" FROM streams WHERE id = ?;", id))
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
	s, err := scanStreamRow(db.QueryRowContext(ctx, "SELECT "+streamCols+" FROM streams WHERE stream_key = ?;", streamKey))
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
SET status = 'live', hls_path = ?, started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;`
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

// MarkStreamEnded transitions the stream to ended status.
func MarkStreamEnded(ctx context.Context, db *sql.DB, id string) error {
	const q = `
UPDATE streams
SET status = 'ended', ended_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;`
	if _, err := db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("mark stream ended: %w", err)
	}
	return nil
}

// ResetStaleStreams moves any 'live' streams to 'ended'.
// Called on server startup to clean up state from a previous run.
func ResetStaleStreams(ctx context.Context, db *sql.DB) error {
	const q = `
UPDATE streams
SET status = 'ended', ended_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE status = 'live';`
	_, err := db.ExecContext(ctx, q)
	return err
}

func scanStreamRow(row *sql.Row) (Stream, error) {
	var s Stream
	var hlsPath, startedAt, endedAt sql.NullString
	if err := row.Scan(&s.ID, &s.Title, &s.StreamKey, &s.Status, &hlsPath, &startedAt, &endedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return Stream{}, err
	}
	s.HLSPath = nullStringToString(hlsPath)
	s.StartedAt = nullStringToString(startedAt)
	s.EndedAt = nullStringToString(endedAt)
	return s, nil
}

func scanStreamRows(rows *sql.Rows) (Stream, error) {
	var s Stream
	var hlsPath, startedAt, endedAt sql.NullString
	if err := rows.Scan(&s.ID, &s.Title, &s.StreamKey, &s.Status, &hlsPath, &startedAt, &endedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return Stream{}, err
	}
	s.HLSPath = nullStringToString(hlsPath)
	s.StartedAt = nullStringToString(startedAt)
	s.EndedAt = nullStringToString(endedAt)
	return s, nil
}
