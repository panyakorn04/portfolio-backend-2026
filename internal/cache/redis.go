package cache

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(redisURL string, ttl time.Duration) (*RedisCache, error) {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil, nil
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &RedisCache{client: redis.NewClient(opt), ttl: ttl}, nil
}

func (c *RedisCache) Enabled() bool { return c != nil && c.client != nil }

func (c *RedisCache) Ping(ctx context.Context) error {
	if !c.Enabled() {
		return nil
	}
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, bool) {
	if !c.Enabled() {
		return nil, false
	}
	value, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return value, true
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte) {
	if !c.Enabled() || len(value) == 0 {
		return
	}
	_ = c.client.Set(ctx, key, value, c.ttl).Err()
}

func (c *RedisCache) DeletePattern(ctx context.Context, pattern string) {
	if !c.Enabled() {
		return
	}
	var cursor uint64
	for {
		keys, next, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			_ = c.client.Del(ctx, keys...).Err()
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

func (c *RedisCache) Close() error {
	if !c.Enabled() {
		return nil
	}
	err := c.client.Close()
	if errors.Is(err, redis.ErrClosed) {
		return nil
	}
	return err
}
