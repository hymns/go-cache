package cache_test

import (
	"context"
	"os"
	"testing"
	"time"

	cache "github.com/hymns/go-cache"
	"github.com/hymns/go-cache/drivers"
)

// testStore is the shared behavioural suite run against every Store driver.
func testStore(t *testing.T, s cache.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("get_miss", func(t *testing.T) {
		_, found, err := s.Get(ctx, "no-such-key")
		noErr(t, err)
		isFalse(t, found)
	})

	t.Run("put_get", func(t *testing.T) {
		noErr(t, s.Put(ctx, "pk", []byte("hello"), time.Minute))
		b, found, err := s.Get(ctx, "pk")
		noErr(t, err)
		isTrue(t, found)
		eq(t, []byte("hello"), b)
	})

	t.Run("overwrite", func(t *testing.T) {
		noErr(t, s.Put(ctx, "ow", []byte("v1"), time.Minute))
		noErr(t, s.Put(ctx, "ow", []byte("v2"), time.Minute))
		b, _, _ := s.Get(ctx, "ow")
		eq(t, []byte("v2"), b)
	})

	t.Run("ttl_expires", func(t *testing.T) {
		noErr(t, s.Put(ctx, "ttl", []byte("x"), 50*time.Millisecond))
		time.Sleep(150 * time.Millisecond)
		_, found, err := s.Get(ctx, "ttl")
		noErr(t, err)
		isFalse(t, found)
	})

	t.Run("forever_does_not_expire", func(t *testing.T) {
		noErr(t, s.Forever(ctx, "fk", []byte("y")))
		time.Sleep(10 * time.Millisecond)
		_, found, err := s.Get(ctx, "fk")
		noErr(t, err)
		isTrue(t, found)
	})

	t.Run("has", func(t *testing.T) {
		noErr(t, s.Put(ctx, "hk", []byte("v"), time.Minute))
		found, err := s.Has(ctx, "hk")
		noErr(t, err)
		isTrue(t, found)

		found, err = s.Has(ctx, "hk-missing")
		noErr(t, err)
		isFalse(t, found)
	})

	t.Run("forget", func(t *testing.T) {
		noErr(t, s.Put(ctx, "fgt", []byte("v"), time.Minute))
		noErr(t, s.Forget(ctx, "fgt"))
		_, found, _ := s.Get(ctx, "fgt")
		isFalse(t, found)

		noErr(t, s.Forget(ctx, "fgt-missing")) // must not error
	})

	t.Run("add_new_key", func(t *testing.T) {
		noErr(t, s.Forget(ctx, "add-k"))
		ok, err := s.Add(ctx, "add-k", []byte("v1"), time.Minute)
		noErr(t, err)
		isTrue(t, ok)
	})

	t.Run("add_existing_no_overwrite", func(t *testing.T) {
		noErr(t, s.Put(ctx, "add-ex", []byte("original"), time.Minute))
		ok, err := s.Add(ctx, "add-ex", []byte("new"), time.Minute)
		noErr(t, err)
		isFalse(t, ok)
		b, _, _ := s.Get(ctx, "add-ex")
		eq(t, []byte("original"), b)
	})

	t.Run("increment", func(t *testing.T) {
		noErr(t, s.Forget(ctx, "ctr"))
		n, err := s.Increment(ctx, "ctr", 1)
		noErr(t, err)
		eq(t, int64(1), n)
		n, err = s.Increment(ctx, "ctr", 4)
		noErr(t, err)
		eq(t, int64(5), n)
	})

	t.Run("decrement", func(t *testing.T) {
		noErr(t, s.Forget(ctx, "ctr2"))
		s.Increment(ctx, "ctr2", 10) //nolint
		n, err := s.Decrement(ctx, "ctr2", 3)
		noErr(t, err)
		eq(t, int64(7), n)
	})

	t.Run("flush", func(t *testing.T) {
		noErr(t, s.Put(ctx, "fl-a", []byte("1"), time.Minute))
		noErr(t, s.Put(ctx, "fl-b", []byte("2"), time.Minute))
		noErr(t, s.Flush(ctx))
		_, found, _ := s.Get(ctx, "fl-a")
		isFalse(t, found)
		_, found, _ = s.Get(ctx, "fl-b")
		isFalse(t, found)
	})
}

func TestMemoryDriver(t *testing.T) { testStore(t, drivers.NewMemory()) }

func TestFileDriver(t *testing.T) {
	s, err := drivers.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	testStore(t, s)
}

func TestRedisDriver(t *testing.T) {
	addr := os.Getenv("CACHE_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	s, err := drivers.NewRedis(addr, "", 0)
	if err != nil {
		t.Skipf("Redis not available at %s — skipping: %v", addr, err)
	}
	testStore(t, s)
}
