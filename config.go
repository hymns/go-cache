package cache

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Driver string

const (
	DriverMemory Driver = "memory"
	DriverRedis  Driver = "redis"
	DriverFile   Driver = "file"
)

type Config struct {
	Driver Driver
	Prefix string
	TTL    time.Duration

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// File
	FilePath string
}

// ConfigFromEnv loads cache config from environment variables.
//
//	CACHE_DRIVER   = memory | redis | file  (default: memory)
//	CACHE_PREFIX   = key prefix             (default: "")
//	CACHE_TTL      = seconds                (default: 3600)
//
//	CACHE_REDIS_ADDR     = host:port        (default: 127.0.0.1:6379)
//	CACHE_REDIS_PASSWORD =                  (default: "")
//	CACHE_REDIS_DB       = 0               (default: 0)
//
//	CACHE_FILE_PATH = /path/to/cache  (default: OS cache dir + "/go-cache")
func ConfigFromEnv() Config {
	redisDB, _ := strconv.Atoi(env("CACHE_REDIS_DB", "0"))
	ttl, _ := strconv.Atoi(env("CACHE_TTL", "3600"))

	return Config{
		Driver:        Driver(env("CACHE_DRIVER", "memory")),
		Prefix:        env("CACHE_PREFIX", ""),
		TTL:           time.Duration(ttl) * time.Second,
		RedisAddr:     env("CACHE_REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: env("CACHE_REDIS_PASSWORD", ""),
		RedisDB:       redisDB,
		FilePath:      env("CACHE_FILE_PATH", defaultFilePath()),
	}
}

// defaultFilePath returns the OS-appropriate cache directory.
//   - Linux/other : $XDG_CACHE_HOME/go-cache  (fallback: $HOME/.cache/go-cache)
//   - macOS       : $HOME/Library/Caches/go-cache
//   - Windows     : %LOCALAPPDATA%\go-cache    (fallback: %TEMP%\go-cache)
func defaultFilePath() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "go-cache")
	}
	// last resort — should rarely happen
	return filepath.Join(os.TempDir(), "go-cache")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
