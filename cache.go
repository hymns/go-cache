// Package cache provides a Laravel-inspired caching library for Go with
// support for multiple backends (memory, file, Redis) configured via env vars.
//
// Quick start:
//
//	c, _ := cache.New()
//
//	c.Put(ctx, "key", "value", time.Minute)
//
//	result, _ := cache.Remember(ctx, c, "users", time.Hour, func() ([]User, error) {
//	    return db.FindAllUsers()
//	})
package cache

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hymns/go-cache/drivers"
	"golang.org/x/sync/singleflight"
)

// Cache wraps a Store with a Laravel-like API.
type Cache struct {
	store  Store
	prefix string
	ttl    time.Duration
	group  singleflight.Group // prevents cache stampedes in Remember/RememberForever
}

// New creates a Cache using ConfigFromEnv.
// This is the standard entry point — configure via environment variables.
func New() (*Cache, error) {
	return NewConfig(ConfigFromEnv())
}

// NewConfig creates a Cache from an explicit Config struct.
func NewConfig(cfg Config) (*Cache, error) {
	var (
		store Store
		err   error
	)

	switch cfg.Driver {
	case DriverMemory, "":
		store = drivers.NewMemory()
	case DriverFile:
		store, err = drivers.NewFile(cfg.FilePath)
	case DriverRedis:
		store, err = drivers.NewRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	default:
		return nil, fmt.Errorf("cache: unknown driver %q (want memory|file|redis)", cfg.Driver)
	}

	if err != nil {
		return nil, err
	}

	return &Cache{store: store, prefix: cfg.Prefix, ttl: cfg.TTL}, nil
}

// NewWithStore creates a Cache around an existing Store.
// Useful for testing or when you need to share a store across multiple Cache instances.
func NewWithStore(store Store, cfg Config) *Cache {
	return &Cache{store: store, prefix: cfg.Prefix, ttl: cfg.TTL}
}

func (c *Cache) prefixed(key string) string {
	if c.prefix == "" {
		return key
	}
	return c.prefix + ":" + key
}

// DefaultTTL returns the configured default TTL.
func (c *Cache) DefaultTTL() time.Duration { return c.ttl }

// ----------------------------------------------------------------------------
// Core read/write
// ----------------------------------------------------------------------------

// Get retrieves a cached value into dest (must be a pointer).
// Returns false if the key does not exist or has expired.
func (c *Cache) Get(ctx context.Context, key string, dest any) (bool, error) {
	b, found, err := c.store.Get(ctx, c.prefixed(key))
	if !found || err != nil {
		return false, err
	}
	return true, json.Unmarshal(b, dest)
}

// Put stores value with the given TTL.
func (c *Cache) Put(ctx context.Context, key string, value any, ttl time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.store.Put(ctx, c.prefixed(key), b, ttl)
}

// Forever stores value without an expiry.
func (c *Cache) Forever(ctx context.Context, key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.store.Forever(ctx, c.prefixed(key), b)
}

// Add stores value only if the key does not already exist.
// Returns true if the value was stored.
func (c *Cache) Add(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	return c.store.Add(ctx, c.prefixed(key), b, ttl)
}

// ----------------------------------------------------------------------------
// Remember helpers (generic, package-level)
// ----------------------------------------------------------------------------

// Remember returns the cached value for key. If the key is missing it calls fn,
// stores the result with ttl, and returns it.
//
// Concurrent calls for the same key on a cache miss are coalesced: only one
// goroutine executes fn and all others receive the same result, preventing
// cache stampedes.
//
//	users, err := cache.Remember(ctx, c, "users.all", time.Hour, func() ([]User, error) {
//	    return db.FindAllUsers()
//	})
func Remember[T any](ctx context.Context, c *Cache, key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	var zero T

	// Fast path: already cached.
	var dest T
	if found, err := c.Get(ctx, key, &dest); found || err != nil {
		return dest, err
	}

	// Slow path: coalesce concurrent misses via singleflight.
	v, err, _ := c.group.Do(key, func() (any, error) {
		// Double-check: another goroutine may have populated the cache while we
		// were waiting for the singleflight slot.
		var dest2 T
		if found, err := c.Get(ctx, key, &dest2); found || err != nil {
			return dest2, err
		}
		val, err := fn()
		if err != nil {
			return nil, err
		}
		return val, c.Put(ctx, key, val, ttl)
	})
	if err != nil {
		return zero, err
	}
	return v.(T), nil
}

// RememberForever is like Remember but stores without expiry.
// Concurrent misses for the same key are coalesced (stampede-safe).
func RememberForever[T any](ctx context.Context, c *Cache, key string, fn func() (T, error)) (T, error) {
	var zero T

	var dest T
	if found, err := c.Get(ctx, key, &dest); found || err != nil {
		return dest, err
	}

	v, err, _ := c.group.Do(key, func() (any, error) {
		var dest2 T
		if found, err := c.Get(ctx, key, &dest2); found || err != nil {
			return dest2, err
		}
		val, err := fn()
		if err != nil {
			return nil, err
		}
		return val, c.Forever(ctx, key, val)
	})
	if err != nil {
		return zero, err
	}
	return v.(T), nil
}

// ----------------------------------------------------------------------------
// Pull, Has, Forget, Flush
// ----------------------------------------------------------------------------

// Pull retrieves a cached value into dest and immediately removes it.
func (c *Cache) Pull(ctx context.Context, key string, dest any) (bool, error) {
	found, err := c.Get(ctx, key, dest)
	if !found || err != nil {
		return found, err
	}
	return true, c.store.Forget(ctx, c.prefixed(key))
}

// Has reports whether a non-expired entry exists for key.
func (c *Cache) Has(ctx context.Context, key string) bool {
	found, _ := c.store.Has(ctx, c.prefixed(key))
	return found
}

// Forget removes a single cache entry.
func (c *Cache) Forget(ctx context.Context, key string) error {
	return c.store.Forget(ctx, c.prefixed(key))
}

// Flush clears all cache entries (respects the store scope, e.g. Redis DB).
func (c *Cache) Flush(ctx context.Context) error {
	return c.store.Flush(ctx)
}

// ----------------------------------------------------------------------------
// Increment / Decrement
// ----------------------------------------------------------------------------

// Increment atomically increases an integer counter by amount.
// Creates the key with value amount if it does not exist.
func (c *Cache) Increment(ctx context.Context, key string, amount int64) (int64, error) {
	return c.store.Increment(ctx, c.prefixed(key), amount)
}

// Decrement atomically decreases an integer counter by amount.
func (c *Cache) Decrement(ctx context.Context, key string, amount int64) (int64, error) {
	return c.store.Decrement(ctx, c.prefixed(key), amount)
}

// ----------------------------------------------------------------------------
// Typed convenience helpers
// ----------------------------------------------------------------------------

// GetString returns the cached string value, or def if missing.
func (c *Cache) GetString(ctx context.Context, key, def string) string {
	var v string
	found, _ := c.Get(ctx, key, &v)
	if !found {
		return def
	}
	return v
}

// GetInt returns the cached int64 counter value, or def if missing.
// Only works with keys written via Increment/Decrement.
func (c *Cache) GetInt(ctx context.Context, key string, def int64) int64 {
	b, found, _ := c.store.Get(ctx, c.prefixed(key))
	if !found || len(b) != 8 {
		return def
	}
	return int64(binary.BigEndian.Uint64(b))
}

// Store returns the underlying Store for driver-specific operations.
func (c *Cache) Store() Store { return c.store }

