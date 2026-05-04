package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateAndGetVideo(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := CreateVideoInput{
		ID:          uuid.NewString(),
		Title:       "test video",
		Description: "a description",
		SourcePath:  "/uploads/test.mp4",
	}

	created, err := CreateVideo(ctx, db, input)
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}

	if created.ID != input.ID {
		t.Errorf("id: got %q, want %q", created.ID, input.ID)
	}
	if created.Title != input.Title {
		t.Errorf("title: got %q, want %q", created.Title, input.Title)
	}
	if created.Description != input.Description {
		t.Errorf("description: got %q, want %q", created.Description, input.Description)
	}
	if created.SourcePath != input.SourcePath {
		t.Errorf("source_path: got %q, want %q", created.SourcePath, input.SourcePath)
	}
	if created.Status != "pending" {
		t.Errorf("default status: got %q, want %q", created.Status, "pending")
	}
	if created.HLSPath != "" {
		t.Errorf("default hls_path: got %q, want empty", created.HLSPath)
	}
	if created.DurationSeconds != 0 {
		t.Errorf("default duration_seconds: got %d, want 0", created.DurationSeconds)
	}
	if created.CreatedAt == "" || created.UpdatedAt == "" {
		t.Errorf("timestamps not set: created=%q updated=%q", created.CreatedAt, created.UpdatedAt)
	}

	fetched, err := GetVideoByID(ctx, db, input.ID)
	if err != nil {
		t.Fatalf("GetVideoByID: %v", err)
	}
	if fetched != created {
		t.Errorf("round-trip mismatch:\n got: %+v\nwant: %+v", fetched, created)
	}
}

func TestGetVideoByIDNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := GetVideoByID(ctx, db, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestUpdateVideoStatus(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := CreateVideo(ctx, db, CreateVideoInput{
		ID:    uuid.NewString(),
		Title: "test video",
	})
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}

	updated, err := UpdateVideoStatus(ctx, db, created.ID, "processing")
	if err != nil {
		t.Fatalf("UpdateVideoStatus: %v", err)
	}
	if updated.Status != "processing" {
		t.Errorf("status: got %q, want %q", updated.Status, "processing")
	}
	if updated.ID != created.ID {
		t.Errorf("id changed: got %q, want %q", updated.ID, created.ID)
	}
	if updated.CreatedAt != created.CreatedAt {
		t.Errorf("created_at must not change, got %q, want %q", updated.CreatedAt, created.CreatedAt)
	}
}

func TestUpdateVideoStatusNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := UpdateVideoStatus(ctx, db, uuid.NewString(), "ready")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestListVideosEmpty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	videos, err := ListVideos(ctx, db)
	if err != nil {
		t.Fatalf("ListVideos: %v", err)
	}
	if len(videos) != 0 {
		t.Errorf("expected empty list, got %d videos", len(videos))
	}
}

func TestListVideosOrderedByCreatedAtDesc(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	titles := []string{"first", "second", "third"}
	for i, title := range titles {
		if _, err := CreateVideo(ctx, db, CreateVideoInput{
			ID:    uuid.NewString(),
			Title: title,
		}); err != nil {
			t.Fatalf("CreateVideo[%d]: %v", i, err)
		}
		// Ensure created_at differs between rows so ORDER BY is deterministic.
		time.Sleep(10 * time.Millisecond)
	}

	videos, err := ListVideos(ctx, db)
	if err != nil {
		t.Fatalf("ListVideos: %v", err)
	}
	if len(videos) != 3 {
		t.Fatalf("expected 3 videos, got %d", len(videos))
	}

	wantOrder := []string{"third", "second", "first"}
	for i, want := range wantOrder {
		if videos[i].Title != want {
			t.Errorf("position %d: got title %q, want %q", i, videos[i].Title, want)
		}
	}
}

func TestSetHLSPath(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := CreateVideo(ctx, db, CreateVideoInput{
		ID:    uuid.NewString(),
		Title: "test video",
	})
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}

	hlsPath := "/hls/vod/" + created.ID + "/index.m3u8"
	if err := SetHLSPath(ctx, db, created.ID, hlsPath); err != nil {
		t.Fatalf("SetHLSPath: %v", err)
	}

	fetched, err := GetVideoByID(ctx, db, created.ID)
	if err != nil {
		t.Fatalf("GetVideoByID: %v", err)
	}
	if fetched.HLSPath != hlsPath {
		t.Errorf("hls_path persisted: got %q, want %q", fetched.HLSPath, hlsPath)
	}
	// SetHLSPath must not change status.
	if fetched.Status != "pending" {
		t.Errorf("status should not change, got %q, want %q", fetched.Status, "pending")
	}
}

func TestSetHLSPathNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := SetHLSPath(ctx, db, uuid.NewString(), "/some/path")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestMarkVideoReady(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := CreateVideo(ctx, db, CreateVideoInput{
		ID:    uuid.NewString(),
		Title: "test video",
	})
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}

	hlsPath := "/hls/vod/" + created.ID + "/index.m3u8"
	const duration = 42

	ready, err := MarkVideoReady(ctx, db, created.ID, hlsPath, duration)
	if err != nil {
		t.Fatalf("MarkVideoReady: %v", err)
	}
	if ready.Status != "ready" {
		t.Errorf("status: got %q, want %q", ready.Status, "ready")
	}
	if ready.HLSPath != hlsPath {
		t.Errorf("hls_path: got %q, want %q", ready.HLSPath, hlsPath)
	}
	if ready.DurationSeconds != duration {
		t.Errorf("duration_seconds: got %d, want %d", ready.DurationSeconds, duration)
	}
}

func TestMarkVideoReadyNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := MarkVideoReady(ctx, db, uuid.NewString(), "/some/path", 10)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestCreateVideoFromStream(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id := uuid.NewString()
	title := "live recording"
	hlsPath := "/hls/live/abc/index.m3u8"

	video, err := CreateVideoFromStream(ctx, db, id, title, hlsPath)
	if err != nil {
		t.Fatalf("CreateVideoFromStream: %v", err)
	}
	if video.ID != id {
		t.Errorf("id: got %q, want %q", video.ID, id)
	}
	if video.Title != title {
		t.Errorf("title: got %q, want %q", video.Title, title)
	}
	if video.HLSPath != hlsPath {
		t.Errorf("hls_path: got %q, want %q", video.HLSPath, hlsPath)
	}
	if video.Status != "ready" {
		t.Errorf("status: got %q, want %q", video.Status, "ready")
	}
	if video.SourcePath != "" {
		t.Errorf("source_path should be empty for stream-derived VODs, got %q", video.SourcePath)
	}
}
