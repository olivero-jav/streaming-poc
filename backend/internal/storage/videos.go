package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	const insertQuery = `
INSERT INTO videos (id, title, description, source_path)
VALUES (?, ?, ?, ?);
`

	if _, err := db.ExecContext(ctx, insertQuery, input.ID, input.Title, input.Description, input.SourcePath); err != nil {
		return Video{}, fmt.Errorf("insert video: %w", err)
	}

	const selectQuery = `
SELECT id, title, description, status, source_path, hls_path, duration_seconds, created_at, updated_at
FROM videos
WHERE id = ?;
`

	video, err := scanVideoRow(db.QueryRowContext(ctx, selectQuery, input.ID))
	if err != nil {
		return Video{}, fmt.Errorf("read created video: %w", err)
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
WHERE id = ?;
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
	const updateQuery = `
UPDATE videos
SET status = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
`

	result, err := db.ExecContext(ctx, updateQuery, status, id)
	if err != nil {
		return Video{}, fmt.Errorf("update video status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Video{}, fmt.Errorf("read updated rows count: %w", err)
	}
	if rowsAffected == 0 {
		return Video{}, fmt.Errorf("video not found: %w", sql.ErrNoRows)
	}

	video, err := GetVideoByID(ctx, db, id)
	if err != nil {
		return Video{}, fmt.Errorf("read updated video: %w", err)
	}

	return video, nil
}

// SetHLSPath updates the hls_path of a video without changing its status.
func SetHLSPath(ctx context.Context, db *sql.DB, id, hlsPath string) error {
	const query = `
UPDATE videos
SET hls_path = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
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
	const updateQuery = `
UPDATE videos
SET status = 'ready', hls_path = ?, duration_seconds = ?, updated_at = CURRENT_TIMESTAMP
WHERE id = ?;
`

	result, err := db.ExecContext(ctx, updateQuery, hlsPath, durationSeconds, id)
	if err != nil {
		return Video{}, fmt.Errorf("mark video ready: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return Video{}, fmt.Errorf("read updated rows count: %w", err)
	}
	if rowsAffected == 0 {
		return Video{}, fmt.Errorf("video not found: %w", sql.ErrNoRows)
	}

	video, err := GetVideoByID(ctx, db, id)
	if err != nil {
		return Video{}, fmt.Errorf("read updated video: %w", err)
	}

	return video, nil
}

func scanVideoRow(row *sql.Row) (Video, error) {
	var (
		video       Video
		description sql.NullString
		sourcePath  sql.NullString
		hlsPath     sql.NullString
	)

	if err := row.Scan(
		&video.ID,
		&video.Title,
		&description,
		&video.Status,
		&sourcePath,
		&hlsPath,
		&video.DurationSeconds,
		&video.CreatedAt,
		&video.UpdatedAt,
	); err != nil {
		return Video{}, err
	}

	video.Description = nullStringToString(description)
	video.SourcePath = nullStringToString(sourcePath)
	video.HLSPath = nullStringToString(hlsPath)

	return video, nil
}

func scanVideoRows(rows *sql.Rows) (Video, error) {
	var (
		video       Video
		description sql.NullString
		sourcePath  sql.NullString
		hlsPath     sql.NullString
	)

	if err := rows.Scan(
		&video.ID,
		&video.Title,
		&description,
		&video.Status,
		&sourcePath,
		&hlsPath,
		&video.DurationSeconds,
		&video.CreatedAt,
		&video.UpdatedAt,
	); err != nil {
		return Video{}, err
	}

	video.Description = nullStringToString(description)
	video.SourcePath = nullStringToString(sourcePath)
	video.HLSPath = nullStringToString(hlsPath)

	return video, nil
}

func nullStringToString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}
