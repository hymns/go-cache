# go-cache

[![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/hymns/go-cache)](https://github.com/hymns/go-cache/releases) [![Go Version](https://img.shields.io/badge/go-1.24.0-blue.svg)](https://golang.org/dl/) [![Go Report Card](https://goreportcard.com/badge/github.com/hymns/go-cache)](https://goreportcard.com/report/github.com/hymns/go-cache) [![GoDoc](https://godoc.org/github.com/hymns/go-cache?status.svg)](https://pkg.go.dev/github.com/hymns/go-cache) [![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A Laravel-inspired caching library for Go with support for multiple backends, cache tags, and built-in stampede prevention.

```go
c, _ := cache.New()

users, _ := cache.Remember(ctx, c, "users.all", time.Hour, func() ([]User, error) {
    return db.FindAllUsers() // called once, result cached for 1 hour
})
```

## Installation

```bash
go get github.com/hymns/go-cache
```

Requires Go 1.21+.

## Drivers

| Driver | Description | Persistence |
|--------|-------------|-------------|
| `memory` | In-process store using a sync'd map | No — lost on restart |
| `file` | JSON files on disk, organised in subdirectories | Yes |
| `redis` | Redis via [go-redis/v9](https://github.com/redis/go-redis) | Yes |

## Configuration

### Via environment variables

```bash
CACHE_DRIVER=memory        # memory | file | redis  (default: memory)
CACHE_PREFIX=myapp         # key prefix, e.g. "myapp:users.all"
CACHE_TTL=3600             # default TTL in seconds  (default: 3600)

# Redis
CACHE_REDIS_ADDR=127.0.0.1:6379
CACHE_REDIS_PASSWORD=secret
CACHE_REDIS_DB=0

# File driver
CACHE_FILE_PATH=/var/cache/myapp
```

```go
c, err := cache.New()
```

### Via struct

```go
c, err := cache.NewConfig(cache.Config{
    Driver: cache.DriverRedis,
    Prefix: "myapp",
    TTL:    30 * time.Minute,

    RedisAddr:     "127.0.0.1:6379",
    RedisPassword: "",
    RedisDB:       0,
})
```

---

## API

### Get / Put / Forever

```go
// Store a value with TTL
c.Put(ctx, "key", value, 5*time.Minute)

// Retrieve into a typed pointer — returns (found bool, err error)
var user User
found, err := c.Get(ctx, "key", &user)

// Store without expiry
c.Forever(ctx, "app.version", "v2.0.0")
```

### Remember

Returns the cached value if present; otherwise calls the function, stores the result, and returns it. The function is **only called once** even under concurrent load (stampede-safe).

```go
// Type-safe via generics
users, err := cache.Remember(ctx, c, "users.all", time.Hour, func() ([]User, error) {
    return db.FindAllUsers()
})

// Without expiry
config, err := cache.RememberForever(ctx, c, "app.config", func() (Config, error) {
    return loadConfigFromDB()
})
```

### Has / Forget / Flush

```go
c.Has(ctx, "key")          // bool
c.Forget(ctx, "key")       // remove one entry
c.Flush(ctx)               // remove all entries
```

### Pull

Retrieve and immediately remove in one operation — useful for one-time tokens.

```go
var otp string
found, err := c.Pull(ctx, "otp:12345", &otp)
// key is gone after this call
```

### Add

Store only if the key does not already exist. Returns `true` if stored.

```go
ok, err := c.Add(ctx, "lock:job", true, 30*time.Second)
if !ok {
    // another process already holds the lock
}
```

### Increment / Decrement

Atomic integer counters. Creates the key at `0` if it does not exist.

```go
views, _ := c.Increment(ctx, "page.views", 1)
stock, _ := c.Decrement(ctx, "product.stock", 1)

// Read back
n := c.GetInt(ctx, "page.views", 0)
```

### Typed helpers

```go
name := c.GetString(ctx, "app.name", "default")
```

---

## Cache Tags

Group related entries under one or more tags. Flushing a tag instantly invalidates every entry stored under it without touching the rest of the cache.

```go
// Write under a tag scope
c.Tags("users").Put(ctx, "1", user, time.Hour)
c.Tags("users").Put(ctx, "2", user, time.Hour)
c.Tags("posts").Put(ctx, "latest", posts, time.Hour)

// Flush only "users" — posts are untouched
c.Tags("users").Flush(ctx)

// Multi-tag entries require ALL tags to be valid
c.Tags("users", "posts").Put(ctx, "feed", feed, time.Hour)
c.Tags("users").Flush(ctx) // ← invalidates the feed entry too
```

### TagRemember

```go
users, err := cache.TagRemember(ctx, c.Tags("users"), "all", time.Hour, func() ([]User, error) {
    return db.FindAllUsers()
})

// After c.Tags("users").Flush(ctx), the next call re-fetches from the DB
```

```go
cfg, err := cache.TagRememberForever(ctx, c.Tags("config"), "settings", func() (Settings, error) {
    return loadSettings()
})
```

**How it works:** each tag holds a random version token in the store. Flushing a tag rotates its token — all previously stored keys become unreachable immediately. TTL entries expire naturally; forever entries are orphaned until the next full `Flush`.

---

## Stampede Prevention

`Remember` and `RememberForever` use [`singleflight`](https://pkg.go.dev/golang.org/x/sync/singleflight) internally. If many goroutines concurrently miss the same key, only **one** executes the callback — the rest wait and receive the same result.

```go
// 100 goroutines, cold cache — DB is called exactly once
for range 100 {
    go func() {
        cache.Remember(ctx, c, "heavy.query", time.Hour, func() (Result, error) {
            return db.ExpensiveQuery() // called once
        })
    }()
}
```

This applies automatically to `Remember`, `RememberForever`, `TagRemember`, and `TagRememberForever`.

---

## File Driver — Storage Layout

Files are organised in a two-level subdirectory structure based on the first two hex characters of the SHA-256 key hash:

```
/var/cache/myapp/
├── a3/
│   └── a3f9bc...json
├── b1/
│   └── b1d042...json
└── e1/
    └── e10a8b...json
```

Each file stores the serialised value, expiry time, and a forever flag.

---

## Sharing a Store

Multiple `Cache` instances can share the same underlying store — useful for applying different prefixes to the same backend:

```go
store := drivers.NewMemory()

users := cache.NewWithStore(store, cache.Config{Prefix: "users"})
posts := cache.NewWithStore(store, cache.Config{Prefix: "posts"})
```

---

## Custom Driver

Implement the `Store` interface to add your own backend:

```go
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
```

```go
c := cache.NewWithStore(myCustomStore, cache.Config{Prefix: "app"})

```

---

## Web frameworks

This library uses standard `context.Context` and works with any Go web framework. Pass the request context as usual:

- **Gin** — `c.Request.Context()`
- **Chi** — `r.Context()`
- **Fiber** — `c.UserContext()` ⚠️ not `c.Context()` (fasthttp context is not a `context.Context`)

---

## Testing

```bash
# Run all tests (Redis skipped if not available)
go test ./tests/...

# Verbose
go test ./tests/... -v

# Run with Redis
CACHE_REDIS_ADDR=127.0.0.1:6379 go test ./tests/...
```

---

## License

MIT © [Muhammad Hamizi Jaminan](mailto:hello@hamizi.net)
