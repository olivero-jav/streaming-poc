package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS returns a gin middleware applying CORS rules. allowedOrigins is the
// explicit allow-list; allowNgrok additionally accepts any https origin under
// *.ngrok-free.app or *.ngrok.io (used to expose dev backends without listing
// every random ngrok URL up front).
func CORS(allowedOrigins []string, allowNgrok bool) gin.HandlerFunc {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if isAllowedOrigin(origin, originSet, allowNgrok) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Methods", "GET,POST,PATCH,HEAD,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,Range")
		c.Header("Access-Control-Expose-Headers", "Content-Length,Content-Range,Accept-Ranges")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func isAllowedOrigin(origin string, allowedOrigins map[string]struct{}, allowNgrok bool) bool {
	if origin == "" {
		return false
	}
	if _, ok := allowedOrigins[origin]; ok {
		return true
	}
	if !allowNgrok {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.HasSuffix(host, ".ngrok-free.app") || strings.HasSuffix(host, ".ngrok.io")
}
