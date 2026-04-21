package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Video is the API-facing model for a VOD entry.
type Video struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	Status          string `json:"status"`
	SourcePath      string `json:"source_path,omitempty"`
	HLSPath         string `json:"hls_path,omitempty"`
	DurationSeconds int    `json:"duration_seconds"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// CreateVideoInput defines the fields accepted for creating a VOD.
type CreateVideoInput struct {
	ID          string
	Title       string
	Description string
	SourcePath  string
}

// CreateVideo inserts a VOD entry and returns it.
func CreateVideo(ctx context.Context, db *sql.DB, input CreateVideoInput) (Video, error) {
	const q = `
INSERT INTO videos (id, title, description, source_path)
VALUES ($1, $2, $3, $4)
RETURNING id, title, description, status, source_path, hls_path, duration_seconds, created_at, updated_at;
`
	video, err := scanVideoRow(db.QueryRowContext(ctx, q, input.ID, input.Title, input.Description, input.SourcePath))
	if err != nil {
		return Video{}, fmt.Errorf("insert video: %w", err)
	}
	return video, nil
}

// ListVideos returns all VOD entries ordered by newest first.
func ListVideos(ctx context.Context, db *sql.DB) ([]Video, error) {
	const query = `
SELECT id, title, description, status, source_path, hls_path, duration_seconds, created_at, updated_at
FROM videos
ORDER BY created_at DESC;
`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list videos: %w", err)
	}
	defer rows.Close()

	videos := make([]Video, 0)
	for rows.Next() {
		video, scanErr := scanVideoRows(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan video row: %w", scanErr)
		}
		videos = append(videos, video)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate video rows: %w", err)
	}

	return videos, nil
}

// GetVideoByID returns a VOD entry by id.
func GetVideoByID(ctx context.Context, db *sql.DB, id string) (Video, error) {
	const query = `
SELECT id, title, description, status, source_path, hls_path, duration_seconds, created_at, updated_at
FROM videos
WHERE id = $1;
`

	video, err := scanVideoRow(db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Video{}, fmt.Errorf("video not found: %w", err)
		}
		return Video{}, fmt.Errorf("get video by id: %w", err)
	}

	return video, nil
}

// UpdateVideoStatus updates a video processing status and returns the updated row.
func UpdateVideoStatus(ctx context.Context, db *sql.DB, id, status string) (Video, error) {
	const q = `
UPDATE videos
SET status = $1, updated_at = NOW()
WHERE id = $2
RETURNING id, title, description, status, source_path, hls_path, duration_seconds, created_at, updated_at;
`
	video, err := scanVideoRow(db.QueryRowContext(ctx, q, status, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Video{}, fmt.Errorf("video not found: %w", sql.ErrNoRows)
		}
		return Video{}, fmt.Errorf("update video status: %w", err)
	}
	return video, nil
}

// SetHLSPath updates the hls_path of a video without changing its status.
func SetHLSPath(ctx context.Context, db *sql.DB, id, hlsPath string) error {
	const query = `
UPDATE videos
SET hls_path = $1, updated_at = NOW()
WHERE id = $2;
`

	result, err := db.ExecContext(ctx, query, hlsPath, id)
	if err != nil {
		return fmt.Errorf("set hls path: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated rows count: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("video not found: %w", sql.ErrNoRows)
	}

	return nil
}

// MarkVideoReady stores output metadata and marks the video as ready.
func MarkVideoReady(ctx context.Context, db *sql.DB, id, hlsPath string, durationSeconds int) (Video, error) {
	const q = `
UPDATE videos
SET status = 'ready', hls_path = $1, duration_seconds = $2, updated_at = NOW()
WHERE id = $3
RETURNING id, title, description, status, source_path, hls_path, duration_seconds, created_at, updated_at;
`
	video, err := scanVideoRow(db.QueryRowContext(ctx, q, hlsPath, durationSeconds, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Video{}, fmt.Errorf("video not found: %w", sql.ErrNoRows)
		}
		return Video{}, fmt.Errorf("mark video ready: %w", err)
	}
	return video, nil
}

func scanVideoRow(row *sql.Row) (Video, error) {
	var (
		video       Video
		description sql.NullString
		sourcePath  sql.NullString
		hlsPath     sql.NullString
		createdAt   time.Time
		updatedAt   time.Time
	)

	if err := row.Scan(
		&video.ID,
		&video.Title,
		&description,
		&video.Status,
		&sourcePath,
		&hlsPath,
		&video.DurationSeconds,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Video{}, err
	}

	video.Description = nullStringToString(description)
	video.SourcePath = nullStringToString(sourcePath)
	video.HLSPath = nullStringToString(hlsPath)
	video.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	video.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)

	return video, nil
}

func scanVideoRows(rows *sql.Rows) (Video, error) {
	var (
		video       Video
		description sql.NullString
		sourcePath  sql.NullString
		hlsPath     sql.NullString
		createdAt   time.Time
		updatedAt   time.Time
	)

	if err := rows.Scan(
		&video.ID,
		&video.Title,
		&description,
		&video.Status,
		&sourcePath,
		&hlsPath,
		&video.DurationSeconds,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Video{}, err
	}

	video.Description = nullStringToString(description)
	video.SourcePath = nullStringToString(sourcePath)
	video.HLSPath = nullStringToString(hlsPath)
	video.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	video.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)

	return video, nil
}

// CreateVideoFromStream inserts a VOD entry that is already ready for playback,
// derived from a completed live stream. No transcoding step is needed because
// the HLS files were produced during the live session.
func CreateVideoFromStream(ctx context.Context, db *sql.DB, id, title, hlsPath string) (Video, error) {
	const q = `
INSERT INTO videos (id, title, status, hls_path)
VALUES ($1, $2, 'ready', $3)
RETURNING id, title, description, status, source_path, hls_path, duration_seconds, created_at, updated_at;
`
	video, err := scanVideoRow(db.QueryRowContext(ctx, q, id, title, hlsPath))
	if err != nil {
		return Video{}, fmt.Errorf("insert video from stream: %w", err)
	}
	return video, nil
}

func nullStringToString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}
