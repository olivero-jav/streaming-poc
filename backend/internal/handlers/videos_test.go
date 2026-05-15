package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// multipartBody builds a multipart/form-data body. fileBytes may be nil to
// omit the file field entirely (useful for "missing file" test cases).
func multipartBody(t *testing.T, fields map[string]string, fileName string, fileBytes []byte) (string, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %q: %v", k, err)
		}
	}
	if fileBytes != nil {
		fw, err := mw.CreateFormFile("file", fileName)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(fileBytes); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return mw.FormDataContentType(), buf
}

func newRouterForUpload(d *Deps) *gin.Engine {
	r := gin.New()
	r.POST("/videos", UploadVideo(d))
	return r
}

func TestUploadVideo_MissingFile(t *testing.T) {
	t.Parallel()
	r := newRouterForUpload(newTestDeps(t, nil))

	ct, body := multipartBody(t, map[string]string{"title": "ok"}, "", nil)
	req := httptest.NewRequest(http.MethodPost, "/videos", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d. body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "file is required") {
		t.Errorf("body did not mention missing file: %s", w.Body.String())
	}
}

func TestUploadVideo_MissingTitle(t *testing.T) {
	t.Parallel()
	r := newRouterForUpload(newTestDeps(t, nil))

	// Whitespace-only title — handler must reject it the same as "".
	ct, body := multipartBody(t, map[string]string{"title": "   "}, "x.mp4", []byte("anything"))
	req := httptest.NewRequest(http.MethodPost, "/videos", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d. body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "title is required") {
		t.Errorf("body did not mention missing title: %s", w.Body.String())
	}
}

// TestUploadVideo_RejectsNonVideoMIME exercises the magic-bytes validation:
// a file named foo.mp4 whose actual bytes are plain text must be rejected
// with 415, proving the handler does not trust the filename extension.
func TestUploadVideo_RejectsNonVideoMIME(t *testing.T) {
	t.Parallel()
	r := newRouterForUpload(newTestDeps(t, nil))

	ct, body := multipartBody(t, map[string]string{"title": "bad"}, "fake.mp4", []byte("Just plain text, definitely not a video."))
	req := httptest.NewRequest(http.MethodPost, "/videos", body)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status: got %d, want %d. body=%s", w.Code, http.StatusUnsupportedMediaType, w.Body.String())
	}
	var resp struct {
		DetectedMime string `json:"detected_mime"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.HasPrefix(resp.DetectedMime, "text/") {
		t.Errorf("expected detected_mime to start with text/, got %q", resp.DetectedMime)
	}
}
