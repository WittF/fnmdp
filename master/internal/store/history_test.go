package store

import (
	"context"
	"testing"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
)

// TestEventsHistoryFilter covers the /api/history filters added to the memory
// store: per-SKU isolation plus the [since, until] created_at range.
func TestEventsHistoryFilter(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()

	base := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	// Three events for sku 1 at +0s/+60s/+120s, one for sku 2 at +60s.
	insert := func(skuID int64, offsetSec int) {
		if err := m.InsertEvent(ctx, model.Event{
			ProjectID: 100,
			ScreenID:  10,
			SkuID:     skuID,
			EventType: "sku_restock",
			Severity:  "critical",
			CreatedAt: base.Add(time.Duration(offsetSec) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	insert(1, 0)
	insert(1, 60)
	insert(2, 60)
	insert(1, 120)

	// SKU filter isolates a single SKU.
	got, err := m.Events(ctx, EventFilter{ProjectID: 100, SkuID: 1, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("sku=1 events = %d, want 3", len(got))
	}
	for _, e := range got {
		if e.SkuID != 1 {
			t.Fatalf("leaked sku_id %d", e.SkuID)
		}
	}

	// since/until window (inclusive) drops the +0s and +120s events.
	since := base.Add(30 * time.Second).Unix()
	until := base.Add(90 * time.Second).Unix()
	got, err = m.Events(ctx, EventFilter{ProjectID: 100, SkuID: 1, SinceTs: since, UntilTs: until, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("windowed sku=1 events = %d, want 1", len(got))
	}
	if got[0].CreatedAt.Unix() != base.Add(60*time.Second).Unix() {
		t.Fatalf("unexpected event at %v", got[0].CreatedAt)
	}

	// screen filter combined with project.
	got, err = m.Events(ctx, EventFilter{ProjectID: 100, ScreenID: 10, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("screen=10 events = %d, want 4", len(got))
	}
}

func TestEventsLimitCaps(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	base := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 250; i++ {
		if err := m.InsertEvent(ctx, model.Event{
			ProjectID: 100,
			ScreenID:  10,
			SkuID:     1,
			EventType: "sku_restock",
			Severity:  "critical",
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := m.Events(ctx, EventFilter{ProjectID: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != DefaultEventLimit {
		t.Fatalf("default limit = %d, want %d", len(got), DefaultEventLimit)
	}

	got, err = m.Events(ctx, EventFilter{ProjectID: 100, MaxLimit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 50 {
		t.Fatalf("low max limit = %d, want 50", len(got))
	}

	got, err = m.Events(ctx, EventFilter{ProjectID: 100, Limit: 250})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != MaxEventLimit {
		t.Fatalf("event cap = %d, want %d", len(got), MaxEventLimit)
	}

	got, err = m.Events(ctx, EventFilter{ProjectID: 100, Limit: 250, MaxLimit: MaxHistoryLimit})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 250 {
		t.Fatalf("history cap override = %d, want 250", len(got))
	}
}

func TestEventsCursorPaginationUsesCreatedAtAndID(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	base := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		if err := m.InsertEvent(ctx, model.Event{
			ProjectID: 100,
			ScreenID:  10,
			SkuID:     1,
			EventType: "sku_restock",
			Severity:  "critical",
			CreatedAt: base,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.InsertEvent(ctx, model.Event{
		ProjectID: 100,
		ScreenID:  10,
		SkuID:     1,
		EventType: "sku_restock",
		Severity:  "critical",
		CreatedAt: base.Add(-time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	first, err := m.Events(ctx, EventFilter{ProjectID: 100, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(first); len(first) != 3 || got[0] != 5 || got[1] != 4 || got[2] != 3 {
		t.Fatalf("first page ids = %v, want [5 4 3]", got)
	}

	last := first[len(first)-1]
	second, err := m.Events(ctx, EventFilter{
		ProjectID:       100,
		Limit:           3,
		BeforeCreatedAt: last.CreatedAt,
		BeforeID:        last.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(second); len(second) != 3 || got[0] != 2 || got[1] != 1 || got[2] != 6 {
		t.Fatalf("second page ids = %v, want [2 1 6]", got)
	}
}

func ids(events []model.Event) []int64 {
	out := make([]int64, 0, len(events))
	for _, e := range events {
		out = append(out, e.ID)
	}
	return out
}

func TestSnapshotLastEventTimesIgnoreOps(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.UpsertProject(ctx, model.Project{ID: 100, ServerTs: 2000}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpsertScreen(ctx, model.Screen{ProjectID: 100, ScreenID: 10, ServerTs: 2000}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpsertSKU(ctx, model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, ServerTs: 2000}); err != nil {
		t.Fatal(err)
	}
	if err := m.InsertEvent(ctx, model.Event{
		ProjectID: 100,
		ScreenID:  10,
		SkuID:     1,
		EventType: "sku_restock",
		Severity:  "critical",
		ServerTs:  2100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.InsertEvent(ctx, model.Event{
		ProjectID: 100,
		EventType: "risk_429",
		Severity:  "ops",
		ServerTs:  2200,
	}); err != nil {
		t.Fatal(err)
	}

	snap, err := m.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Projects[0].LastEventTs; got != 2100 {
		t.Fatalf("project last_event_ts = %d, want 2100", got)
	}
	if got := snap.Screens[0].LastEventTs; got != 2100 {
		t.Fatalf("screen last_event_ts = %d, want 2100", got)
	}
	if got := snap.SKUs[0].LastEventTs; got != 2100 {
		t.Fatalf("sku last_event_ts = %d, want 2100", got)
	}
}

func TestMemoryCoverageUpsertQueryAndPrune(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.UpsertCoverage(ctx, model.PollCoverage{ProjectID: 100, StartTs: 1000, EndTs: 1010}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpsertCoverage(ctx, model.PollCoverage{ProjectID: 100, StartTs: 1000, EndTs: 1020}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpsertCoverage(ctx, model.PollCoverage{ProjectID: 100, StartTs: 1100, EndTs: 1110}); err != nil {
		t.Fatal(err)
	}
	got, err := m.Coverage(ctx, 100, 1015, 1105)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("coverage rows = %d, want 2: %+v", len(got), got)
	}
	if got[0].StartTs != 1000 || got[0].EndTs != 1020 {
		t.Fatalf("upserted coverage = %+v, want end 1020", got[0])
	}
	if err := m.PruneBefore(ctx, 1050); err != nil {
		t.Fatal(err)
	}
	got, err = m.Coverage(ctx, 100, 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].StartTs != 1100 {
		t.Fatalf("coverage after prune = %+v, want only second segment", got)
	}
}

func TestMemoryPruneBeforeRemovesOldEvents(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	base := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if err := m.InsertEvent(ctx, model.Event{
			ProjectID: 100,
			ScreenID:  10,
			SkuID:     1,
			EventType: "sku_restock",
			Severity:  "critical",
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.PruneBefore(ctx, base.Add(90*time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	got, err := m.Events(ctx, EventFilter{ProjectID: 100, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].CreatedAt.Unix() != base.Add(2*time.Hour).Unix() {
		t.Fatalf("events after prune = %+v, want only newest", got)
	}
}
