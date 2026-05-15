package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// contentSecurityPolicy is the directive sent for HTML responses. 'unsafe-inline'
// in script-src is required by the inline onload handler Angular CLI emits for
// async stylesheet loading; 'unsafe-inline' in style-src covers the critical CSS
// inlined by Beasties. blob: in media-src lets hls.js attach a MediaSource to
// the <video>; blob: in worker-src lets it spawn its parsing worker. Both can
// be tightened (nonces) if/when SSR is introduced.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' https://fonts.googleapis.com 'unsafe-inline'; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data:; " +
	"media-src 'self' blob:; " +
	"worker-src 'self' blob:; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'"

// ContentSecurityPolicy returns the CSP directive intended for HTML responses.
// JSON and HLS responses don't need it (CSP only applies to documents).
func ContentSecurityPolicy() string {
	return contentSecurityPolicy
}

// SecurityHeaders is a gin middleware that sets headers applicable to every
// response (nosniff, referrer policy, anti-clickjacking, HSTS over HTTPS).
// CSP is intentionally NOT here — see ContentSecurityPolicy.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("X-Frame-Options", "DENY")

		if isHTTPS(c.Request) {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
