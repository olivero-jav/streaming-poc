package cache

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	KeyVideoList  = "videos:list"
	KeyStreamList = "streams:list"
)

func KeyVideo(id string) string  { return "videos:" + id }
func KeyStream(id string) string { return "streams:" + id }

// Client wraps a Redis connection with fail-soft semantics: if redisURL is
// empty or the initial ping fails, rdb stays nil and every method becomes a
// no-op (GetJSON returns false = miss, SetJSON/Del swallow the call). This
// lets the backend keep working when Redis is unavailable.
type Client struct {
	rdb *redis.Client
}

func New(ctx context.Context, redisURL string) *Client {
	if redisURL == "" {
		log.Printf("cache: REDIS_URL empty, running without cache")
		return &Client{}
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("cache: invalid REDIS_URL %q: %v — running without cache", redisURL, err)
		return &Client{}
	}

	rdb := redis.NewClient(opts)
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("cache: ping failed (%v) — running without cache", err)
		_ = rdb.Close()
		return &Client{}
	}

	log.Printf("cache: connected to %s", redisURL)
	return &Client{rdb: rdb}
}

func (c *Client) GetJSON(ctx context.Context, key string, dest any) bool {
	if c == nil || c.rdb == nil {
		return false
	}
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			log.Printf("cache: GET %s failed: %v", key, err)
		}
		return false
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		log.Printf("cache: unmarshal %s failed: %v", key, err)
		return false
	}
	return true
}

func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) {
	if c == nil || c.rdb == nil {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		log.Printf("cache: marshal %s failed: %v", key, err)
		return
	}
	if err := c.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		log.Printf("cache: SET %s failed: %v", key, err)
	}
}

func (c *Client) Del(ctx context.Context, keys ...string) {
	if c == nil || c.rdb == nil || len(keys) == 0 {
		return
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		log.Printf("cache: DEL %v failed: %v", keys, err)
	}
}

func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// IsConnected reports whether the client has a live Redis connection. Returns
// false when the client is in fail-soft mode (rdb=nil after init failure).
func (c *Client) IsConnected() bool {
	return c != nil && c.rdb != nil
}
