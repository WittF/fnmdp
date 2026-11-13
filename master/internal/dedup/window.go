package dedup

import (
	"context"
	"sync"
	"time"
)

type Window interface {
	SeenOrMark(context.Context, string, time.Duration) bool
}

type MemoryWindow struct {
	mu    sync.Mutex
	items map[string]time.Time
}

func NewMemoryWindow() *MemoryWindow {
	return &MemoryWindow{items: map[string]time.Time{}}
}

func (w *MemoryWindow) SeenOrMark(_ context.Context, key string, ttl time.Duration) bool {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	now := time.Now()
	expires := now.Add(ttl)
	w.mu.Lock()
	defer w.mu.Unlock()
	for k, exp := range w.items {
		if now.After(exp) {
			delete(w.items, k)
		}
	}
	if exp, ok := w.items[key]; ok && now.Before(exp) {
		return true
	}
	w.items[key] = expires
	return false
}
