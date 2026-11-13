package state

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/WittF/fnmdp/master/internal/dedup"
	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/store"
	"github.com/WittF/fnmdp/shared/protocol"
)

type capturePublisher struct {
	mu   sync.Mutex
	msgs []any
}

func (p *capturePublisher) Broadcast(ctx context.Context, msg any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, msg)
}

type blockingSKUStore struct {
	store.Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingSKUStore) UpsertSKU(ctx context.Context, sku model.SKU) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return s.Store.UpsertSKU(ctx, sku)
	case <-ctx.Done():
		return ctx.Err()
	}
}

type blockingProjectStore struct {
	store.Store
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingProjectStore) UpsertProject(ctx context.Context, p model.Project) error {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-s.release:
		return s.Store.UpsertProject(ctx, p)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestEngineProcessPollReleasesLockBeforeStoreWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	st := &blockingSKUStore{
		Store:   store.NewMemory(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := engine.ProcessPoll(ctx, poll(100, 10, 1, 2, true, 1000))
		done <- err
	}()

	select {
	case <-st.entered:
	case <-ctx.Done():
		t.Fatal("ProcessPoll did not reach blocked store write")
	}

	snapDone := make(chan model.Snapshot, 1)
	go func() {
		snapDone <- engine.Snapshot(nil)
	}()
	select {
	case snap := <-snapDone:
		if len(snap.SKUs) != 1 || snap.SKUs[0].SkuID != 1 {
			t.Fatalf("snapshot = %+v", snap)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Snapshot blocked behind ProcessPoll store write")
	}

	close(st.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestEngineConfigureProjectReleasesLockBeforeStoreWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	st := &blockingProjectStore{
		Store:   store.NewMemory(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- engine.ConfigureProject(ctx, 100, 1, 10000, 2, true)
	}()

	select {
	case <-st.entered:
	case <-ctx.Done():
		t.Fatal("ConfigureProject did not reach blocked store write")
	}

	snapDone := make(chan model.Snapshot, 1)
	go func() {
		snapDone <- engine.Snapshot(nil)
	}()
	select {
	case snap := <-snapDone:
		if len(snap.Projects) != 1 || snap.Projects[0].ID != 100 {
			t.Fatalf("snapshot = %+v", snap)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Snapshot blocked behind ConfigureProject store write")
	}

	close(st.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestEngineProcessesPollAndDropsOld(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	pub := &capturePublisher{}
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), pub)
	if err != nil {
		t.Fatal(err)
	}

	first := poll(100, 10, 1, 4, false, 1000)
	if changed, err := engine.ProcessPoll(ctx, first); err != nil || !changed {
		t.Fatal(err)
	}
	second := poll(100, 10, 1, 2, true, 1001)
	second.Project.Hot = true
	if changed, err := engine.ProcessPoll(ctx, second); err != nil || !changed {
		t.Fatal(err)
	}
	old := poll(100, 10, 1, 1, false, 999)
	if changed, err := engine.ProcessPoll(ctx, old); err != nil || changed {
		t.Fatal(err)
	}

	snap, err := st.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.SKUs) != 1 || snap.SKUs[0].Flag != 2 || !snap.SKUs[0].Clickable {
		t.Fatalf("sku state = %+v", snap.SKUs)
	}
	events, err := st.Events(ctx, store.EventFilter{ProjectID: 100, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var restock bool
	for _, ev := range events {
		if ev.EventType == string(protocol.EventSkuRestock) {
			restock = true
		}
	}
	if !restock {
		t.Fatalf("events missing restock: %+v", events)
	}
	runtimeSnap := engine.Snapshot(nil)
	if len(runtimeSnap.Projects) != 1 || runtimeSnap.Projects[0].LastEventTs != 1001 {
		t.Fatalf("runtime project last_event_ts = %+v", runtimeSnap.Projects)
	}
	if len(runtimeSnap.SKUs) != 1 || runtimeSnap.SKUs[0].LastEventTs != 1001 {
		t.Fatalf("runtime sku last_event_ts = %+v", runtimeSnap.SKUs)
	}
}

func TestEngineProjectAndScreenEvents(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := poll(100, 10, 1, 4, false, 1000)
	msg.Project.Hot = false
	msg.Screens[0].Flag = 1
	msg.Screens[0].FlagText = "未开售"
	msg.Screens[0].Clickable = false
	if _, err := engine.ProcessPoll(ctx, msg); err != nil {
		t.Fatal(err)
	}
	msg.ServerTs = 1001
	msg.Project.ServerTs = 1001
	msg.Project.Hot = true
	msg.Screens = []protocol.ScreenDiff{{ScreenID: 10, Name: "Day 1", Flag: 2, FlagText: "在售", Clickable: true}}
	msg.Diff = nil
	if _, err := engine.ProcessPoll(ctx, msg); err != nil {
		t.Fatal(err)
	}
	events, err := st.Events(ctx, store.EventFilter{ProjectID: 100, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var hot, screenOnsale bool
	for _, ev := range events {
		hot = hot || ev.EventType == string(protocol.EventProjectHotOn)
		screenOnsale = screenOnsale || ev.EventType == string(protocol.EventScreenOnsale)
	}
	if !hot || !screenOnsale {
		t.Fatalf("events = %+v", events)
	}
}

func TestEngineDedupsScreenRemovedAcrossNodes(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Seed the screen as present.
	if _, err := engine.ProcessPoll(ctx, poll(100, 10, 1, 2, true, 1000)); err != nil {
		t.Fatal(err)
	}
	// Two different nodes each report the same screen removed (mirrors the real flood:
	// one upstream removal observed independently by ~60 nodes, each with a slightly
	// different server_ts). Only the first arrival should emit screen_removed.
	removeA := protocol.PollResult{
		Type: protocol.TypePollResult, Version: protocol.Version,
		NodeID: "node-a", ProjectID: 100, ServerTs: 1002,
		Screens: []protocol.ScreenDiff{{ScreenID: 10, Removed: true}},
	}
	removeB := removeA
	removeB.NodeID = "node-b"
	removeB.ServerTs = 1001 // earlier than A's, arriving second — must still be dropped
	if _, err := engine.ProcessPoll(ctx, removeA); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ProcessPoll(ctx, removeB); err != nil {
		t.Fatal(err)
	}
	events, err := st.Events(ctx, store.EventFilter{ProjectID: 100, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	removed := 0
	for _, ev := range events {
		if ev.EventType == string(protocol.EventScreenRemoved) {
			removed++
		}
	}
	if removed != 1 {
		t.Fatalf("screen_removed events = %d, want 1 (%+v)", removed, events)
	}
}

func TestEngineIgnoresTransientUnsupportedProjectFlag(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}

	first := poll(100, 10, 1, 1, false, 1000)
	first.Project.Flag = 1
	first.Project.FlagText = "未开售"
	first.Project.Hot = true
	first.Project.SalableLow = 38000
	first.Project.SalableHigh = 138000
	if _, err := engine.ProcessPoll(ctx, first); err != nil {
		t.Fatal(err)
	}

	transient := poll(100, 10, 1, 1, false, 1001)
	transient.Project.Flag = 5
	transient.Project.FlagText = "不可售"
	transient.Project.Hot = true
	transient.Project.SalableLow = 0
	transient.Project.SalableHigh = 0
	if changed, err := engine.ProcessPoll(ctx, transient); err != nil || changed {
		t.Fatalf("transient project flag changed=%v err=%v, want silent", changed, err)
	}

	recovered := poll(100, 10, 1, 1, false, 1002)
	recovered.Project.Flag = 1
	recovered.Project.FlagText = "未开售"
	recovered.Project.Hot = true
	recovered.Project.SalableLow = 38000
	recovered.Project.SalableHigh = 138000
	if _, err := engine.ProcessPoll(ctx, recovered); err != nil {
		t.Fatal(err)
	}

	snap, err := st.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Projects) != 1 || snap.Projects[0].Flag != 1 || snap.Projects[0].FlagText != "未开售" ||
		snap.Projects[0].SalableLow != 38000 || snap.Projects[0].SalableHigh != 138000 {
		t.Fatalf("project snapshot polluted by transient flag: %+v", snap.Projects)
	}
	events, err := st.Events(ctx, store.EventFilter{ProjectID: 100, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.EventType == string(protocol.EventProjectStatusChange) ||
			ev.EventType == string(protocol.EventProjectPriceBandChange) {
			t.Fatalf("transient project flag emitted event: %+v", ev)
		}
	}
}

func TestEnginePriceChangeKeepsOldPrice(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	pub := &capturePublisher{}
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), pub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ProcessPoll(ctx, poll(100, 10, 1, 2, true, 1000)); err != nil {
		t.Fatal(err)
	}
	msg := poll(100, 10, 1, 2, true, 1001)
	msg.Diff[0].New.PriceFen = 9900
	if _, err := engine.ProcessPoll(ctx, msg); err != nil {
		t.Fatal(err)
	}
	events, err := st.Events(ctx, store.EventFilter{ProjectID: 100, EventType: string(protocol.EventSkuPriceChange), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].OldPriceFen != 8800 || events[0].PriceFen != 9900 {
		t.Fatalf("price events = %+v", events)
	}
	var wire protocol.Event
	for _, msg := range pub.msgs {
		if ev, ok := msg.(protocol.Event); ok && ev.EventType == protocol.EventSkuPriceChange {
			wire = ev
		}
	}
	if wire.Old == nil || wire.New == nil || wire.Old.PriceFen != 8800 || wire.New.PriceFen != 9900 {
		t.Fatalf("wire event = %+v", wire)
	}
}

func TestEngineRateEventPublishesNodeDetails(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	pub := &capturePublisher{}
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), pub)
	if err != nil {
		t.Fatal(err)
	}

	engine.RateEvent(ctx, protocol.RateEvent{
		Type:         protocol.TypeRateEvent,
		Version:      protocol.Version,
		NodeID:       "zt1288536817",
		ProjectID:    100,
		StatusCode:   429,
		Kind:         protocol.RateSoftLimited,
		SecRequestID: "sec-1",
		Ts:           1234,
	})

	events, err := st.Events(ctx, store.EventFilter{ProjectID: 100, EventType: string(protocol.EventRisk429), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].NodeID != "zt1288536817" || events[0].Details["status_code"] != 429 {
		t.Fatalf("stored rate event = %+v", events)
	}
	var wire protocol.Event
	for _, msg := range pub.msgs {
		if ev, ok := msg.(protocol.Event); ok && ev.EventType == protocol.EventRisk429 {
			wire = ev
		}
	}
	if wire.NodeID != "zt1288536817" || wire.Details["status_code"] != 429 || wire.Details["sec_request_id"] != "sec-1" {
		t.Fatalf("wire rate event = %+v", wire)
	}
}

func TestEngineInventoryFlow(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	pub := &capturePublisher{}
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), pub)
	if err != nil {
		t.Fatal(err)
	}

	// First poll: ample (num == limit). New SKU → sku_added, no low-stock.
	first := poll(100, 10, 1, 2, true, 1000)
	first.Diff[0].LimitNum = 8
	first.Diff[0].New.Num = 8
	if _, err := engine.ProcessPoll(ctx, first); err != nil {
		t.Fatal(err)
	}

	// Second poll: drops below the cap (8→5) → sku_low_stock, num=exact remaining.
	second := poll(100, 10, 1, 2, true, 1001)
	second.Diff[0].LimitNum = 8
	second.Diff[0].New.Num = 5
	if _, err := engine.ProcessPoll(ctx, second); err != nil {
		t.Fatal(err)
	}

	// Snapshot (REST) carries num/limit_num.
	snap, err := st.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.SKUs) != 1 || snap.SKUs[0].Num != 5 || snap.SKUs[0].LimitNum != 8 {
		t.Fatalf("snapshot sku inventory = %+v", snap.SKUs)
	}

	// A low-stock event was recorded with old/new num.
	low, err := st.Events(ctx, store.EventFilter{ProjectID: 100, EventType: string(protocol.EventSkuLowStock), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(low) != 1 || low[0].OldNum != 8 || low[0].NewNum != 5 {
		t.Fatalf("low-stock event = %+v", low)
	}

	// The state_update + wire event carry num/limit through to the frontend.
	var su protocol.StateUpdate
	var ev protocol.Event
	for _, m := range pub.msgs {
		if u, ok := m.(protocol.StateUpdate); ok {
			su = u
		}
		if e, ok := m.(protocol.Event); ok && e.EventType == protocol.EventSkuLowStock {
			ev = e
		}
	}
	if su.Num != 5 || su.LimitNum != 8 {
		t.Fatalf("state_update inventory = num %d / limit %d", su.Num, su.LimitNum)
	}
	if ev.New == nil || ev.New.Num != 5 || ev.Old == nil || ev.Old.Num != 8 {
		t.Fatalf("wire event num old/new = %+v / %+v", ev.Old, ev.New)
	}
}

func TestEngineEmitsRepeatedSKUTransitionAfterInterveningStateInSameSecond(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// getV2's server_ts is second-resolution. At sale open the same SKU can
	// legitimately oscillate inside one second: soldout -> restock -> soldout.
	// Current state dedups duplicate reports already; a second-resolution L2 key
	// must not swallow the final transition, or trajectory history diverges from
	// sku_state and paints a long false-green span.
	if _, err := engine.ProcessPoll(ctx, poll(100, 10, 1, 4, false, 1000)); err != nil {
		t.Fatal(err)
	}
	for _, msg := range []protocol.PollResult{
		poll(100, 10, 1, 2, true, 1001),
		poll(100, 10, 1, 4, false, 1001),
		poll(100, 10, 1, 2, true, 1001),
		poll(100, 10, 1, 4, false, 1001),
	} {
		if _, err := engine.ProcessPoll(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}

	events, err := st.Events(ctx, store.EventFilter{
		ProjectID: 100,
		ScreenID:  10,
		SkuID:     1,
		EventType: string(protocol.EventSkuSoldout),
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("sku_soldout events = %d, want 2: %+v", len(events), events)
	}
	snap, err := st.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.SKUs) != 1 || snap.SKUs[0].Flag != 4 || snap.SKUs[0].Clickable {
		t.Fatalf("final sku state = %+v", snap.SKUs)
	}
}

func TestEngineFloorPollAdvancesLivenessWithoutEvents(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	pub := &capturePublisher{}
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), pub)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := engine.ProcessPoll(ctx, poll(100, 10, 1, 4, false, 1000)); err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(pub.msgs)

	// A floor/keepalive report: empty diff, no project/screen meta, fresher ServerTs.
	floor := protocol.PollResult{
		Type:      protocol.TypePollResult,
		Version:   protocol.Version,
		NodeID:    "node-1",
		ProjectID: 100,
		ServerTs:  1500,
		FetchedAt: 1500,
	}
	if changed, err := engine.ProcessPoll(ctx, floor); err != nil || changed {
		t.Fatalf("floor poll changed=%v err=%v, want no schedule change", changed, err)
	}

	// Liveness advanced...
	snap := engine.Snapshot(nil)
	if len(snap.Projects) != 1 || snap.Projects[0].LastPolledTs != 1500 {
		t.Fatalf("last_poll_ts = %+v, want 1500", snap.Projects)
	}
	// ...but no new events were emitted, and last_event_ts stays at the real change.
	for _, m := range pub.msgs[beforeEvents:] {
		if ev, ok := m.(protocol.Event); ok {
			t.Fatalf("floor poll emitted a spurious event: %+v", ev)
		}
	}
	if snap.Projects[0].LastEventTs == 1500 {
		t.Fatalf("floor poll must not bump last_event_ts: %+v", snap.Projects)
	}
}

func TestEnginePollLivenessPrefersFetchedAtOverServerTs(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}

	first := poll(100, 10, 1, 4, false, 1000)
	first.FetchedAt = 1000
	if _, err := engine.ProcessPoll(ctx, first); err != nil {
		t.Fatal(err)
	}

	floor := protocol.PollResult{
		Type:      protocol.TypePollResult,
		Version:   protocol.Version,
		NodeID:    "node-1",
		ProjectID: 100,
		ServerTs:  1500,
		FetchedAt: 1527,
	}
	if changed, err := engine.ProcessPoll(ctx, floor); err != nil || changed {
		t.Fatalf("floor poll changed=%v err=%v, want no schedule change", changed, err)
	}

	snap := engine.Snapshot(nil)
	if len(snap.Projects) != 1 || snap.Projects[0].LastPolledTs != 1527 {
		t.Fatalf("last_poll_ts = %+v, want fetched_at 1527", snap.Projects)
	}
}

func TestEngineCoverageFlushAndGapSegmentation(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	engine.SetCoverageGap(5 * time.Minute)

	first := poll(100, 10, 1, 4, false, 1000)
	first.FetchedAt = 1000
	if _, err := engine.ProcessPoll(ctx, first); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Coverage(ctx, 100, 0, 2000); err != nil || len(got) != 1 || got[0].StartTs != 1000 || got[0].EndTs != 1000 {
		t.Fatalf("initial coverage = %+v err=%v", got, err)
	}

	floor := protocol.PollResult{
		Type:      protocol.TypePollResult,
		Version:   protocol.Version,
		NodeID:    "node-1",
		ProjectID: 100,
		ServerTs:  1090,
		FetchedAt: 1090,
	}
	if _, err := engine.ProcessPoll(ctx, floor); err != nil {
		t.Fatal(err)
	}
	if err := engine.FlushCoverage(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := st.Coverage(ctx, 100, 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].StartTs != 1000 || got[0].EndTs != 1090 {
		t.Fatalf("extended coverage = %+v, want 1000-1090", got)
	}

	gapped := floor
	gapped.ServerTs = 1500
	gapped.FetchedAt = 1500
	if _, err := engine.ProcessPoll(ctx, gapped); err != nil {
		t.Fatal(err)
	}
	got, err = st.Coverage(ctx, 100, 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("segmented coverage rows = %+v, want 2", got)
	}
	if got[0].StartTs != 1000 || got[0].EndTs != 1090 || got[1].StartTs != 1500 || got[1].EndTs != 1500 {
		t.Fatalf("segmented coverage = %+v", got)
	}
}

func poll(projectID, screenID, skuID int64, flag int, clickable bool, serverTs int64) protocol.PollResult {
	return protocol.PollResult{
		Type:      protocol.TypePollResult,
		Version:   protocol.Version,
		NodeID:    "node-1",
		ProjectID: projectID,
		ServerTs:  serverTs,
		Project: &protocol.ProjectMeta{
			Name:     "Project",
			Flag:     2,
			FlagText: "在售",
			ServerTs: serverTs,
		},
		Screens: []protocol.ScreenDiff{{
			ScreenID:  screenID,
			Name:      "Day 1",
			Flag:      2,
			FlagText:  "在售",
			Clickable: true,
		}},
		Diff: []protocol.SkuDiff{{
			ScreenID: screenID,
			SkuID:    skuID,
			Desc:     "VIP",
			FlagText: "state",
			New: &protocol.Fingerprint{
				Flag:      flag,
				Clickable: clickable,
				PriceFen:  8800,
			},
		}},
	}
}
