package cache_test

import (
	"context"
	"testing"
	"time"

	cache "github.com/hymns/go-cache"
)

func newTaggedCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.NewConfig(cache.Config{Driver: cache.DriverMemory})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ── Basic operations ──────────────────────────────────────────────────────────

func TestTagged_PutGet(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	noErr(t, c.Tags("users").Put(ctx, "1", "Ali", time.Hour))

	var v string
	found, err := c.Tags("users").Get(ctx, "1", &v)
	noErr(t, err)
	isTrue(t, found)
	eq(t, "Ali", v)
}

func TestTagged_Has(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	c.Tags("users").Put(ctx, "k", "v", time.Hour) //nolint
	isTrue(t, c.Tags("users").Has(ctx, "k"))
	isFalse(t, c.Tags("users").Has(ctx, "missing"))
}

func TestTagged_Forget(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	c.Tags("users").Put(ctx, "k", "v", time.Hour) //nolint
	noErr(t, c.Tags("users").Forget(ctx, "k"))
	isFalse(t, c.Tags("users").Has(ctx, "k"))
}

func TestTagged_Pull(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	c.Tags("users").Put(ctx, "token", "abc123", time.Hour) //nolint

	var v string
	found, err := c.Tags("users").Pull(ctx, "token", &v)
	noErr(t, err)
	isTrue(t, found)
	eq(t, "abc123", v)
	isFalse(t, c.Tags("users").Has(ctx, "token"))
}

func TestTagged_Add(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	ok, err := c.Tags("users").Add(ctx, "k", "first", time.Hour)
	noErr(t, err)
	isTrue(t, ok)

	ok, err = c.Tags("users").Add(ctx, "k", "second", time.Hour)
	noErr(t, err)
	isFalse(t, ok)
}

// ── Flush ─────────────────────────────────────────────────────────────────────

func TestTagged_Flush_InvalidatesTaggedEntries(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	c.Tags("users").Put(ctx, "1", "Ali", time.Hour)  //nolint
	c.Tags("users").Put(ctx, "2", "Siti", time.Hour) //nolint
	noErr(t, c.Tags("users").Flush(ctx))

	isFalse(t, c.Tags("users").Has(ctx, "1"))
	isFalse(t, c.Tags("users").Has(ctx, "2"))
}

func TestTagged_Flush_DoesNotAffectOtherTags(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	c.Tags("users").Put(ctx, "u1", "Ali", time.Hour)   //nolint
	c.Tags("posts").Put(ctx, "p1", "Hello", time.Hour) //nolint
	noErr(t, c.Tags("users").Flush(ctx))

	isFalse(t, c.Tags("users").Has(ctx, "u1"))
	isTrue(t, c.Tags("posts").Has(ctx, "p1"))
}

func TestTagged_MultiTag_AllTagsMustBeValid(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	c.Tags("users", "posts").Put(ctx, "feed", "data", time.Hour) //nolint
	isTrue(t, c.Tags("users", "posts").Has(ctx, "feed"))

	c.Tags("users").Flush(ctx) //nolint

	isFalse(t, c.Tags("users", "posts").Has(ctx, "feed"))
	c.Tags("posts").Put(ctx, "p", "still here", time.Hour) //nolint
	isTrue(t, c.Tags("posts").Has(ctx, "p"))
}

func TestTagged_IsolatedFromUntagged(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	c.Put(ctx, "k", "untagged", time.Hour)             //nolint
	c.Tags("users").Put(ctx, "k", "tagged", time.Hour) //nolint

	var plain, tagged string
	c.Get(ctx, "k", &plain)                 //nolint
	c.Tags("users").Get(ctx, "k", &tagged) //nolint
	eq(t, "untagged", plain)
	eq(t, "tagged", tagged)

	c.Tags("users").Flush(ctx) //nolint
	isTrue(t, c.Has(ctx, "k"))
}

// ── Increment / Decrement ─────────────────────────────────────────────────────

func TestTagged_Increment(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	n, err := c.Tags("stats").Increment(ctx, "views", 1)
	noErr(t, err)
	eq(t, int64(1), n)

	n, _ = c.Tags("stats").Increment(ctx, "views", 4)
	eq(t, int64(5), n)

	n, _ = c.Tags("stats").Decrement(ctx, "views", 2)
	eq(t, int64(3), n)
}

// ── TagRemember ───────────────────────────────────────────────────────────────

func TestTagRemember_MissCallsFn(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	calls := 0
	v, err := cache.TagRemember(ctx, c.Tags("users"), "all", time.Hour, func() ([]string, error) {
		calls++
		return []string{"Ali", "Siti"}, nil
	})
	noErr(t, err)
	eq(t, []string{"Ali", "Siti"}, v)
	eq(t, 1, calls)
}

func TestTagRemember_HitSkipsFn(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	cache.TagRemember(ctx, c.Tags("users"), "all", time.Hour, func() (string, error) { return "v", nil }) //nolint

	calls := 0
	cache.TagRemember(ctx, c.Tags("users"), "all", time.Hour, func() (string, error) { //nolint
		calls++
		return "should-not-run", nil
	})
	eq(t, 0, calls)
}

func TestTagRemember_FlushInvalidatesAndRefetches(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	calls := 0
	fn := func() (string, error) { calls++; return "data", nil }

	cache.TagRemember(ctx, c.Tags("users"), "list", time.Hour, fn) //nolint
	eq(t, 1, calls)

	c.Tags("users").Flush(ctx) //nolint

	cache.TagRemember(ctx, c.Tags("users"), "list", time.Hour, fn) //nolint
	eq(t, 2, calls)
}

func TestTagRememberForever(t *testing.T) {
	c := newTaggedCache(t)
	ctx := context.Background()

	calls := 0
	cache.TagRememberForever(ctx, c.Tags("cfg"), "settings", func() (map[string]string, error) { //nolint
		calls++
		return map[string]string{"theme": "dark"}, nil
	})
	cache.TagRememberForever(ctx, c.Tags("cfg"), "settings", func() (map[string]string, error) { //nolint
		calls++
		return nil, nil
	})
	eq(t, 1, calls)
}
