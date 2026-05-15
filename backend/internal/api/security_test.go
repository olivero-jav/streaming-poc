package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter() *gin.Engine {
	r := gin.New()
	r.Use(SecurityHeaders())
	r.GET("/json", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/html", func(c *gin.Context) {
		c.Header("Content-Security-Policy", ContentSecurityPolicy())
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<!doctype html><html></html>"))
	})
	return r
}

func TestSecurityHeaders_CommonHeadersAlwaysSet(t *testing.T) {
	r := newRouter()

	for _, path := range []string{"/json", "/html"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q, want nosniff", path, got)
		}
		if got := w.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
			t.Errorf("%s: Referrer-Policy = %q, want strict-origin-when-cross-origin", path, got)
		}
		if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s: X-Frame-Options = %q, want DENY", path, got)
		}
	}
}

func TestSecurityHeaders_CSPOnlyOnHTML(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("/json: CSP should be empty, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/html", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("/html: CSP header missing")
	}
	for _, directive := range []string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' https://fonts.googleapis.com 'unsafe-inline'",
		"font-src 'self' https://fonts.gstatic.com",
		"img-src 'self' data:",
		"media-src 'self' blob:",
		"worker-src 'self' blob:",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP missing directive %q; got %q", directive, csp)
		}
	}
}

func TestSecurityHeaders_HSTSOffOverHTTP(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should be absent over plain HTTP, got %q", got)
	}
}

func TestSecurityHeaders_HSTSOnWithTLS(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("HSTS should be set when request.TLS is non-nil")
	}
}

func TestSecurityHeaders_HSTSOnWithForwardedProto(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("HSTS should be set when X-Forwarded-Proto is https")
	}
}

func TestSecurityHeaders_HSTSOffWithHTTPForwardedProto(t *testing.T) {
	r := newRouter()

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	req.Header.Set("X-Forwarded-Proto", "http")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should be absent when X-Forwarded-Proto is http, got %q", got)
	}
}
