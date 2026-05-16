package drivers

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type fileEntry struct {
	Value   []byte    `json:"v"`
	Expiry  time.Time `json:"e"`
	Forever bool      `json:"f"`
}

func (e fileEntry) expired() bool {
	return !e.Forever && time.Now().After(e.Expiry)
}

// FileDriver persists cache entries as JSON files on disk, organised in
// a two-level subdirectory structure: {root}/{first-2-hex}/{sha256}.json
type FileDriver struct {
	path string
}

func NewFile(path string) (*FileDriver, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("cache/file: mkdir %q: %w", path, err)
	}
	return &FileDriver{path: path}, nil
}

func (f *FileDriver) filename(key string) string {
	sum := sha256.Sum256([]byte(key))
	hash := fmt.Sprintf("%x", sum)
	return filepath.Join(f.path, hash[:2], hash+".json")
}

func (f *FileDriver) load(key string) (fileEntry, bool, error) {
	b, err := os.ReadFile(f.filename(key))
	if os.IsNotExist(err) {
		return fileEntry{}, false, nil
	}
	if err != nil {
		return fileEntry{}, false, err
	}
	var entry fileEntry
	return entry, true, json.Unmarshal(b, &entry)
}

func (f *FileDriver) save(key string, entry fileEntry) error {
	path := f.filename(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (f *FileDriver) Get(_ context.Context, key string) ([]byte, bool, error) {
	entry, found, err := f.load(key)
	if !found || err != nil {
		return nil, false, err
	}
	if entry.expired() {
		_ = os.Remove(f.filename(key))
		return nil, false, nil
	}
	return entry.Value, true, nil
}

func (f *FileDriver) Put(_ context.Context, key string, value []byte, ttl time.Duration) error {
	return f.save(key, fileEntry{Value: value, Expiry: time.Now().Add(ttl)})
}

func (f *FileDriver) Forever(_ context.Context, key string, value []byte) error {
	return f.save(key, fileEntry{Value: value, Forever: true})
}

func (f *FileDriver) Add(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	entry, found, err := f.load(key)
	if err != nil {
		return false, err
	}
	if found && !entry.expired() {
		return false, nil
	}
	return true, f.save(key, fileEntry{Value: value, Expiry: time.Now().Add(ttl)})
}

func (f *FileDriver) Increment(_ context.Context, key string, amount int64) (int64, error) {
	entry, found, err := f.load(key)
	if err != nil {
		return 0, err
	}

	var current int64
	if found && !entry.expired() {
		if len(entry.Value) != 8 {
			return 0, fmt.Errorf("cache/file: value at %q is not an integer", key)
		}
		current = int64(binary.BigEndian.Uint64(entry.Value))
	}

	next := current + amount
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(next))

	expiry := entry.Expiry
	forever := entry.Forever
	if !found || entry.expired() {
		forever = true
	}
	return next, f.save(key, fileEntry{Value: b, Expiry: expiry, Forever: forever})
}

func (f *FileDriver) Decrement(ctx context.Context, key string, amount int64) (int64, error) {
	return f.Increment(ctx, key, -amount)
}

func (f *FileDriver) Forget(_ context.Context, key string) error {
	err := os.Remove(f.filename(key))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (f *FileDriver) Flush(_ context.Context) error {
	entries, err := os.ReadDir(f.path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			_ = os.RemoveAll(filepath.Join(f.path, e.Name()))
		}
	}
	return nil
}

func (f *FileDriver) Has(ctx context.Context, key string) (bool, error) {
	_, found, err := f.Get(ctx, key)
	return found, err
}
