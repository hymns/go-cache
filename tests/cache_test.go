package cache_test

import (
	"context"
	"sync"
	"testing"
	"time"

	cache "github.com/hymns/go-cache"
	"github.com/hymns/go-cache/drivers"
)

func newCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.NewConfig(cache.Config{Driver: cache.DriverMemory})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ── Get / Put ─────────────────────────────────────────────────────────────────

func TestCache_PutGet_String(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	noErr(t, c.Put(ctx, "k", "hello", time.Minute))

	var v string
	found, err := c.Get(ctx, "k", &v)
	noErr(t, err)
	isTrue(t, found)
	eq(t, "hello", v)
}

func TestCache_PutGet_Struct(t *testing.T) {
	type User struct{ ID int; Name string }
	c := newCache(t)
	ctx := context.Background()

	noErr(t, c.Put(ctx, "user", User{1, "Ali"}, time.Minute))

	var u User
	found, err := c.Get(ctx, "user", &u)
	noErr(t, err)
	isTrue(t, found)
	eq(t, User{1, "Ali"}, u)
}

func TestCache_Get_Miss(t *testing.T) {
	c := newCache(t)
	var v string
	found, err := c.Get(context.Background(), "nope", &v)
	noErr(t, err)
	isFalse(t, found)
}

func TestCache_TTL_Expires(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	noErr(t, c.Put(ctx, "short", "x", 30*time.Millisecond))
	time.Sleep(80 * time.Millisecond)

	var v string
	found, _ := c.Get(ctx, "short", &v)
	isFalse(t, found)
}

func TestCache_Forever(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	noErr(t, c.Forever(ctx, "perm", 42))
	time.Sleep(10 * time.Millisecond)

	var v int
	found, err := c.Get(ctx, "perm", &v)
	noErr(t, err)
	isTrue(t, found)
	eq(t, 42, v)
}

// ── Has / Forget / Flush ──────────────────────────────────────────────────────

func TestCache_Has(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	c.Put(ctx, "k", "v", time.Minute) //nolint
	isTrue(t, c.Has(ctx, "k"))
	isFalse(t, c.Has(ctx, "missing"))
}

func TestCache_Forget(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	c.Put(ctx, "k", "v", time.Minute) //nolint
	noErr(t, c.Forget(ctx, "k"))
	isFalse(t, c.Has(ctx, "k"))
}

func TestCache_Flush(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	c.Put(ctx, "a", 1, time.Minute) //nolint
	c.Put(ctx, "b", 2, time.Minute) //nolint
	noErr(t, c.Flush(ctx))
	isFalse(t, c.Has(ctx, "a"))
	isFalse(t, c.Has(ctx, "b"))
}

// ── Add / Pull ────────────────────────────────────────────────────────────────

func TestCache_Add(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	ok, err := c.Add(ctx, "k", "first", time.Minute)
	noErr(t, err)
	isTrue(t, ok)

	ok, err = c.Add(ctx, "k", "second", time.Minute)
	noErr(t, err)
	isFalse(t, ok)

	var v string
	c.Get(ctx, "k", &v) //nolint
	eq(t, "first", v)
}

func TestCache_Pull(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	c.Put(ctx, "otp", "999", time.Minute) //nolint

	var v string
	found, err := c.Pull(ctx, "otp", &v)
	noErr(t, err)
	isTrue(t, found)
	eq(t, "999", v)

	found, _ = c.Pull(ctx, "otp", &v)
	isFalse(t, found)
}

// ── Increment / Decrement / GetInt / GetString ────────────────────────────────

func TestCache_Increment(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	n, err := c.Increment(ctx, "hits", 1)
	noErr(t, err)
	eq(t, int64(1), n)

	n, _ = c.Increment(ctx, "hits", 9)
	eq(t, int64(10), n)
}

func TestCache_Decrement(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	c.Increment(ctx, "stock", 10) //nolint
	n, err := c.Decrement(ctx, "stock", 3)
	noErr(t, err)
	eq(t, int64(7), n)
}

func TestCache_GetInt(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	eq(t, int64(0), c.GetInt(ctx, "ctr", 0))
	c.Increment(ctx, "ctr", 5) //nolint
	eq(t, int64(5), c.GetInt(ctx, "ctr", 0))
}

func TestCache_GetString(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	eq(t, "default", c.GetString(ctx, "missing", "default"))
	c.Put(ctx, "name", "Siti", time.Minute) //nolint
	eq(t, "Siti", c.GetString(ctx, "name", "default"))
}

// ── Prefix isolation ──────────────────────────────────────────────────────────

func TestCache_PrefixIsolation(t *testing.T) {
	store := drivers.NewMemory()
	ctx := context.Background()

	a := cache.NewWithStore(store, cache.Config{Prefix: "app-a"})
	b := cache.NewWithStore(store, cache.Config{Prefix: "app-b"})

	a.Put(ctx, "k", "from-a", time.Minute) //nolint

	var v string
	found, _ := b.Get(ctx, "k", &v)
	isFalse(t, found)
}

// ── Remember ─────────────────────────────────────────────────────────────────

func TestRemember_MissCallsFn(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	calls := 0
	v, err := cache.Remember(ctx, c, "k", time.Hour, func() (string, error) {
		calls++
		return "computed", nil
	})
	noErr(t, err)
	eq(t, "computed", v)
	eq(t, 1, calls)
}

func TestRemember_HitSkipsFn(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	cache.Remember(ctx, c, "k", time.Hour, func() (string, error) { return "v", nil }) //nolint

	calls := 0
	v, err := cache.Remember(ctx, c, "k", time.Hour, func() (string, error) {
		calls++
		return "should-not-run", nil
	})
	noErr(t, err)
	eq(t, "v", v)
	eq(t, 0, calls)
}

func TestRemember_TypedStruct(t *testing.T) {
	type Item struct{ Name string }
	c := newCache(t)
	ctx := context.Background()

	item, err := cache.Remember(ctx, c, "item", time.Hour, func() (Item, error) {
		return Item{"widget"}, nil
	})
	noErr(t, err)
	eq(t, Item{"widget"}, item)
}

func TestRememberForever(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	calls := 0
	cache.RememberForever(ctx, c, "k", func() (int, error) { calls++; return 99, nil }) //nolint
	cache.RememberForever(ctx, c, "k", func() (int, error) { calls++; return 0, nil })  //nolint

	eq(t, 1, calls)
	isTrue(t, c.Has(ctx, "k"))
}

// ── Stampede prevention ───────────────────────────────────────────────────────

func TestRemember_StampedeCoalesced(t *testing.T) {
	c := newCache(t)
	ctx := context.Background()

	const goroutines = 100
	var callCount int
	var mu sync.Mutex
	var wg sync.WaitGroup
	results := make([]string, goroutines)

	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v, _ := cache.Remember(ctx, c, "expensive", time.Hour, func() (string, error) {
				mu.Lock()
				callCount++
				mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				return "result", nil
			})
			results[i] = v
		}(i)
	}
	wg.Wait()

	eq(t, 1, callCount)
	for _, r := range results {
		eq(t, "result", r)
	}
}
