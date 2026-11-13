package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/store"
)

type Service interface {
	Snapshot() model.Snapshot
	Events(store.EventFilter) ([]model.Event, error)
	Trajectory(store.TrajectoryQuery) ([]model.TrajectoryPoint, error)
	Trajectories(store.TrajectoryQuery) ([]model.TrajectorySeries, error)
	ConfigureProject(id int64, priority, intervalMs, replica int, enabled bool) error
	Rebalance()
}

type Handler struct {
	AdminToken string
	Service    Service
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case r.Method == http.MethodGet && path == "/snapshot":
		writeJSON(w, h.Service.Snapshot())
	case r.Method == http.MethodGet && path == "/projects":
		writeJSON(w, h.Service.Snapshot().Projects)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/projects/"):
		h.project(w, r, strings.TrimPrefix(path, "/projects/"))
	case r.Method == http.MethodGet && path == "/events/page":
		h.eventsPage(w, r)
	case r.Method == http.MethodGet && path == "/events":
		h.events(w, r)
	case r.Method == http.MethodGet && path == "/history":
		h.history(w, r)
	case r.Method == http.MethodGet && path == "/trajectories":
		h.trajectories(w, r)
	case r.Method == http.MethodGet && path == "/trajectory":
		h.trajectory(w, r)
	case r.Method == http.MethodGet && path == "/nodes":
		writeJSON(w, h.Service.Snapshot().Nodes)
	case strings.HasPrefix(path, "/admin/projects"):
		h.adminProjects(w, r, path)
	default:
		http.NotFound(w, r)
	}
}

func (h Handler) trajectory(w http.ResponseWriter, r *http.Request) {
	q, ok := parseTrajectoryQuery(w, r, true)
	if !ok {
		return
	}
	points, err := h.Service.Trajectory(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, points)
}

func (h Handler) trajectories(w http.ResponseWriter, r *http.Request) {
	q, ok := parseTrajectoryQuery(w, r, false)
	if !ok {
		return
	}
	series, err := h.Service.Trajectories(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, series)
}

func parseTrajectoryQuery(w http.ResponseWriter, r *http.Request, requireSKU bool) (store.TrajectoryQuery, bool) {
	q := r.URL.Query()
	projectID, _ := strconv.ParseInt(q.Get("project_id"), 10, 64)
	screenID, _ := strconv.ParseInt(q.Get("screen_id"), 10, 64)
	skuID, _ := strconv.ParseInt(q.Get("sku_id"), 10, 64)
	sinceMs := parseMillis(q.Get("since_ms"), q.Get("since"))
	untilMs := parseMillis(q.Get("until_ms"), q.Get("until"))
	if untilMs <= 0 {
		untilMs = time.Now().UnixMilli()
	}
	intervalMs, _ := strconv.ParseInt(q.Get("interval_ms"), 10, 64)
	if projectID <= 0 || (requireSKU && (screenID <= 0 || skuID <= 0)) || sinceMs <= 0 || untilMs < sinceMs || intervalMs <= 0 ||
		((untilMs-sinceMs)/intervalMs)+1 > store.MaxTrajectoryPoints {
		http.Error(w, "bad trajectory query", http.StatusBadRequest)
		return store.TrajectoryQuery{}, false
	}
	return store.TrajectoryQuery{
		ProjectID:  projectID,
		ScreenID:   screenID,
		SkuID:      skuID,
		SinceMs:    sinceMs,
		UntilMs:    untilMs,
		IntervalMs: intervalMs,
	}, true
}

func parseMillis(msText, secText string) int64 {
	if msText != "" {
		n, _ := strconv.ParseInt(msText, 10, 64)
		return n
	}
	n, _ := strconv.ParseInt(secText, 10, 64)
	if n <= 0 {
		return 0
	}
	return n * 1000
}

func (h Handler) project(w http.ResponseWriter, r *http.Request, idText string) {
	id, err := strconv.ParseInt(strings.Trim(idText, "/"), 10, 64)
	if err != nil {
		http.Error(w, "bad project id", http.StatusBadRequest)
		return
	}
	snap := h.Service.Snapshot()
	for _, p := range snap.Projects {
		if p.ID == id {
			resp := struct {
				Project model.Project  `json:"project"`
				Screens []model.Screen `json:"screens"`
				SKUs    []model.SKU    `json:"skus"`
			}{Project: p}
			for _, s := range snap.Screens {
				if s.ProjectID == id {
					resp.Screens = append(resp.Screens, s)
				}
			}
			for _, s := range snap.SKUs {
				if s.ProjectID == id {
					resp.SKUs = append(resp.SKUs, s)
				}
			}
			writeJSON(w, resp)
			return
		}
	}
	http.NotFound(w, r)
}

func (h Handler) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID, _ := strconv.ParseInt(q.Get("project_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	events, err := h.Service.Events(store.EventFilter{
		ProjectID: projectID,
		EventType: q.Get("event_type"),
		Severity:  q.Get("severity"),
		Limit:     limit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, events)
}

type eventsPageResponse struct {
	Events     []model.Event `json:"events"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func (h Handler) eventsPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID, _ := strconv.ParseInt(q.Get("project_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	limit = eventPageLimit(limit)
	beforeCreatedAt, beforeID, err := parseEventCursor(q.Get("cursor"))
	if err != nil {
		http.Error(w, "bad event cursor", http.StatusBadRequest)
		return
	}
	events, err := h.Service.Events(store.EventFilter{
		ProjectID:       projectID,
		EventType:       q.Get("event_type"),
		Severity:        q.Get("severity"),
		BeforeCreatedAt: beforeCreatedAt,
		BeforeID:        beforeID,
		Limit:           limit + 1,
		MaxLimit:        limit + 1,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := eventsPageResponse{Events: events}
	if len(events) > limit {
		resp.HasMore = true
		resp.Events = events[:limit]
		last := resp.Events[len(resp.Events)-1]
		resp.NextCursor = encodeEventCursor(last)
	}
	if resp.Events == nil {
		resp.Events = []model.Event{}
	}
	writeJSON(w, resp)
}

func eventPageLimit(limit int) int {
	if limit <= 0 {
		return store.DefaultEventLimit
	}
	if limit > store.MaxEventLimit {
		return store.MaxEventLimit
	}
	return limit
}

func encodeEventCursor(e model.Event) string {
	raw := fmt.Sprintf("%d:%d", e.CreatedAt.UTC().UnixNano(), e.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func parseEventCursor(cursor string) (time.Time, int64, error) {
	if cursor == "" {
		return time.Time{}, 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, 0, err
	}
	tsText, idText, ok := strings.Cut(string(raw), ":")
	if !ok {
		return time.Time{}, 0, fmt.Errorf("missing cursor separator")
	}
	ts, err := strconv.ParseInt(tsText, 10, 64)
	if err != nil || ts <= 0 {
		return time.Time{}, 0, fmt.Errorf("bad cursor timestamp")
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return time.Time{}, 0, fmt.Errorf("bad cursor id")
	}
	return time.Unix(0, ts).UTC(), id, nil
}

// history returns the state-change events for a given SKU (or screen/project)
// within an optional [since, until] time range, so the frontend can rebuild a
// per-SKU status timeline. Reuses the state_events store query.
func (h Handler) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID, _ := strconv.ParseInt(q.Get("project_id"), 10, 64)
	screenID, _ := strconv.ParseInt(q.Get("screen_id"), 10, 64)
	skuID, _ := strconv.ParseInt(q.Get("sku_id"), 10, 64)
	since, _ := strconv.ParseInt(q.Get("since"), 10, 64)
	until, _ := strconv.ParseInt(q.Get("until"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	events, err := h.Service.Events(store.EventFilter{
		ProjectID: projectID,
		ScreenID:  screenID,
		SkuID:     skuID,
		SinceTs:   since,
		UntilTs:   until,
		Limit:     limit,
		MaxLimit:  store.MaxHistoryLimit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, events)
}

func (h Handler) adminProjects(w http.ResponseWriter, r *http.Request, path string) {
	if h.AdminToken != "" && r.Header.Get("Authorization") != "Bearer "+h.AdminToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodPost && path == "/admin/projects":
		var req projectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ProjectID <= 0 {
			http.Error(w, "bad project payload", http.StatusBadRequest)
			return
		}
		if err := h.Service.ConfigureProject(req.ProjectID, req.Priority, req.TargetIntervalMs, req.ReplicaFactor, true); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.Service.Rebalance()
		w.WriteHeader(http.StatusCreated)
	case (r.Method == http.MethodPut || r.Method == http.MethodDelete) && strings.HasPrefix(path, "/admin/projects/"):
		id, err := strconv.ParseInt(strings.TrimPrefix(path, "/admin/projects/"), 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "bad project id", http.StatusBadRequest)
			return
		}
		if r.Method == http.MethodDelete {
			if err := h.Service.ConfigureProject(id, 0, 0, 0, false); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			h.Service.Rebalance()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		var req projectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad project payload", http.StatusBadRequest)
			return
		}
		if err := h.Service.ConfigureProject(id, req.Priority, req.TargetIntervalMs, req.ReplicaFactor, true); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.Service.Rebalance()
		writeJSON(w, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

type projectRequest struct {
	ProjectID        int64 `json:"bili_project_id"`
	Priority         int   `json:"priority"`
	TargetIntervalMs int   `json:"target_interval_ms"`
	ReplicaFactor    int   `json:"replica_factor"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
