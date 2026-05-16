package drivers

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"time"
)

type memoryEntry struct {
	value   []byte
	expiry  time.Time
	forever bool
}

func (e memoryEntry) expired() bool {
	return !e.forever && time.Now().After(e.expiry)
}

// MemoryDriver stores cache entries in memory (non-persistent, process-scoped).
type MemoryDriver struct {
	mu   sync.RWMutex
	data map[string]memoryEntry
}

func NewMemory() *MemoryDriver {
	return &MemoryDriver{data: make(map[string]memoryEntry)}
}

func (m *MemoryDriver) Get(_ context.Context, key string) ([]byte, bool, error) {
	m.mu.RLock()
	entry, ok := m.data[key]
	m.mu.RUnlock()

	if !ok || entry.expired() {
		return nil, false, nil
	}
	return entry.value, true, nil
}

func (m *MemoryDriver) Put(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	m.data[key] = memoryEntry{value: value, expiry: time.Now().Add(ttl)}
	m.mu.Unlock()
	return nil
}

func (m *MemoryDriver) Forever(_ context.Context, key string, value []byte) error {
	m.mu.Lock()
	m.data[key] = memoryEntry{value: value, forever: true}
	m.mu.Unlock()
	return nil
}

func (m *MemoryDriver) Add(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.data[key]; ok && !entry.expired() {
		return false, nil
	}
	m.data[key] = memoryEntry{value: value, expiry: time.Now().Add(ttl)}
	return true, nil
}

func (m *MemoryDriver) Increment(_ context.Context, key string, amount int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var current int64
	if entry, ok := m.data[key]; ok && !entry.expired() {
		if len(entry.value) != 8 {
			return 0, fmt.Errorf("cache/memory: value at %q is not an integer", key)
		}
		current = int64(binary.BigEndian.Uint64(entry.value))
	}

	next := current + amount
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(next))
	m.data[key] = memoryEntry{value: b, forever: true}
	return next, nil
}

func (m *MemoryDriver) Decrement(ctx context.Context, key string, amount int64) (int64, error) {
	return m.Increment(ctx, key, -amount)
}

func (m *MemoryDriver) Forget(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}

func (m *MemoryDriver) Flush(_ context.Context) error {
	m.mu.Lock()
	m.data = make(map[string]memoryEntry)
	m.mu.Unlock()
	return nil
}

func (m *MemoryDriver) Has(ctx context.Context, key string) (bool, error) {
	_, found, err := m.Get(ctx, key)
	return found, err
}
