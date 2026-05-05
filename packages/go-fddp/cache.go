package fddp

import (
	"context"
	"strings"
	"sync"
	"time"
)

type CacheEntry struct {
	Value     any
	Fields    []string
	ExpiresAt time.Time
}

type Cache interface {
	Get(ctx context.Context, key string) (CacheEntry, bool)
	Set(ctx context.Context, key string, entry CacheEntry)
	Delete(ctx context.Context, key string)
	InvalidateFields(ctx context.Context, fields []string)
}

type MemoryCache struct {
	mu      sync.RWMutex
	records map[string]CacheEntry
	now     func() time.Time
}

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{
		records: make(map[string]CacheEntry),
		now:     time.Now,
	}
}

func (c *MemoryCache) Get(_ context.Context, key string) (CacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.records[key]
	c.mu.RUnlock()
	if !ok {
		return CacheEntry{}, false
	}

	if !entry.ExpiresAt.IsZero() && !entry.ExpiresAt.After(c.now()) {
		c.Delete(context.Background(), key)
		return CacheEntry{}, false
	}

	return entry, true
}

func (c *MemoryCache) Set(_ context.Context, key string, entry CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records[key] = entry
}

func (c *MemoryCache) Delete(_ context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.records, key)
}

func (c *MemoryCache) InvalidateFields(_ context.Context, fields []string) {
	if len(fields) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.records {
		if fieldsOverlap(fields, entry.Fields) {
			delete(c.records, key)
		}
	}
}

func fieldsOverlap(patterns []string, fields []string) bool {
	for _, pattern := range patterns {
		for _, field := range fields {
			if pattern == field {
				return true
			}
			if strings.HasSuffix(pattern, ".*") && strings.HasPrefix(field, strings.TrimSuffix(pattern, "*")) {
				return true
			}
		}
	}
	return false
}
