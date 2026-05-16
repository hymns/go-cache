package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

// Tagged is a cache scope tied to one or more tags.
// Calling Flush on a Tagged invalidates every entry stored under those tags
// without touching the rest of the cache.
//
//	tc := c.Tags("users", "roles")
//	tc.Put(ctx, "admin", user, time.Hour)
//	tc.Flush(ctx) // only invalidates "users"+"roles" entries
type Tagged struct {
	cache *Cache
	tags  []string
}

// Tags returns a Tagged cache scoped to the given tags.
func (c *Cache) Tags(tags ...string) *Tagged {
	return &Tagged{cache: c, tags: tags}
}

// versionKey is the store key used to hold a tag's current version token.
// It lives outside the normal prefix namespace to avoid collisions.
func (t *Tagged) versionKey(tag string) string {
	if t.cache.prefix != "" {
		return "__tag__:" + t.cache.prefix + ":" + tag
	}
	return "__tag__:" + tag
}

// tagVersion returns the current version token for a tag, creating one if absent.
func (t *Tagged) tagVersion(ctx context.Context, tag string) (string, error) {
	b, found, err := t.cache.store.Get(ctx, t.versionKey(tag))
	if err != nil {
		return "", err
	}
	if found {
		return string(b), nil
	}
	v := randToken()
	return v, t.cache.store.Forever(ctx, t.versionKey(tag), []byte(v))
}

// resolveKey builds the namespaced key by prepending each tag's version token.
// This means rotating any tag's version instantly invalidates all its entries.
func (t *Tagged) resolveKey(ctx context.Context, key string) (string, error) {
	segments := make([]string, 0, len(t.tags)+1)
	for _, tag := range t.tags {
		v, err := t.tagVersion(ctx, tag)
		if err != nil {
			return "", err
		}
		segments = append(segments, v)
	}
	segments = append(segments, key)
	return strings.Join(segments, "|"), nil
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Get retrieves a tagged cache entry into dest.
func (t *Tagged) Get(ctx context.Context, key string, dest any) (bool, error) {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return false, err
	}
	return t.cache.Get(ctx, k, dest)
}

// Put stores a tagged cache entry with the given TTL.
func (t *Tagged) Put(ctx context.Context, key string, value any, ttl time.Duration) error {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return err
	}
	return t.cache.Put(ctx, k, value, ttl)
}

// Forever stores a tagged entry without expiry.
func (t *Tagged) Forever(ctx context.Context, key string, value any) error {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return err
	}
	return t.cache.Forever(ctx, k, value)
}

// Add stores a tagged entry only if the key does not already exist.
func (t *Tagged) Add(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return false, err
	}
	return t.cache.Add(ctx, k, value, ttl)
}

// Pull retrieves and atomically removes a tagged cache entry.
func (t *Tagged) Pull(ctx context.Context, key string, dest any) (bool, error) {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return false, err
	}
	return t.cache.Pull(ctx, k, dest)
}

// Has reports whether a non-expired tagged entry exists.
func (t *Tagged) Has(ctx context.Context, key string) bool {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return false
	}
	return t.cache.Has(ctx, k)
}

// Forget removes a single tagged cache entry.
func (t *Tagged) Forget(ctx context.Context, key string) error {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return err
	}
	return t.cache.Forget(ctx, k)
}

// Flush invalidates all entries under these tags by rotating each tag's
// version token. Old entries become unreachable immediately and will either
// expire (TTL entries) or be orphaned (forever entries, cleaned up on next
// full Flush of the underlying store).
func (t *Tagged) Flush(ctx context.Context) error {
	for _, tag := range t.tags {
		if err := t.cache.store.Forever(ctx, t.versionKey(tag), []byte(randToken())); err != nil {
			return err
		}
	}
	return nil
}

// Increment atomically increases an integer counter under this tag scope.
func (t *Tagged) Increment(ctx context.Context, key string, amount int64) (int64, error) {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return 0, err
	}
	return t.cache.Increment(ctx, k, amount)
}

// Decrement atomically decreases an integer counter under this tag scope.
func (t *Tagged) Decrement(ctx context.Context, key string, amount int64) (int64, error) {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		return 0, err
	}
	return t.cache.Decrement(ctx, k, amount)
}

// ----------------------------------------------------------------------------
// Generic helpers for Tagged
// ----------------------------------------------------------------------------

// TagRemember returns the cached value for key under this tag scope.
// On miss it calls fn, stores the result with ttl, and returns it.
//
//	users, err := cache.TagRemember(ctx, c.Tags("users"), "all", time.Hour, func() ([]User, error) {
//	    return db.FindAllUsers()
//	})
func TagRemember[T any](ctx context.Context, t *Tagged, key string, ttl time.Duration, fn func() (T, error)) (T, error) {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		var zero T
		return zero, err
	}
	return Remember(ctx, t.cache, k, ttl, fn)
}

// TagRememberForever is like TagRemember but stores without expiry.
func TagRememberForever[T any](ctx context.Context, t *Tagged, key string, fn func() (T, error)) (T, error) {
	k, err := t.resolveKey(ctx, key)
	if err != nil {
		var zero T
		return zero, err
	}
	return RememberForever(ctx, t.cache, k, fn)
}
