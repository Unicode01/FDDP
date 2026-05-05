package fddp

import (
	"context"
	"sync"
)

type IdempotencyStore interface {
	Get(ctx context.Context, key string) (CommandResponse, bool)
	Set(ctx context.Context, key string, response CommandResponse)
}

type MemoryIdempotencyStore struct {
	mu      sync.RWMutex
	records map[string]CommandResponse
}

func NewMemoryIdempotencyStore() *MemoryIdempotencyStore {
	return &MemoryIdempotencyStore{records: make(map[string]CommandResponse)}
}

func (s *MemoryIdempotencyStore) Get(_ context.Context, key string) (CommandResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	response, ok := s.records[key]
	return response, ok
}

func (s *MemoryIdempotencyStore) Set(_ context.Context, key string, response CommandResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[key] = response
}
