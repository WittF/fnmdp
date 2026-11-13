package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
)

type Memory struct {
	mu       sync.Mutex
	projects map[int64]model.Project
	screens  map[screenKey]model.Screen
	skus     map[skuKey]model.SKU
	events   []model.Event
	coverage []model.PollCoverage
	nextID   int64
}

type screenKey struct {
	projectID int64
	screenID  int64
}

type skuKey struct {
	projectID int64
	screenID  int64
	skuID     int64
}

func NewMemory() *Memory {
	return &Memory{
		projects: map[int64]model.Project{},
		screens:  map[screenKey]model.Screen{},
		skus:     map[skuKey]model.SKU{},
		nextID:   1,
	}
}

func (m *Memory) EnsureProject(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[id]; !ok {
		m.projects[id] = model.Project{ID: id, Enabled: true, UpdatedAt: time.Now().UTC()}
	}
	return nil
}

func (m *Memory) UpsertProject(_ context.Context, p model.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := m.projects[p.ID]
	if p.Priority == 0 {
		p.Priority = existing.Priority
	}
	if p.TargetIntervalMs == 0 {
		p.TargetIntervalMs = existing.TargetIntervalMs
	}
	if p.ReplicaFactor == 0 {
		p.ReplicaFactor = existing.ReplicaFactor
	}
	p.UpdatedAt = nowIfZero(p.UpdatedAt)
	m.projects[p.ID] = p
	return nil
}

func (m *Memory) UpsertScreen(_ context.Context, s model.Screen) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.UpdatedAt = nowIfZero(s.UpdatedAt)
	m.screens[screenKey{s.ProjectID, s.ScreenID}] = s
	return nil
}

func (m *Memory) DeleteScreen(_ context.Context, projectID, screenID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.screens, screenKey{projectID, screenID})
	for key := range m.skus {
		if key.projectID == projectID && key.screenID == screenID {
			delete(m.skus, key)
		}
	}
	return nil
}

func (m *Memory) UpsertSKU(_ context.Context, s model.SKU) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.UpdatedAt = nowIfZero(s.UpdatedAt)
	m.skus[skuKey{s.ProjectID, s.ScreenID, s.SkuID}] = s
	return nil
}

func (m *Memory) DeleteSKU(_ context.Context, projectID, screenID, skuID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.skus, skuKey{projectID, screenID, skuID})
	return nil
}

func (m *Memory) InsertEvent(_ context.Context, e model.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.ID = m.nextID
	m.nextID++
	e.CreatedAt = nowIfZero(e.CreatedAt)
	m.events = append(m.events, e)
	return nil
}

func (m *Memory) UpsertCoverage(_ context.Context, c model.PollCoverage) error {
	if c.ProjectID <= 0 || c.StartTs <= 0 || c.EndTs <= 0 {
		return nil
	}
	if c.EndTs < c.StartTs {
		c.EndTs = c.StartTs
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.coverage {
		existing := &m.coverage[i]
		if existing.ProjectID != c.ProjectID || existing.StartTs != c.StartTs {
			continue
		}
		if c.EndTs > existing.EndTs {
			existing.EndTs = c.EndTs
		}
		return nil
	}
	m.coverage = append(m.coverage, c)
	return nil
}

func (m *Memory) Coverage(_ context.Context, projectID, sinceTs, untilTs int64) ([]model.PollCoverage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []model.PollCoverage{}
	for _, c := range m.coverage {
		if c.ProjectID != projectID {
			continue
		}
		if sinceTs > 0 && c.EndTs < sinceTs {
			continue
		}
		if untilTs > 0 && c.StartTs > untilTs {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartTs != out[j].StartTs {
			return out[i].StartTs < out[j].StartTs
		}
		return out[i].EndTs < out[j].EndTs
	})
	return out, nil
}

func (m *Memory) PruneBefore(_ context.Context, cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	events := m.events[:0]
	for _, e := range m.events {
		if e.CreatedAt.Unix() >= cutoffTs {
			events = append(events, e)
		}
	}
	m.events = events
	coverage := m.coverage[:0]
	for _, c := range m.coverage {
		if c.EndTs >= cutoffTs {
			coverage = append(coverage, c)
		}
	}
	m.coverage = coverage
	return nil
}

func (m *Memory) Snapshot(_ context.Context) (model.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := model.Snapshot{
		Projects: make([]model.Project, 0, len(m.projects)),
		Screens:  make([]model.Screen, 0, len(m.screens)),
		SKUs:     make([]model.SKU, 0, len(m.skus)),
	}
	for _, p := range m.projects {
		out.Projects = append(out.Projects, p)
	}
	for _, s := range m.screens {
		out.Screens = append(out.Screens, s)
	}
	for _, s := range m.skus {
		out.SKUs = append(out.SKUs, s)
	}
	applyLastEventTimes(&out, m.events)
	sort.Slice(out.Projects, func(i, j int) bool { return out.Projects[i].ID < out.Projects[j].ID })
	sort.Slice(out.Screens, func(i, j int) bool {
		if out.Screens[i].ProjectID != out.Screens[j].ProjectID {
			return out.Screens[i].ProjectID < out.Screens[j].ProjectID
		}
		return out.Screens[i].ScreenID < out.Screens[j].ScreenID
	})
	sort.Slice(out.SKUs, func(i, j int) bool {
		if out.SKUs[i].ProjectID != out.SKUs[j].ProjectID {
			return out.SKUs[i].ProjectID < out.SKUs[j].ProjectID
		}
		if out.SKUs[i].ScreenID != out.SKUs[j].ScreenID {
			return out.SKUs[i].ScreenID < out.SKUs[j].ScreenID
		}
		return out.SKUs[i].SkuID < out.SKUs[j].SkuID
	})
	return out, nil
}

func applyLastEventTimes(snap *model.Snapshot, events []model.Event) {
	projectTs := map[int64]int64{}
	screenTs := map[screenKey]int64{}
	skuTs := map[skuKey]int64{}
	for _, e := range events {
		if e.ProjectID <= 0 || e.Severity == "ops" {
			continue
		}
		ts := e.ServerTs
		if ts <= 0 && !e.CreatedAt.IsZero() {
			ts = e.CreatedAt.Unix()
		}
		if ts <= 0 {
			continue
		}
		if ts > projectTs[e.ProjectID] {
			projectTs[e.ProjectID] = ts
		}
		if e.ScreenID > 0 {
			sk := screenKey{projectID: e.ProjectID, screenID: e.ScreenID}
			if ts > screenTs[sk] {
				screenTs[sk] = ts
			}
		}
		if e.SkuID > 0 {
			k := skuKey{projectID: e.ProjectID, screenID: e.ScreenID, skuID: e.SkuID}
			if ts > skuTs[k] {
				skuTs[k] = ts
			}
		}
	}
	for i := range snap.Projects {
		if ts := projectTs[snap.Projects[i].ID]; ts > snap.Projects[i].LastEventTs {
			snap.Projects[i].LastEventTs = ts
		}
	}
	for i := range snap.Screens {
		k := screenKey{projectID: snap.Screens[i].ProjectID, screenID: snap.Screens[i].ScreenID}
		if ts := screenTs[k]; ts > snap.Screens[i].LastEventTs {
			snap.Screens[i].LastEventTs = ts
		}
	}
	for i := range snap.SKUs {
		k := skuKey{projectID: snap.SKUs[i].ProjectID, screenID: snap.SKUs[i].ScreenID, skuID: snap.SKUs[i].SkuID}
		if ts := skuTs[k]; ts > snap.SKUs[i].LastEventTs {
			snap.SKUs[i].LastEventTs = ts
		}
	}
}

func (m *Memory) Events(_ context.Context, f EventFilter) ([]model.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	limit := eventLimit(f)
	out := make([]model.Event, 0, min(limit, len(m.events)))
	for _, e := range m.events {
		if f.ProjectID > 0 && e.ProjectID != f.ProjectID {
			continue
		}
		if f.ScreenID > 0 && e.ScreenID != f.ScreenID {
			continue
		}
		if f.SkuID > 0 && e.SkuID != f.SkuID {
			continue
		}
		if f.EventType != "" && e.EventType != f.EventType {
			continue
		}
		if f.Severity != "" && e.Severity != f.Severity {
			continue
		}
		if f.SinceTs > 0 && e.CreatedAt.Unix() < f.SinceTs {
			continue
		}
		if f.UntilTs > 0 && e.CreatedAt.Unix() > f.UntilTs {
			continue
		}
		if !eventBeforeCursor(e, f.BeforeCreatedAt, f.BeforeID) {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Memory) Close() {}

func eventBeforeCursor(e model.Event, before time.Time, beforeID int64) bool {
	if before.IsZero() || beforeID <= 0 {
		return true
	}
	if e.CreatedAt.Before(before) {
		return true
	}
	return e.CreatedAt.Equal(before) && e.ID < beforeID
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
