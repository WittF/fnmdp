package state

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/WittF/fnmdp/master/internal/dedup"
	"github.com/WittF/fnmdp/master/internal/events"
	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/store"
	"github.com/WittF/fnmdp/shared/protocol"
)

const defaultCoverageGap = 5 * time.Minute

type Publisher interface {
	Broadcast(context.Context, any)
}

type Engine struct {
	mu    sync.Mutex
	store store.Store
	pub   Publisher

	projects map[int64]model.Project
	screens  map[screenKey]model.Screen
	skus     map[skuKey]model.SKU

	coverage    map[int64]model.PollCoverage
	coverageGap time.Duration
}

type storeOp struct {
	desc string
	fn   func(context.Context) error
}

type sideEffects struct {
	storeOps []storeOp
	msgs     []any
	pub      Publisher
}

func (se *sideEffects) store(desc string, fn func(context.Context) error) {
	if fn == nil {
		return
	}
	se.storeOps = append(se.storeOps, storeOp{desc: desc, fn: fn})
}

func (se *sideEffects) broadcast(msg any) {
	se.msgs = append(se.msgs, msg)
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

func New(ctx context.Context, st store.Store, _ dedup.Window, pub Publisher) (*Engine, error) {
	e := &Engine{
		store:       st,
		pub:         pub,
		projects:    map[int64]model.Project{},
		screens:     map[screenKey]model.Screen{},
		skus:        map[skuKey]model.SKU{},
		coverage:    map[int64]model.PollCoverage{},
		coverageGap: defaultCoverageGap,
	}
	if st == nil {
		return e, nil
	}
	snap, err := st.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range snap.Projects {
		e.projects[p.ID] = p
	}
	for _, s := range snap.Screens {
		e.screens[screenKey{s.ProjectID, s.ScreenID}] = s
	}
	for _, s := range snap.SKUs {
		e.skus[skuKey{s.ProjectID, s.ScreenID, s.SkuID}] = s
	}
	return e, nil
}

func (e *Engine) SetPublisher(pub Publisher) {
	e.mu.Lock()
	e.pub = pub
	e.mu.Unlock()
}

func (e *Engine) SetCoverageGap(gap time.Duration) {
	if gap <= 0 {
		gap = defaultCoverageGap
	}
	e.mu.Lock()
	e.coverageGap = gap
	e.mu.Unlock()
}

func (e *Engine) Ping() {
	e.mu.Lock()
	e.mu.Unlock()
}

func (e *Engine) UpsertProject(ctx context.Context, p model.Project) error {
	e.mu.Lock()
	st := e.store
	p.UpdatedAt = time.Now().UTC()
	e.projects[p.ID] = p
	e.mu.Unlock()
	if st != nil {
		return st.UpsertProject(ctx, p)
	}
	return nil
}

func (e *Engine) ConfigureProject(ctx context.Context, id int64, priority, intervalMs, replica int, enabled bool) error {
	e.mu.Lock()
	st := e.store
	p := e.projects[id]
	p.ID = id
	p.Priority = priority
	p.TargetIntervalMs = intervalMs
	p.ReplicaFactor = replica
	p.Enabled = enabled
	p.UpdatedAt = time.Now().UTC()
	e.projects[id] = p
	e.mu.Unlock()
	if st != nil {
		return st.UpsertProject(ctx, p)
	}
	return nil
}

func (e *Engine) EnsureProject(ctx context.Context, id int64) error {
	if id <= 0 {
		return nil
	}
	e.mu.Lock()
	st := e.store
	if _, ok := e.projects[id]; !ok {
		e.projects[id] = model.Project{ID: id, Enabled: true, UpdatedAt: time.Now().UTC()}
	}
	e.mu.Unlock()
	if st != nil {
		return st.EnsureProject(ctx, id)
	}
	return nil
}

// ProcessPoll applies the authoritative in-memory state transition while holding
// e.mu, then flushes store writes and frontend broadcasts after releasing it.
// Store errors no longer truncate the in-memory transition; all deferred store
// operations are attempted and the first error is returned.
func (e *Engine) ProcessPoll(ctx context.Context, pr protocol.PollResult) (bool, error) {
	var se sideEffects
	e.mu.Lock()
	st := e.store
	se.pub = e.pub
	scheduleChanged := false
	if pr.Project != nil {
		changed := e.applyProject(st, pr.ProjectID, *pr.Project, &se)
		scheduleChanged = scheduleChanged || changed
	}
	for _, screen := range pr.Screens {
		e.applyScreen(st, pr, screen, &se)
	}
	for _, diff := range pr.Diff {
		e.applySKU(st, pr, diff, &se)
	}
	// Advance per-project liveness on EVERY poll — changed OR a floor/keepalive poll
	// with an empty diff. This is what lets the frontend distinguish "state stable,
	// still being observed" (timeline forward-fills) from "collection stalled"
	// (timeline shows no-data). In-memory only (no PG column) → no write amplification.
	polledTs := e.touchPolled(pr.ProjectID, pr.ServerTs, pr.FetchedAt)
	e.touchCoverage(st, pr.ProjectID, polledTs, &se)
	e.mu.Unlock()
	return scheduleChanged, flush(ctx, &se)
}

// touchPolled monotonically advances a known project's LastPolledTs. It uses the
// fetch completion clock for liveness; ServerTs is the upstream business clock
// (getV2 current_time) and can lag the collector/master clock by tens of seconds.
// It only touches projects that already exist (a floor report always follows real
// polls that seeded the entry), so it never conjures a phantom project into the
// snapshot.
func (e *Engine) touchPolled(projectID, serverTs, fetchedAt int64) int64 {
	if projectID <= 0 {
		return 0
	}
	ts := fetchedAt
	if ts <= 0 {
		ts = serverTs
	}
	p, ok := e.projects[projectID]
	if !ok || ts <= p.LastPolledTs {
		return 0
	}
	p.LastPolledTs = ts
	e.projects[projectID] = p
	return ts
}

func (e *Engine) touchCoverage(st store.Store, projectID, ts int64, se *sideEffects) {
	if st == nil || projectID <= 0 || ts <= 0 {
		return
	}
	gap := e.coverageGap
	if gap <= 0 {
		gap = defaultCoverageGap
	}
	cur, ok := e.coverage[projectID]
	if ok {
		if ts <= cur.EndTs {
			return
		}
		if time.Duration(ts-cur.EndTs)*time.Second <= gap {
			cur.EndTs = ts
			e.coverage[projectID] = cur
			return
		}
		segment := cur
		se.store("upsert coverage", func(ctx context.Context) error {
			return st.UpsertCoverage(ctx, segment)
		})
	}
	next := model.PollCoverage{ProjectID: projectID, StartTs: ts, EndTs: ts}
	e.coverage[projectID] = next
	se.store("upsert coverage", func(ctx context.Context) error {
		return st.UpsertCoverage(ctx, next)
	})
}

func (e *Engine) FlushCoverage(ctx context.Context) error {
	e.mu.Lock()
	st := e.store
	segments := make([]model.PollCoverage, 0, len(e.coverage))
	for _, c := range e.coverage {
		segments = append(segments, c)
	}
	e.mu.Unlock()
	if st == nil {
		return nil
	}
	for _, c := range segments {
		if err := st.UpsertCoverage(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) CoverageSnapshot() []model.PollCoverage {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.PollCoverage, 0, len(e.coverage))
	for _, c := range e.coverage {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ProjectID != out[j].ProjectID {
			return out[i].ProjectID < out[j].ProjectID
		}
		if out[i].StartTs != out[j].StartTs {
			return out[i].StartTs < out[j].StartTs
		}
		return out[i].EndTs < out[j].EndTs
	})
	return out
}

func (e *Engine) RateEvent(ctx context.Context, msg protocol.RateEvent) {
	eventType := protocol.EventRisk429
	if msg.Kind == protocol.RateHardBlocked {
		eventType = protocol.EventRisk412
	}
	e.emit(ctx, model.Event{
		ProjectID: msg.ProjectID,
		EventType: string(eventType),
		Severity:  string(protocol.SeverityOps),
		ServerTs:  msg.Ts,
		NodeID:    msg.NodeID,
		Details: map[string]any{
			"status_code":    msg.StatusCode,
			"kind":           msg.Kind,
			"sec_request_id": msg.SecRequestID,
		},
	})
}

func (e *Engine) NodeEvent(ctx context.Context, node model.Node, eventType protocol.EventType) {
	e.emit(ctx, model.Event{
		EventType: string(eventType),
		Severity:  string(protocol.SeverityOps),
		ServerTs:  time.Now().Unix(),
		NodeID:    node.NodeID,
		Details: map[string]any{
			"node_name": node.NodeName,
			"region":    node.Region,
		},
	})
}

func (e *Engine) MonitorEvent(ctx context.Context, projectID int64, eventType protocol.EventType) {
	e.emit(ctx, model.Event{
		ProjectID: projectID,
		EventType: string(eventType),
		Severity:  string(protocol.SeverityOps),
		ServerTs:  time.Now().Unix(),
	})
}

func (e *Engine) Snapshot(nodes []model.Node) model.Snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	snap := model.Snapshot{
		Projects: make([]model.Project, 0, len(e.projects)),
		Screens:  make([]model.Screen, 0, len(e.screens)),
		SKUs:     make([]model.SKU, 0, len(e.skus)),
		Nodes:    append([]model.Node(nil), nodes...),
	}
	for _, p := range e.projects {
		snap.Projects = append(snap.Projects, p)
	}
	for _, s := range e.screens {
		snap.Screens = append(snap.Screens, s)
	}
	for _, s := range e.skus {
		snap.SKUs = append(snap.SKUs, s)
	}
	sort.Slice(snap.Projects, func(i, j int) bool { return snap.Projects[i].ID < snap.Projects[j].ID })
	sort.Slice(snap.Screens, func(i, j int) bool {
		if snap.Screens[i].ProjectID != snap.Screens[j].ProjectID {
			return snap.Screens[i].ProjectID < snap.Screens[j].ProjectID
		}
		return snap.Screens[i].ScreenID < snap.Screens[j].ScreenID
	})
	sort.Slice(snap.SKUs, func(i, j int) bool {
		if snap.SKUs[i].ProjectID != snap.SKUs[j].ProjectID {
			return snap.SKUs[i].ProjectID < snap.SKUs[j].ProjectID
		}
		if snap.SKUs[i].ScreenID != snap.SKUs[j].ScreenID {
			return snap.SKUs[i].ScreenID < snap.SKUs[j].ScreenID
		}
		return snap.SKUs[i].SkuID < snap.SKUs[j].SkuID
	})
	return snap
}

func (e *Engine) CurrentSKU(projectID, screenID, skuID int64) (model.SKU, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.skus[skuKey{projectID: projectID, screenID: screenID, skuID: skuID}]
	return s, ok
}

func (e *Engine) ProjectsForAssign() []model.Project {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.Project, 0, len(e.projects))
	for _, p := range e.projects {
		if p.Enabled {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (e *Engine) applyProject(st store.Store, projectID int64, meta protocol.ProjectMeta, se *sideEffects) bool {
	old, exists := e.projects[projectID]
	if exists && older(meta.ServerTs, old.ServerTs) {
		return false
	}
	if exists && !authoritativeProjectFlag(meta.Flag) {
		return false
	}
	enabled := true
	if exists {
		enabled = old.Enabled
	}
	cur := model.Project{
		ID:               projectID,
		Name:             meta.Name,
		Flag:             meta.Flag,
		FlagText:         meta.FlagText,
		Hot:              meta.Hot,
		WishCount:        meta.WishCount,
		PriceLow:         meta.PriceLow,
		PriceHigh:        meta.PriceHigh,
		SalableLow:       meta.SalableLow,
		SalableHigh:      meta.SalableHigh,
		Status:           meta.Status,
		IsSale:           meta.IsSale,
		SaleStart:        meta.SaleStart,
		SaleEnd:          meta.SaleEnd,
		CountDown:        meta.CountDown,
		ServerTs:         meta.ServerTs,
		LastEventTs:      old.LastEventTs,
		LastPolledTs:     old.LastPolledTs,
		Priority:         old.Priority,
		TargetIntervalMs: old.TargetIntervalMs,
		ReplicaFactor:    old.ReplicaFactor,
		Enabled:          enabled,
		UpdatedAt:        time.Now().UTC(),
	}
	scheduleChanged := !exists || old.Hot != cur.Hot || old.SaleStart != cur.SaleStart || old.Enabled != cur.Enabled
	e.projects[projectID] = cur
	if st != nil {
		se.store("upsert project", func(ctx context.Context) error {
			return st.UpsertProject(ctx, cur)
		})
	}
	if exists {
		eventType, severity, ok := events.ClassifyProject(projectComparable(old), projectComparable(cur), true)
		if ok {
			e.emitInto(st, eventFromProject(projectID, eventType, severity, old, cur), se)
		}
	}
	return scheduleChanged
}

func (e *Engine) applyScreen(st store.Store, pr protocol.PollResult, diff protocol.ScreenDiff, se *sideEffects) {
	key := screenKey{projectID: pr.ProjectID, screenID: diff.ScreenID}
	old, exists := e.screens[key]
	if exists && older(pr.ServerTs, old.ServerTs) {
		return
	}
	// A removal for a screen the master no longer tracks is a duplicate report from
	// another node — the first node's removal already deleted it and emitted the event.
	// Drop it so N nodes observing one upstream removal yield one screen_removed event
	// (mirrors ClassifySKU's old==nil && new==nil guard), and skip the redundant store
	// delete.
	if diff.Removed && !exists {
		return
	}
	eventType, severity, ok := events.ClassifyScreen(old.Flag, old.Clickable, exists, diff.Flag, diff.Clickable, diff.Removed)
	if diff.Removed {
		delete(e.screens, key)
		if st != nil {
			projectID, screenID := pr.ProjectID, diff.ScreenID
			se.store("delete screen", func(ctx context.Context) error {
				return st.DeleteScreen(ctx, projectID, screenID)
			})
		}
	} else {
		cur := model.Screen{
			ProjectID:   pr.ProjectID,
			ScreenID:    diff.ScreenID,
			Name:        diff.Name,
			StartTime:   diff.StartTime,
			Flag:        diff.Flag,
			FlagText:    diff.FlagText,
			Clickable:   diff.Clickable,
			SaleStart:   diff.SaleStart,
			SaleEnd:     diff.SaleEnd,
			ServerTs:    pr.ServerTs,
			LastEventTs: old.LastEventTs,
			UpdatedAt:   time.Now().UTC(),
		}
		e.screens[key] = cur
		if st != nil {
			se.store("upsert screen", func(ctx context.Context) error {
				return st.UpsertScreen(ctx, cur)
			})
		}
	}
	if ok {
		e.emitInto(st, model.Event{
			ProjectID:    pr.ProjectID,
			ScreenID:     diff.ScreenID,
			EventType:    string(eventType),
			Severity:     string(severity),
			OldFlag:      old.Flag,
			NewFlag:      diff.Flag,
			OldClickable: old.Clickable,
			NewClickable: diff.Clickable,
			FlagText:     diff.FlagText,
			ServerTs:     pr.ServerTs,
			NodeID:       pr.NodeID,
		}, se)
	}
}

func (e *Engine) applySKU(st store.Store, pr protocol.PollResult, diff protocol.SkuDiff, se *sideEffects) {
	key := skuKey{projectID: pr.ProjectID, screenID: diff.ScreenID, skuID: diff.SkuID}
	oldState, exists := e.skus[key]
	if exists && older(pr.ServerTs, oldState.ServerTs) {
		return
	}
	oldFP := fingerprintFromSKU(oldState, exists)
	newFP := diff.New
	if newFP != nil && exists && oldState.Flag == newFP.Flag && oldState.Clickable == newFP.Clickable &&
		oldState.PriceFen == newFP.PriceFen && oldState.Num == newFP.Num && oldState.LimitNum == diff.LimitNum {
		return
	}
	eventType, severity, ok := events.ClassifySKU(oldFP, newFP, diff.FlagText, diff.LimitNum)
	if diff.New == nil {
		delete(e.skus, key)
		if st != nil {
			projectID, screenID, skuID := pr.ProjectID, diff.ScreenID, diff.SkuID
			se.store("delete sku", func(ctx context.Context) error {
				return st.DeleteSKU(ctx, projectID, screenID, skuID)
			})
		}
	} else {
		cur := model.SKU{
			ProjectID:   pr.ProjectID,
			ScreenID:    diff.ScreenID,
			SkuID:       diff.SkuID,
			Desc:        diff.Desc,
			Flag:        diff.New.Flag,
			FlagText:    diff.FlagText,
			Clickable:   diff.New.Clickable,
			PriceFen:    diff.New.PriceFen,
			Num:         diff.New.Num,
			LimitNum:    diff.LimitNum,
			ServerTs:    pr.ServerTs,
			LastEventTs: oldState.LastEventTs,
			UpdatedAt:   time.Now().UTC(),
		}
		e.skus[key] = cur
		if st != nil {
			se.store("upsert sku", func(ctx context.Context) error {
				return st.UpsertSKU(ctx, cur)
			})
		}
		se.broadcast(protocol.StateUpdate{
			Type:      protocol.TypeStateUpdate,
			Version:   protocol.Version,
			ProjectID: pr.ProjectID,
			ScreenID:  diff.ScreenID,
			SkuID:     diff.SkuID,
			Flag:      cur.Flag,
			FlagText:  cur.FlagText,
			Clickable: cur.Clickable,
			PriceFen:  cur.PriceFen,
			Num:       cur.Num,
			LimitNum:  cur.LimitNum,
			ServerTs:  pr.ServerTs,
		})
	}
	if ok {
		e.emitInto(st, model.Event{
			ProjectID:    pr.ProjectID,
			ScreenID:     diff.ScreenID,
			SkuID:        diff.SkuID,
			EventType:    string(eventType),
			Severity:     string(severity),
			OldFlag:      fpFlag(oldFP),
			NewFlag:      fpFlag(newFP),
			OldClickable: fpClickable(oldFP),
			NewClickable: fpClickable(newFP),
			OldPriceFen:  fpPrice(oldFP),
			PriceFen:     fpPrice(newFP),
			OldNum:       fpNum(oldFP),
			NewNum:       fpNum(newFP),
			FlagText:     diff.FlagText,
			ServerTs:     pr.ServerTs,
			NodeID:       pr.NodeID,
		}, se)
	}
}

func (e *Engine) emit(ctx context.Context, ev model.Event) {
	ev.CreatedAt = time.Now().UTC()
	if e.store != nil {
		_ = e.store.InsertEvent(ctx, ev)
	}
	e.markLastEvent(ev)
	e.publish(ctx, eventMessage(ev))
}

func (e *Engine) emitInto(st store.Store, ev model.Event, se *sideEffects) {
	ev.CreatedAt = time.Now().UTC()
	e.markLastEvent(ev)
	if st != nil {
		se.store("insert event", func(ctx context.Context) error {
			return st.InsertEvent(ctx, ev)
		})
	}
	se.broadcast(eventMessage(ev))
}

func eventMessage(ev model.Event) protocol.Event {
	return protocol.Event{
		Type:      protocol.TypeEvent,
		Version:   protocol.Version,
		EventType: protocol.EventType(ev.EventType),
		Severity:  protocol.Severity(ev.Severity),
		ProjectID: ev.ProjectID,
		ScreenID:  ev.ScreenID,
		SkuID:     ev.SkuID,
		Old:       eventOld(ev),
		New:       eventNew(ev),
		PriceFen:  ev.PriceFen,
		FlagText:  ev.FlagText,
		NodeID:    ev.NodeID,
		Details:   ev.Details,
		ServerTs:  ev.ServerTs,
	}
}

func (e *Engine) markLastEvent(ev model.Event) {
	if ev.ProjectID <= 0 || ev.ServerTs <= 0 || ev.Severity == string(protocol.SeverityOps) {
		return
	}
	if p, ok := e.projects[ev.ProjectID]; ok && ev.ServerTs > p.LastEventTs {
		p.LastEventTs = ev.ServerTs
		e.projects[ev.ProjectID] = p
	}
	if ev.ScreenID > 0 {
		sk := screenKey{projectID: ev.ProjectID, screenID: ev.ScreenID}
		if s, ok := e.screens[sk]; ok && ev.ServerTs > s.LastEventTs {
			s.LastEventTs = ev.ServerTs
			e.screens[sk] = s
		}
	}
	if ev.SkuID > 0 {
		k := skuKey{projectID: ev.ProjectID, screenID: ev.ScreenID, skuID: ev.SkuID}
		if s, ok := e.skus[k]; ok && ev.ServerTs > s.LastEventTs {
			s.LastEventTs = ev.ServerTs
			e.skus[k] = s
		}
	}
}

func (e *Engine) publish(ctx context.Context, msg any) {
	e.mu.Lock()
	pub := e.pub
	e.mu.Unlock()
	if pub != nil {
		pub.Broadcast(ctx, msg)
	}
}

func flush(ctx context.Context, se *sideEffects) error {
	var firstErr error
	for _, op := range se.storeOps {
		if err := op.fn(ctx); err != nil {
			log.Printf("state store op %s failed: %v", op.desc, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if se.pub != nil {
		for _, msg := range se.msgs {
			se.pub.Broadcast(ctx, msg)
		}
	}
	return firstErr
}

func older(serverTs, current int64) bool {
	return current > 0 && serverTs > 0 && serverTs < current
}

func fingerprintFromSKU(s model.SKU, exists bool) *protocol.Fingerprint {
	if !exists {
		return nil
	}
	return &protocol.Fingerprint{Flag: s.Flag, Clickable: s.Clickable, PriceFen: s.PriceFen, Num: s.Num}
}

func fpFlag(fp *protocol.Fingerprint) int {
	if fp == nil {
		return 0
	}
	return fp.Flag
}

func fpClickable(fp *protocol.Fingerprint) bool {
	return fp != nil && fp.Clickable
}

func fpPrice(fp *protocol.Fingerprint) int {
	if fp == nil {
		return 0
	}
	return fp.PriceFen
}

func fpNum(fp *protocol.Fingerprint) int {
	if fp == nil {
		return 0
	}
	return fp.Num
}

func eventOld(ev model.Event) *protocol.Fingerprint {
	if ev.OldFlag == 0 && !ev.OldClickable && ev.OldNum == 0 {
		return nil
	}
	return &protocol.Fingerprint{Flag: ev.OldFlag, Clickable: ev.OldClickable, PriceFen: ev.OldPriceFen, Num: ev.OldNum}
}

func eventNew(ev model.Event) *protocol.Fingerprint {
	if ev.NewFlag == 0 && !ev.NewClickable && ev.NewNum == 0 {
		return nil
	}
	return &protocol.Fingerprint{Flag: ev.NewFlag, Clickable: ev.NewClickable, PriceFen: ev.PriceFen, Num: ev.NewNum}
}

func projectComparable(p model.Project) events.ProjectComparable {
	return events.ProjectComparable{
		Flag:        p.Flag,
		Hot:         p.Hot,
		SalableLow:  p.SalableLow,
		SalableHigh: p.SalableHigh,
	}
}

func authoritativeProjectFlag(flag int) bool {
	switch flag {
	case 1, 2, 4, 8:
		return true
	default:
		return false
	}
}

func eventFromProject(projectID int64, eventType protocol.EventType, severity protocol.Severity, old, cur model.Project) model.Event {
	return model.Event{
		ProjectID: projectID,
		EventType: string(eventType),
		Severity:  string(severity),
		OldFlag:   old.Flag,
		NewFlag:   cur.Flag,
		FlagText:  cur.FlagText,
		ServerTs:  cur.ServerTs,
		Details: map[string]any{
			"old_hot": old.Hot,
			"new_hot": cur.Hot,
		},
	}
}
