package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Health(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":   "ok",
			"redis_up": d.Cache.IsConnected(),
			"commit":   d.Cfg.GitCommit,
		})
	}
}
