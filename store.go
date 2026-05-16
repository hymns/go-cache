package cache

import (
	"context"
	"time"
)

// Store is the low-level interface every driver must satisfy.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Forever(ctx context.Context, key string, value []byte) error
	Add(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	Increment(ctx context.Context, key string, amount int64) (int64, error)
	Decrement(ctx context.Context, key string, amount int64) (int64, error)
	Forget(ctx context.Context, key string) error
	Flush(ctx context.Context) error
	Has(ctx context.Context, key string) (bool, error)
}
