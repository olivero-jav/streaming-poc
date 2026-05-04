//go:build e2e

// Package e2etest holds end-to-end smoke tests that exercise the running
// backend. They are guarded by the `e2e` build tag so `go test ./...` does
// not pull them in. Run with:
//
//	go test -tags=e2e ./internal/e2etest/...
//
// Requirements: backend running on BASE_URL (default http://localhost:8080),
// Postgres up, ffmpeg on PATH.
package e2etest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	defaultBaseURL  = "http://localhost:8080"
	processTimeout  = 60 * time.Second
	pollInterval    = 1 * time.Second
	requestTimeout  = 10 * time.Second
)

func baseURL() string {
	if v := os.Getenv("BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBaseURL
}

func fixturePath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve caller path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "testing", "fixtures", "sample.mp4")
}

// requireBackend skips the test if /health is not reachable, with a useful
// hint instead of a cascade of confusing failures from later steps.
func requireBackend(t *testing.T, base string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("backend not reachable at %s: %v (start it with `go run ./cmd` or your usual command)", base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("backend health check returned %d at %s", resp.StatusCode, base)
	}
}

func TestVODHappyPath(t *testing.T) {
	base := baseURL()
	requireBackend(t, base)

	id := uploadFixture(t, base)
	t.Logf("uploaded video id=%s", id)

	waitForReady(t, base, id)
	t.Logf("video transitioned to ready")

	assertListed(t, base, id)
	playlist := fetchPlaylist(t, base, id)
	segment := firstSegment(t, playlist)
	fetchSegment(t, base, id, segment)
}

// uploadFixture posts the sample.mp4 to /videos and returns the new video ID.
func uploadFixture(t *testing.T, base string) string {
	t.Helper()

	f, err := os.Open(fixturePath(t))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if err := w.WriteField("title", fmt.Sprintf("e2e-vod-%d", time.Now().Unix())); err != nil {
		t.Fatalf("write title field: %v", err)
	}
	fw, err := w.CreateFormFile("file", "sample.mp4")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/videos", body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /videos: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /videos: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var video struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&video); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if video.ID == "" {
		t.Fatal("upload response did not contain an id")
	}
	return video.ID
}

// waitForReady polls GET /videos/:id until status=ready or fails on timeout
// or terminal error status.
func waitForReady(t *testing.T, base, id string) {
	t.Helper()

	deadline := time.Now().Add(processTimeout)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("video %s did not reach status=ready within %s", id, processTimeout)
		}

		status := getVideoStatus(t, base, id)
		switch status {
		case "ready":
			return
		case "error":
			t.Fatalf("video %s ended in status=error", id)
		case "pending", "processing":
			// keep polling
		default:
			t.Fatalf("unexpected status %q for video %s", status, id)
		}

		time.Sleep(pollInterval)
	}
}

func getVideoStatus(t *testing.T, base, id string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/videos/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /videos/%s: %v", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /videos/%s: status=%d", id, resp.StatusCode)
	}
	var v struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode video: %v", err)
	}
	return v.Status
}

// assertListed verifies the new video appears in GET /videos. This validates
// that cache invalidation on upload completion is working — a stale list
// would not contain the new id.
func assertListed(t *testing.T, base, id string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/videos", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /videos: %v", err)
	}
	defer resp.Body.Close()

	var listing struct {
		Items []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	for _, item := range listing.Items {
		if item.ID == id {
			if item.Status != "ready" {
				t.Errorf("listed video %s status: got %q, want %q", id, item.Status, "ready")
			}
			return
		}
	}
	t.Fatalf("video %s not found in /videos listing (%d items)", id, len(listing.Items))
}

func fetchPlaylist(t *testing.T, base, id string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/hls/vod/%s/index.m3u8", base, id), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET playlist: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET playlist: status=%d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read playlist body: %v", err)
	}
	if !strings.HasPrefix(string(body), "#EXTM3U") {
		t.Fatalf("playlist does not start with #EXTM3U, got: %.100q", string(body))
	}
	return string(body)
}

// firstSegment returns the first non-comment, non-blank line from a playlist.
func firstSegment(t *testing.T, playlist string) string {
	t.Helper()
	for _, line := range strings.Split(playlist, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line
	}
	t.Fatalf("playlist has no segments:\n%s", playlist)
	return ""
}

func fetchSegment(t *testing.T, base, id, segment string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/hls/vod/%s/%s", base, id, segment), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET segment: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET segment %s: status=%d", segment, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp2t" {
		t.Errorf("segment Content-Type: got %q, want %q", ct, "video/mp2t")
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read segment body: %v", err)
	}
	if len(body) == 0 {
		t.Errorf("segment %s body is empty", segment)
	}
}
