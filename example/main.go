package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	cache "github.com/hymns/go-cache"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	c, _ := cache.New()
	ctx := context.Background()

	// ── Tags ────────────────────────────────────────────────────────────────
	fmt.Println("=== Cache Tags ===")

	// Store entries under different tag scopes
	userCache := c.Tags("users")
	postCache := c.Tags("posts")
	mixedCache := c.Tags("users", "posts")

	userCache.Put(ctx, "1", User{1, "Ali"}, time.Hour)
	userCache.Put(ctx, "2", User{2, "Siti"}, time.Hour)
	postCache.Put(ctx, "title", "Hello World", time.Hour)
	mixedCache.Put(ctx, "summary", "1 user, 1 post", time.Hour)

	var u User
	userCache.Get(ctx, "1", &u)
	fmt.Println("user:1 →", u.Name) // Ali

	// Flush only "users" tag — "posts" entries survive
	userCache.Flush(ctx)

	fmt.Println("after flush users:")
	fmt.Println("  user:1 exists?", userCache.Has(ctx, "1"))          // false
	fmt.Println("  user:2 exists?", userCache.Has(ctx, "2"))          // false
	fmt.Println("  posts:title exists?", postCache.Has(ctx, "title")) // true

	// "mixed" requires BOTH tags to be valid — user tag was rotated, so miss
	fmt.Println("  mixed:summary exists?", mixedCache.Has(ctx, "summary")) // false

	// TagRemember — type-safe generic remember with tag scope
	users, _ := cache.TagRemember(ctx, c.Tags("users"), "all", time.Hour, func() ([]User, error) {
		fmt.Println("  → fetching from DB…")
		return []User{{1, "Ali"}, {2, "Siti"}}, nil
	})
	fmt.Println("users:", users)

	// Second call — served from cache (fn NOT called)
	users2, _ := cache.TagRemember(ctx, c.Tags("users"), "all", time.Hour, func() ([]User, error) {
		fmt.Println("  → should NOT print")
		return nil, nil
	})
	fmt.Println("users (cached):", users2)

	// ── Stampede prevention ──────────────────────────────────────────────────
	fmt.Println("\n=== Cache Stampede Prevention ===")

	c.Flush(ctx)
	callCount := 0
	var mu sync.Mutex

	// 50 goroutines hit the same cold key simultaneously
	var wg sync.WaitGroup
	results := make([]string, 50)

	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			val, _ := cache.Remember(ctx, c, "expensive.query", time.Hour, func() (string, error) {
				mu.Lock()
				callCount++
				mu.Unlock()
				time.Sleep(20 * time.Millisecond) // simulate slow DB query
				return "result from DB", nil
			})
			results[i] = val
		}(i)
	}
	wg.Wait()

	fmt.Printf("50 concurrent goroutines, fn called %d time(s)\n", callCount) // 1
	fmt.Println("all got same result:", results[0] == results[49])            // true
}
