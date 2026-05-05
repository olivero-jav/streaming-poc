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

// zeroReader emits an unbounded stream of zero bytes. Used as the source for
// a streaming oversize-upload test so the test does not buffer hundreds of
// megabytes in RAM.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// TestVODRejectsOversizeUpload streams a body larger than the server cap
// (default 500 MB) and expects the request to fail with 413 or a server-side
// connection close — both indicate the MaxBytesReader middleware tripped.
func TestVODRejectsOversizeUpload(t *testing.T) {
	base := baseURL()
	requireBackend(t, base)

	// 600 MB of zeros — comfortably above the default 500 MB cap.
	const payloadBytes int64 = 600 << 20

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer mw.Close()
		if err := mw.WriteField("title", fmt.Sprintf("e2e-big-%d", time.Now().Unix())); err != nil {
			return
		}
		fw, err := mw.CreateFormFile("file", "huge.mp4")
		if err != nil {
			return
		}
		// Ignore write errors: once the server-side cap trips it closes the
		// connection, which surfaces as a write error here. That is the
		// expected outcome, not a test failure.
		_, _ = io.CopyN(fw, zeroReader{}, payloadBytes)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/videos", pr)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		// The server can close the connection mid-stream once the cap trips,
		// which is a legitimate signal that the limit worked.
		t.Logf("request failed after %s (likely server-side close): %v", elapsed, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 413 for oversize upload, got %d: %s", resp.StatusCode, string(raw))
	}
	t.Logf("server returned 413 after %s of streaming", elapsed)
}

// TestVODRejectsNonVideo posts a plain-text payload disguised as `fake.mp4`
// and expects a 415 from the magic-bytes validation. Verifies that the
// extension/filename alone is not enough to fool the upload endpoint.
func TestVODRejectsNonVideo(t *testing.T) {
	base := baseURL()
	requireBackend(t, base)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if err := w.WriteField("title", fmt.Sprintf("e2e-bad-%d", time.Now().Unix())); err != nil {
		t.Fatalf("write title field: %v", err)
	}
	fw, err := w.CreateFormFile("file", "fake.mp4")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte("This is plain text, not a video. The .mp4 extension is a lie.")); err != nil {
		t.Fatalf("write fake content: %v", err)
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

	if resp.StatusCode != http.StatusUnsupportedMediaType {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 415 for non-video upload, got %d: %s", resp.StatusCode, string(raw))
	}
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
