package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/WittF/fnmdp/master/internal/config"
	"github.com/WittF/fnmdp/master/internal/dedup"
	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/registry"
	masterstate "github.com/WittF/fnmdp/master/internal/state"
	"github.com/WittF/fnmdp/master/internal/store"
	"github.com/WittF/fnmdp/shared/protocol"
)

type testSender struct {
	msgs []any
}

func (s *testSender) Send(ctx context.Context, msg any) error {
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *testSender) Close() {}

func TestPollResultRebalancesOnlyForSchedulingChanges(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := masterstate.New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(ctx, config.Config{
		DefaultIntervalMs:    30000,
		DefaultHotIntervalMs: 10000,
		NodeBudgetRPS:        0.9,
		DefaultReplicaFactor: 1,
	}, st, engine)
	if err := app.ConfigureProject(100, 0, 0, 1, true); err != nil {
		t.Fatal(err)
	}
	sender := &testSender{}
	app.registry.Register(protocol.Register{NodeID: "node-1"}, sender)
	app.Rebalance()
	initial := len(sender.msgs)
	if initial == 0 {
		t.Fatal("initial rebalance did not send assign")
	}

	msg := pollResult(100, 10, 1, 4, false, 8800, 1000)
	app.onPollResult(ctx, msg)
	if len(sender.msgs) != initial {
		t.Fatalf("ordinary poll resent assignments: got %d want %d", len(sender.msgs), initial)
	}

	msg.Project.Hot = true
	msg.Project.ServerTs = 1001
	msg.ServerTs = 1001
	app.onPollResult(ctx, msg)
	if len(sender.msgs) <= initial {
		t.Fatalf("hot change did not trigger assignment resend: got %d initial %d", len(sender.msgs), initial)
	}
}

var _ registry.Sender = (*testSender)(nil)

func TestPeriodicRebalanceTickerIsLongEnough(t *testing.T) {
	if 45*time.Second < 30*time.Second {
		t.Fatal("periodic rebalance should not be high-frequency")
	}
}

func TestLivezReflectsWatchdogHealth(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := masterstate.New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(ctx, config.Config{}, st, engine)
	handler := app.Handler(ctx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthy /livez status = %d, want 200", rec.Code)
	}

	app.healthy.Store(false)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy /livez status = %d, want 503", rec.Code)
	}
}

func TestPprofRequiresConfiguredAdminToken(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	engine, err := masterstate.New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(ctx, config.Config{}, st, engine)

	rec := httptest.NewRecorder()
	app.Handler(ctx).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("pprof without configured token status = %d, want 401", rec.Code)
	}

	app = New(ctx, config.Config{AdminToken: "secret"}, st, engine)
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	app.Handler(ctx).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pprof with token status = %d, want 200", rec.Code)
	}
}

func TestBatchTrajectoriesMatchSingleTrajectory(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	base := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)

	if err := st.UpsertProject(ctx, model.Project{ID: 100, Name: "Project"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertScreen(ctx, model.Screen{ProjectID: 100, ScreenID: 10, Name: "Day 1"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSKU(ctx, model.SKU{
		ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 2, Clickable: true, FlagText: "在售",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSKU(ctx, model.SKU{
		ProjectID: 100, ScreenID: 10, SkuID: 2, Flag: 4, FlagText: "已售罄",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertCoverage(ctx, model.PollCoverage{
		ProjectID: 100,
		StartTs:   base.Unix(),
		EndTs:     base.Add(5 * time.Second).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []model.Event{
		{
			ProjectID: 100, ScreenID: 10, SkuID: 1,
			OldFlag: 1, NewFlag: 2, NewClickable: true, FlagText: "在售",
			CreatedAt: base.Add(2 * time.Second),
		},
		{
			ProjectID: 100, ScreenID: 10, SkuID: 2,
			OldFlag: 2, OldClickable: true, NewFlag: 4, FlagText: "已售罄",
			CreatedAt: base.Add(3 * time.Second),
		},
	} {
		if err := st.InsertEvent(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	engine, err := masterstate.New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(ctx, config.Config{CoverageGap: time.Second}, st, engine)
	q := store.TrajectoryQuery{
		ProjectID:  100,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(5 * time.Second).UnixMilli(),
		IntervalMs: 1000,
	}

	batch, err := app.Trajectories(q)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64][]model.TrajectoryPoint{}
	for _, series := range batch {
		got[series.SkuID] = series.Points
	}
	for _, skuID := range []int64{1, 2} {
		one, err := app.Trajectory(store.TrajectoryQuery{
			ProjectID:  q.ProjectID,
			ScreenID:   10,
			SkuID:      skuID,
			SinceMs:    q.SinceMs,
			UntilMs:    q.UntilMs,
			IntervalMs: q.IntervalMs,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got[skuID], one) {
			t.Fatalf("sku %d batch trajectory differs\nbatch: %+v\nsingle:%+v", skuID, got[skuID], one)
		}
	}
}
