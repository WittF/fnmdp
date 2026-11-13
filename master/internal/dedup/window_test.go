package dedup

import (
	"context"
	"testing"
	"time"
)

func TestMemoryWindowSeenOrMark(t *testing.T) {
	w := NewMemoryWindow()
	ctx := context.Background()
	if w.SeenOrMark(ctx, "k", 10*time.Millisecond) {
		t.Fatal("first mark should be unseen")
	}
	if !w.SeenOrMark(ctx, "k", 10*time.Millisecond) {
		t.Fatal("second mark should be seen")
	}
	time.Sleep(20 * time.Millisecond)
	if w.SeenOrMark(ctx, "k", 10*time.Millisecond) {
		t.Fatal("expired key should be unseen")
	}
}
