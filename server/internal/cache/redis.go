package cache

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
)

var ErrRedisDisabled = errors.New("redis is disabled")

type RedisConfig struct {
	Addr       string
	Username   string
	Password   string
	DB         int
	TLSEnabled bool
	Required   bool
}

func (c RedisConfig) Enabled() bool {
	return c.Addr != ""
}

type RedisClient struct {
	client *redis.Client
}

func OpenRedis(ctx context.Context, config RedisConfig) (*RedisClient, error) {
	ctx, span := otel.Tracer("watch_together/redis").Start(ctx, "redis.open")
	defer span.End()
	if !config.Enabled() {
		return nil, ErrRedisDisabled
	}

	options := &redis.Options{
		Addr:     config.Addr,
		Username: config.Username,
		Password: config.Password,
		DB:       config.DB,
	}
	if config.TLSEnabled {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &RedisClient{client: client}, nil
}

func (c *RedisClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *RedisClient) Raw() *redis.Client {
	if c == nil {
		return nil
	}
	return c.client
}

func (c *RedisClient) Ping(ctx context.Context) error {
	ctx, span := otel.Tracer("watch_together/redis").Start(ctx, "redis.ping")
	defer span.End()
	if c == nil || c.client == nil {
		return ErrRedisDisabled
	}
	return c.client.Ping(ctx).Err()
}

func (c *RedisClient) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	ctx, span := otel.Tracer("watch_together/redis").Start(ctx, "redis.get_json")
	defer span.End()
	if c == nil || c.client == nil {
		return false, ErrRedisDisabled
	}
	value, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(value, dest); err != nil {
		return false, fmt.Errorf("decode redis json key %q: %w", key, err)
	}
	return true, nil
}

func (c *RedisClient) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	ctx, span := otel.Tracer("watch_together/redis").Start(ctx, "redis.set_json")
	defer span.End()
	if c == nil || c.client == nil {
		return ErrRedisDisabled
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode redis json key %q: %w", key, err)
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *RedisClient) Delete(ctx context.Context, keys ...string) error {
	ctx, span := otel.Tracer("watch_together/redis").Start(ctx, "redis.delete")
	defer span.End()
	if c == nil || c.client == nil {
		return ErrRedisDisabled
	}
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}
