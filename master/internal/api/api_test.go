package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/store"
)

type testService struct {
	st *store.Memory
}

func (s testService) Snapshot() model.Snapshot { return model.Snapshot{} }

func (s testService) Events(f store.EventFilter) ([]model.Event, error) {
	return s.st.Events(context.Background(), f)
}

func (s testService) Trajectory(store.TrajectoryQuery) ([]model.TrajectoryPoint, error) {
	return nil, nil
}

func (s testService) Trajectories(q store.TrajectoryQuery) ([]model.TrajectorySeries, error) {
	return []model.TrajectorySeries{{
		ProjectID: q.ProjectID,
		ScreenID:  10,
		SkuID:     1,
		Points: []model.TrajectoryPoint{{
			T:         q.SinceMs,
			Flag:      2,
			Clickable: true,
			FlagText:  "在售",
			Observed:  true,
		}},
	}}, nil
}

func (s testService) ConfigureProject(int64, int, int, int, bool) error { return nil }

func (s testService) Rebalance() {}

func TestEventsPageUsesCursor(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemory()
	base := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		if err := st.InsertEvent(ctx, model.Event{
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
	h := Handler{Service: testService{st: st}}

	first := getEventsPage(t, h, "/api/events/page?project_id=100&limit=2")
	if len(first.Events) != 2 || first.Events[0].ID != 3 || first.Events[1].ID != 2 {
		t.Fatalf("first page = %+v, want ids [3 2]", first.Events)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page cursor = has_more:%v next:%q, want next cursor", first.HasMore, first.NextCursor)
	}

	second := getEventsPage(t, h, "/api/events/page?project_id=100&limit=2&cursor="+first.NextCursor)
	if len(second.Events) != 1 || second.Events[0].ID != 1 {
		t.Fatalf("second page = %+v, want id [1]", second.Events)
	}
	if second.HasMore || second.NextCursor != "" {
		t.Fatalf("second page cursor = has_more:%v next:%q, want end", second.HasMore, second.NextCursor)
	}
}

func TestEventsPageRejectsBadCursor(t *testing.T) {
	h := Handler{Service: testService{st: store.NewMemory()}}
	req := httptest.NewRequest(http.MethodGet, "/api/events/page?cursor=bad", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestTrajectoriesAllowsProjectBatch(t *testing.T) {
	h := Handler{Service: testService{st: store.NewMemory()}}
	req := httptest.NewRequest(http.MethodGet, "/api/trajectories?project_id=100&since_ms=1000&until_ms=2000&interval_ms=1000", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body %q", rr.Code, rr.Body.String())
	}
	var series []model.TrajectorySeries
	if err := json.NewDecoder(rr.Body).Decode(&series); err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].ProjectID != 100 || len(series[0].Points) != 1 {
		t.Fatalf("series = %+v", series)
	}
}

func TestTrajectoryStillRequiresSku(t *testing.T) {
	h := Handler{Service: testService{st: store.NewMemory()}}
	req := httptest.NewRequest(http.MethodGet, "/api/trajectory?project_id=100&since_ms=1000&until_ms=2000&interval_ms=1000", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func getEventsPage(t *testing.T, h Handler, target string) eventsPageResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("%s status = %d body %q", target, rr.Code, rr.Body.String())
	}
	var page eventsPageResponse
	if err := json.NewDecoder(rr.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	return page
}
