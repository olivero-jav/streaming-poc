package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// newStreamInput returns a CreateStreamInput with fresh UUIDs for ID and key,
// so concurrent tests do not collide on the UNIQUE(stream_key) constraint.
func newStreamInput(title string) CreateStreamInput {
	return CreateStreamInput{
		ID:        uuid.NewString(),
		Title:     title,
		StreamKey: uuid.NewString(),
	}
}

func TestCreateAndGetStream(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := newStreamInput("test stream")

	created, err := CreateStream(ctx, db, input)
	if err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	if created.ID != input.ID {
		t.Errorf("id: got %q, want %q", created.ID, input.ID)
	}
	if created.Title != input.Title {
		t.Errorf("title: got %q, want %q", created.Title, input.Title)
	}
	if created.StreamKey != input.StreamKey {
		t.Errorf("stream_key: got %q, want %q", created.StreamKey, input.StreamKey)
	}
	if created.Status != "pending" {
		t.Errorf("default status: got %q, want %q", created.Status, "pending")
	}
	if created.HLSPath != "" {
		t.Errorf("default hls_path: got %q, want empty", created.HLSPath)
	}
	if created.StartedAt != "" {
		t.Errorf("default started_at: got %q, want empty", created.StartedAt)
	}
	if created.EndedAt != "" {
		t.Errorf("default ended_at: got %q, want empty", created.EndedAt)
	}

	fetched, err := GetStreamByID(ctx, db, input.ID)
	if err != nil {
		t.Fatalf("GetStreamByID: %v", err)
	}
	if fetched != created {
		t.Errorf("round-trip mismatch:\n got: %+v\nwant: %+v", fetched, created)
	}
}

func TestGetStreamByIDNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := GetStreamByID(ctx, db, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestGetStreamByKey(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := newStreamInput("test stream")
	if _, err := CreateStream(ctx, db, input); err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	found, err := GetStreamByKey(ctx, db, input.StreamKey)
	if err != nil {
		t.Fatalf("GetStreamByKey: %v", err)
	}
	if found.ID != input.ID {
		t.Errorf("id: got %q, want %q", found.ID, input.ID)
	}
	if found.StreamKey != input.StreamKey {
		t.Errorf("stream_key: got %q, want %q", found.StreamKey, input.StreamKey)
	}
}

func TestGetStreamByKeyNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := GetStreamByKey(ctx, db, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestListStreamsEmpty(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	streams, err := ListStreams(ctx, db)
	if err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
	if len(streams) != 0 {
		t.Errorf("expected empty list, got %d streams", len(streams))
	}
}

func TestListStreamsOrderedByCreatedAtDesc(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	titles := []string{"first", "second", "third"}
	for i, title := range titles {
		if _, err := CreateStream(ctx, db, newStreamInput(title)); err != nil {
			t.Fatalf("CreateStream[%d]: %v", i, err)
		}
		// Ensure created_at differs between rows so ORDER BY is deterministic.
		time.Sleep(10 * time.Millisecond)
	}

	streams, err := ListStreams(ctx, db)
	if err != nil {
		t.Fatalf("ListStreams: %v", err)
	}
	if len(streams) != 3 {
		t.Fatalf("expected 3 streams, got %d", len(streams))
	}

	wantOrder := []string{"third", "second", "first"}
	for i, want := range wantOrder {
		if streams[i].Title != want {
			t.Errorf("position %d: got title %q, want %q", i, streams[i].Title, want)
		}
	}
}

func TestMarkStreamLive(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := newStreamInput("test stream")
	if _, err := CreateStream(ctx, db, input); err != nil {
		t.Fatalf("CreateStream: %v", err)
	}

	hlsPath := "/hls/live/" + input.ID + "/index.m3u8"
	if err := MarkStreamLive(ctx, db, input.ID, hlsPath); err != nil {
		t.Fatalf("MarkStreamLive: %v", err)
	}

	got, err := GetStreamByID(ctx, db, input.ID)
	if err != nil {
		t.Fatalf("GetStreamByID: %v", err)
	}
	if got.Status != "live" {
		t.Errorf("status: got %q, want %q", got.Status, "live")
	}
	if got.HLSPath != hlsPath {
		t.Errorf("hls_path: got %q, want %q", got.HLSPath, hlsPath)
	}
	if got.StartedAt == "" {
		t.Errorf("started_at should be set after MarkStreamLive, got empty")
	}
}

func TestMarkStreamLiveNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := MarkStreamLive(ctx, db, uuid.NewString(), "/some/path")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestMarkStreamEnded(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := newStreamInput("test stream")
	if _, err := CreateStream(ctx, db, input); err != nil {
		t.Fatalf("CreateStream: %v", err)
	}
	if err := MarkStreamLive(ctx, db, input.ID, "/hls/live/x/index.m3u8"); err != nil {
		t.Fatalf("MarkStreamLive: %v", err)
	}
	if err := MarkStreamEnded(ctx, db, input.ID); err != nil {
		t.Fatalf("MarkStreamEnded: %v", err)
	}

	got, err := GetStreamByID(ctx, db, input.ID)
	if err != nil {
		t.Fatalf("GetStreamByID: %v", err)
	}
	if got.Status != "ended" {
		t.Errorf("status: got %q, want %q", got.Status, "ended")
	}
	if got.EndedAt == "" {
		t.Errorf("ended_at should be set after MarkStreamEnded, got empty")
	}
}

func TestMarkStreamEndedNotFound(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := MarkStreamEnded(ctx, db, uuid.NewString())
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected wrapped sql.ErrNoRows, got %v", err)
	}
}

func TestResetStaleStreams(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// One stream stays in 'pending', two get promoted to 'live' before the reset.
	pendingInput := newStreamInput("pending stream")
	if _, err := CreateStream(ctx, db, pendingInput); err != nil {
		t.Fatalf("CreateStream pending: %v", err)
	}

	liveIDs := make([]string, 2)
	for i := range liveIDs {
		input := newStreamInput("live stream")
		if _, err := CreateStream(ctx, db, input); err != nil {
			t.Fatalf("CreateStream live[%d]: %v", i, err)
		}
		if err := MarkStreamLive(ctx, db, input.ID, "/hls/live/x/index.m3u8"); err != nil {
			t.Fatalf("MarkStreamLive[%d]: %v", i, err)
		}
		liveIDs[i] = input.ID
	}

	if err := ResetStaleStreams(ctx, db); err != nil {
		t.Fatalf("ResetStaleStreams: %v", err)
	}

	// The two previously-live streams must now be 'ended'.
	for i, id := range liveIDs {
		s, err := GetStreamByID(ctx, db, id)
		if err != nil {
			t.Fatalf("GetStreamByID live[%d]: %v", i, err)
		}
		if s.Status != "ended" {
			t.Errorf("live[%d] status after reset: got %q, want %q", i, s.Status, "ended")
		}
		if s.EndedAt == "" {
			t.Errorf("live[%d] ended_at should be set after reset, got empty", i)
		}
	}

	// The pending stream must NOT be touched.
	pending, err := GetStreamByID(ctx, db, pendingInput.ID)
	if err != nil {
		t.Fatalf("GetStreamByID pending: %v", err)
	}
	if pending.Status != "pending" {
		t.Errorf("pending status after reset: got %q, want %q", pending.Status, "pending")
	}
}
