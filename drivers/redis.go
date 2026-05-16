package drivers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisDriver stores cache entries in Redis.
type RedisDriver struct {
	client *redis.Client
}

func NewRedis(addr, password string, db int) (*RedisDriver, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("cache/redis: ping failed: %w", err)
	}

	return &RedisDriver{client: client}, nil
}

func (r *RedisDriver) Get(ctx context.Context, key string) ([]byte, bool, error) {
	b, err := r.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

func (r *RedisDriver) Put(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisDriver) Forever(ctx context.Context, key string, value []byte) error {
	return r.client.Set(ctx, key, value, 0).Err()
}

func (r *RedisDriver) Add(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, key, value, ttl).Result()
}

func (r *RedisDriver) Increment(ctx context.Context, key string, amount int64) (int64, error) {
	return r.client.IncrBy(ctx, key, amount).Result()
}

func (r *RedisDriver) Decrement(ctx context.Context, key string, amount int64) (int64, error) {
	return r.client.DecrBy(ctx, key, amount).Result()
}

func (r *RedisDriver) Forget(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisDriver) Flush(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

func (r *RedisDriver) Has(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, key).Result()
	return n > 0, err
}
