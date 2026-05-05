package fddp

import (
	"context"
	"fmt"
	"sync"
)

type BatchLoadFunc func(ctx context.Context, keys []string) (map[string]any, error)

type RequestBatcher struct {
	mu      sync.Mutex
	cache   map[string]map[string]any
	loaders map[string]BatchLoadFunc
	calls   TraceCalls
}

func NewRequestBatcher() *RequestBatcher {
	return &RequestBatcher{
		cache:   make(map[string]map[string]any),
		loaders: make(map[string]BatchLoadFunc),
	}
}

func (b *RequestBatcher) Register(group string, load BatchLoadFunc) {
	if b == nil || group == "" || load == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loaders[group] = load
}

func (b *RequestBatcher) LoadRegistered(ctx context.Context, group string, key string) (any, error) {
	values, err := b.LoadManyRegistered(ctx, group, []string{key})
	if err != nil {
		return nil, err
	}
	return values[key], nil
}

func (b *RequestBatcher) LoadManyRegistered(ctx context.Context, group string, keys []string) (map[string]any, error) {
	load, ok := b.loader(group)
	if !ok {
		return nil, fmt.Errorf("fddp: batch loader %q is not registered", group)
	}
	return b.LoadMany(ctx, group, keys, load)
}

func (b *RequestBatcher) Load(ctx context.Context, group string, key string, load BatchLoadFunc) (any, error) {
	values, err := b.LoadMany(ctx, group, []string{key}, load)
	if err != nil {
		return nil, err
	}
	return values[key], nil
}

func (b *RequestBatcher) LoadMany(ctx context.Context, group string, keys []string, load BatchLoadFunc) (map[string]any, error) {
	if b == nil {
		b = NewRequestBatcher()
	}

	b.mu.Lock()
	groupCache := b.cache[group]
	if groupCache == nil {
		groupCache = make(map[string]any)
		b.cache[group] = groupCache
	}

	missing := make([]string, 0, len(keys))
	seenMissing := make(map[string]struct{}, len(keys))
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := groupCache[key]; ok {
			result[key] = value
			continue
		}
		if _, seen := seenMissing[key]; seen {
			continue
		}
		seenMissing[key] = struct{}{}
		missing = append(missing, key)
	}
	b.mu.Unlock()

	if len(missing) == 0 {
		return result, nil
	}

	b.recordLoad(group, len(missing))

	loaded, err := load(ctx, missing)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	groupCache = b.cache[group]
	for key, value := range loaded {
		groupCache[key] = value
	}
	for _, key := range keys {
		if value, ok := groupCache[key]; ok {
			result[key] = value
		}
	}

	return result, nil
}

func (b *RequestBatcher) TraceCalls() TraceCalls {
	if b == nil {
		return TraceCalls{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneTraceCalls(b.calls)
}

func (b *RequestBatcher) recordLoad(group string, keys int) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls.BatchLoads++
	b.calls.BatchKeys += keys
	if b.calls.ByBatcher == nil {
		b.calls.ByBatcher = make(map[string]int)
	}
	b.calls.ByBatcher[group] += keys
}

func (b *RequestBatcher) loader(group string) (BatchLoadFunc, bool) {
	if b == nil {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	load, ok := b.loaders[group]
	return load, ok
}

func cloneTraceCalls(input TraceCalls) TraceCalls {
	out := input
	if len(input.ByBatcher) > 0 {
		out.ByBatcher = make(map[string]int, len(input.ByBatcher))
		for key, value := range input.ByBatcher {
			out.ByBatcher[key] = value
		}
	}
	return out
}
