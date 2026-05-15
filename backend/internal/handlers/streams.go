package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"streaming-poc/backend/internal/cache"
	"streaming-poc/backend/internal/storage"
)

func CreateStream(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Title string `json:"title"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json payload"})
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
			return
		}
		stream, err := storage.CreateStream(c.Request.Context(), d.DB, storage.CreateStreamInput{
			ID:        uuid.NewString(),
			Title:     title,
			StreamKey: uuid.NewString(),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create stream"})
			return
		}
		d.Cache.Del(c.Request.Context(), cache.KeyStreamList)
		c.JSON(http.StatusCreated, stream)
	}
}

func ListStreams(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		var streams []storage.Stream
		if d.Cache.GetJSON(ctx, cache.KeyStreamList, &streams) {
			c.JSON(http.StatusOK, gin.H{"items": streams})
			return
		}

		streams, err := storage.ListStreams(ctx, d.DB)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list streams"})
			return
		}
		d.Cache.SetJSON(ctx, cache.KeyStreamList, streams, 30*time.Second)
		c.JSON(http.StatusOK, gin.H{"items": streams})
	}
}

func GetStream(d *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		id := c.Param("id")

		var cached storage.Stream
		if d.Cache.GetJSON(ctx, cache.KeyStream(id), &cached) {
			c.JSON(http.StatusOK, cached)
			return
		}

		stream, err := storage.GetStreamByID(ctx, d.DB, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "stream not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stream"})
			return
		}
		d.Cache.SetJSON(ctx, cache.KeyStream(id), stream, 60*time.Second)
		c.JSON(http.StatusOK, stream)
	}
}
