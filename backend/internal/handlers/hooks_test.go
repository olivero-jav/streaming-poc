package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRouterForHooks(d *Deps) *gin.Engine {
	r := gin.New()
	r.POST("/internal/hooks/publish", PublishHook(d))
	r.POST("/internal/hooks/unpublish", UnpublishHook(d))
	return r
}

func TestPublishHook_InvalidPath(t *testing.T) {
	t.Parallel()
	r := newRouterForHooks(newTestDeps(t, nil))

	cases := []struct {
		name string
		url  string
	}{
		{"missing path query", "/internal/hooks/publish"},
		{"empty path", "/internal/hooks/publish?path="},
		{"path without live/ prefix", "/internal/hooks/publish?path=foo/key"},
		{"path is only live/", "/internal/hooks/publish?path=live/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.url, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status: got %d, want 400. body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPublishHook_UnknownStreamKey(t *testing.T) {
	t.Parallel()
	r := newRouterForHooks(newTestDeps(t, newTestDB(t)))

	req := httptest.NewRequest(http.MethodPost, "/internal/hooks/publish?path=live/nonexistent-key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404. body=%s", w.Code, w.Body.String())
	}
}

// The happy-path publish hook spawns ffmpeg in a background goroutine, which
// is awkward to drain in a unit test. That path is covered by the e2e/live
// flow with MediaMTX (see testing/hls-live).

func TestUnpublishHook_InvalidPath(t *testing.T) {
	t.Parallel()
	r := newRouterForHooks(newTestDeps(t, nil))

	req := httptest.NewRequest(http.MethodPost, "/internal/hooks/unpublish?path=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400. body=%s", w.Code, w.Body.String())
	}
}

func TestUnpublishHook_UnknownKeyIsNoOp(t *testing.T) {
	t.Parallel()
	r := newRouterForHooks(newTestDeps(t, nil))

	req := httptest.NewRequest(http.MethodPost, "/internal/hooks/unpublish?path=live/unknown-key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200. body=%s", w.Code, w.Body.String())
	}
}
