package server

import (
	"context"
	"errors"
	"net/http"
	_ "net/http/pprof"
	"sync/atomic"
	"time"

	"github.com/WittF/fnmdp/master/internal/api"
	"github.com/WittF/fnmdp/master/internal/assign"
	"github.com/WittF/fnmdp/master/internal/config"
	"github.com/WittF/fnmdp/master/internal/hub"
	"github.com/WittF/fnmdp/master/internal/ingest"
	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/registry"
	"github.com/WittF/fnmdp/master/internal/state"
	"github.com/WittF/fnmdp/master/internal/store"
	"github.com/WittF/fnmdp/shared/protocol"
)

type App struct {
	cfg      config.Config
	store    store.Store
	registry *registry.Registry
	engine   *state.Engine
	hub      *hub.Hub
	server   *http.Server
	healthy  atomic.Bool
}

func New(ctx context.Context, cfg config.Config, st store.Store, engine *state.Engine) *App {
	reg := registry.New()
	engine.SetCoverageGap(cfg.CoverageGap)
	app := &App{
		cfg:      cfg,
		store:    st,
		registry: reg,
		engine:   engine,
	}
	app.healthy.Store(true)
	app.hub = hub.New(func() model.Snapshot { return app.Snapshot() })
	app.engine.SetPublisher(app.hub)
	return app
}

func (a *App) Run(ctx context.Context) error {
	for _, id := range a.cfg.InitialProjects {
		_ = a.engine.EnsureProject(ctx, id)
		a.engine.MonitorEvent(ctx, id, protocol.EventMonitorAdded)
	}

	mux := a.Handler(ctx)
	a.server = &http.Server{Addr: a.cfg.ListenAddr, Handler: mux}
	go a.reapLoop(ctx)
	go a.periodicRebalanceLoop(ctx)
	go a.coverageFlushLoop(ctx)
	go a.pruneLoop(ctx)
	go a.watchdogLoop(ctx)

	errCh := make(chan error, 1)
	go func() {
		err := a.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		a.hub.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.server.Shutdown(shutdownCtx)
		_ = a.engine.FlushCoverage(shutdownCtx)
		if a.store != nil {
			a.store.Close()
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (a *App) Handler(ctx context.Context) http.Handler {
	if ctx == nil {
		ctx = context.Background()
	}
	mux := http.NewServeMux()
	mux.Handle("/ingest", ingest.Handler{
		NodeToken: a.cfg.NodeToken,
		BaseCtx:   ctx,
		Callbacks: ingest.Callbacks{
			OnRegister:   a.onRegister,
			OnHeartbeat:  a.onHeartbeat,
			OnPollResult: a.onPollResult,
			OnRateEvent:  a.onRateEvent,
			OnDisconnect: a.onDisconnect,
		},
	})
	mux.HandleFunc("/live", a.hub.HandleLive)
	mux.Handle("/api/", api.Handler{AdminToken: a.cfg.AdminToken, Service: a})
	mux.Handle("/debug/pprof/", a.adminGate(http.DefaultServeMux))
	// Liveness/readiness probe for k8s. Always 200 once the HTTP server is up;
	// PG/Valkey degrade gracefully so they are not gated here.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		if !a.healthy.Load() {
			http.Error(w, "engine lock watchdog failed", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func (a *App) Snapshot() model.Snapshot {
	return a.engine.Snapshot(a.registry.Snapshot())
}

func (a *App) Events(f store.EventFilter) ([]model.Event, error) {
	if a.store == nil {
		return nil, nil
	}
	timeout := a.cfg.PGOpTimeout
	if timeout <= 0 {
		timeout = config.DefaultPGOpTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return a.store.Events(ctx, f)
}

func (a *App) Trajectory(q store.TrajectoryQuery) ([]model.TrajectoryPoint, error) {
	current, ok := a.engine.CurrentSKU(q.ProjectID, q.ScreenID, q.SkuID)
	if !ok {
		return []model.TrajectoryPoint{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	untilSec := ceilDiv(q.UntilMs, 1000)
	gap := a.coverageGap()
	sinceSec := q.SinceMs / 1000
	if gapSec := int64(gap / time.Second); gapSec > 0 {
		sinceSec -= gapSec + 1
		if sinceSec < 0 {
			sinceSec = 0
		}
	}
	var events []model.Event
	var coverage []model.PollCoverage
	var err error
	if a.store != nil {
		events, err = a.store.Events(ctx, store.EventFilter{
			ProjectID: q.ProjectID,
			ScreenID:  q.ScreenID,
			SkuID:     q.SkuID,
			UntilTs:   untilSec,
			Limit:     store.MaxHistoryLimit,
			MaxLimit:  store.MaxHistoryLimit,
		})
		if err != nil {
			return nil, err
		}
		coverage, err = a.store.Coverage(ctx, q.ProjectID, sinceSec, untilSec)
		if err != nil {
			return nil, err
		}
	}
	coverage = mergeCoverage(coverage, a.engine.CoverageSnapshot(), q.ProjectID, sinceSec, untilSec)
	return state.BuildTrajectory(current, events, coverage, q, gap, time.Now().UnixMilli()), nil
}

const maxBatchTrajectoryEvents = 50000

func (a *App) Trajectories(q store.TrajectoryQuery) ([]model.TrajectorySeries, error) {
	skus := a.projectTrajectorySKUs(q)
	if len(skus) == 0 {
		return []model.TrajectorySeries{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	untilSec := ceilDiv(q.UntilMs, 1000)
	gap := a.coverageGap()
	sinceSec := q.SinceMs / 1000
	if gapSec := int64(gap / time.Second); gapSec > 0 {
		sinceSec -= gapSec + 1
		if sinceSec < 0 {
			sinceSec = 0
		}
	}

	var events []model.Event
	var coverage []model.PollCoverage
	var err error
	if a.store != nil {
		eventCap := store.MaxHistoryLimit * len(skus)
		if eventCap > maxBatchTrajectoryEvents {
			eventCap = maxBatchTrajectoryEvents
		}
		if eventCap < store.MaxHistoryLimit {
			eventCap = store.MaxHistoryLimit
		}
		events, err = a.store.Events(ctx, store.EventFilter{
			ProjectID: q.ProjectID,
			UntilTs:   untilSec,
			Limit:     eventCap,
			MaxLimit:  eventCap,
		})
		if err != nil {
			return nil, err
		}
		if len(events) >= eventCap {
			return a.singleTrajectories(q, skus)
		}
		coverage, err = a.store.Coverage(ctx, q.ProjectID, sinceSec, untilSec)
		if err != nil {
			return nil, err
		}
	}
	coverage = mergeCoverage(coverage, a.engine.CoverageSnapshot(), q.ProjectID, sinceSec, untilSec)

	eventsBySKU := map[trajectorySKUKey][]model.Event{}
	for _, ev := range events {
		if ev.ScreenID <= 0 || ev.SkuID <= 0 {
			continue
		}
		key := trajectorySKUKey{screenID: ev.ScreenID, skuID: ev.SkuID}
		eventsBySKU[key] = append(eventsBySKU[key], ev)
	}

	nowMs := time.Now().UnixMilli()
	out := make([]model.TrajectorySeries, 0, len(skus))
	for _, sku := range skus {
		tq := q
		tq.ScreenID = sku.ScreenID
		tq.SkuID = sku.SkuID
		key := trajectorySKUKey{screenID: sku.ScreenID, skuID: sku.SkuID}
		out = append(out, model.TrajectorySeries{
			ProjectID: sku.ProjectID,
			ScreenID:  sku.ScreenID,
			SkuID:     sku.SkuID,
			Points:    state.BuildTrajectory(sku, eventsBySKU[key], coverage, tq, gap, nowMs),
		})
	}
	return out, nil
}

func (a *App) singleTrajectories(q store.TrajectoryQuery, skus []model.SKU) ([]model.TrajectorySeries, error) {
	out := make([]model.TrajectorySeries, 0, len(skus))
	for _, sku := range skus {
		tq := q
		tq.ScreenID = sku.ScreenID
		tq.SkuID = sku.SkuID
		points, err := a.Trajectory(tq)
		if err != nil {
			return nil, err
		}
		out = append(out, model.TrajectorySeries{
			ProjectID: sku.ProjectID,
			ScreenID:  sku.ScreenID,
			SkuID:     sku.SkuID,
			Points:    points,
		})
	}
	return out, nil
}

type trajectorySKUKey struct {
	screenID int64
	skuID    int64
}

func (a *App) projectTrajectorySKUs(q store.TrajectoryQuery) []model.SKU {
	snap := a.engine.Snapshot(nil)
	out := make([]model.SKU, 0)
	for _, sku := range snap.SKUs {
		if sku.ProjectID != q.ProjectID {
			continue
		}
		if q.ScreenID > 0 && sku.ScreenID != q.ScreenID {
			continue
		}
		if q.SkuID > 0 && sku.SkuID != q.SkuID {
			continue
		}
		out = append(out, sku)
	}
	return out
}

func (a *App) coverageGap() time.Duration {
	if a.cfg.CoverageGap > 0 {
		return a.cfg.CoverageGap
	}
	return config.DefaultCoverageGap
}

func (a *App) adminGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.AdminToken == "" || r.Header.Get("Authorization") != "Bearer "+a.cfg.AdminToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) ConfigureProject(id int64, priority, intervalMs, replica int, enabled bool) error {
	err := a.engine.ConfigureProject(context.Background(), id, priority, intervalMs, replica, enabled)
	if err == nil {
		if enabled {
			a.engine.MonitorEvent(context.Background(), id, protocol.EventMonitorAdded)
		} else {
			a.engine.MonitorEvent(context.Background(), id, protocol.EventMonitorRemoved)
		}
	}
	return err
}

func (a *App) Rebalance() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	assignments := a.planAssignments()
	_ = a.registry.SendAssignments(ctx, assignments)
}

func (a *App) onRegister(ctx context.Context, msg protocol.Register, sender registry.Sender) {
	node, online := a.registry.Register(msg, sender)
	a.hub.Broadcast(ctx, nodeStatus(node))
	if online {
		a.engine.NodeEvent(ctx, node, protocol.EventNodeOnline)
	}
	_ = sender.Send(ctx, protocol.Config{
		Type:           protocol.TypeConfig,
		Version:        protocol.Version,
		MinIntervalMs:  a.cfg.MinIntervalMs,
		SoftBackoffMs:  a.cfg.SoftBackoffMs,
		HardCooldownMs: a.cfg.HardCooldownMs,
		KeepaliveMs:    a.cfg.KeepaliveMs,
		UserAgent:      a.cfg.UserAgent,
	})
	a.Rebalance()
}

func (a *App) onHeartbeat(ctx context.Context, msg protocol.Heartbeat) {
	node := a.registry.Heartbeat(msg)
	a.hub.Broadcast(ctx, nodeStatus(node))
}

func (a *App) onPollResult(ctx context.Context, msg protocol.PollResult) {
	scheduleChanged, err := a.engine.ProcessPoll(ctx, msg)
	if err == nil && scheduleChanged {
		a.Rebalance()
	}
}

func (a *App) onRateEvent(ctx context.Context, msg protocol.RateEvent) {
	node := a.registry.RateEvent(msg, time.Duration(a.cfg.HardCooldownMs)*time.Millisecond, a.cfg.NodeBudgetRPS)
	a.engine.RateEvent(ctx, msg)
	a.hub.Broadcast(ctx, nodeStatus(node))
	a.Rebalance()
}

func (a *App) onDisconnect(ctx context.Context, nodeID string) {
	node, changed := a.registry.MarkDisconnected(nodeID)
	if !changed {
		return
	}
	a.hub.Broadcast(ctx, nodeStatus(node))
	a.engine.NodeEvent(ctx, node, protocol.EventNodeOffline)
	a.Rebalance()
}

func (a *App) reapLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed := a.registry.Reap(a.cfg.HeartbeatTimeout)
			if len(changed) == 0 {
				continue
			}
			for _, node := range changed {
				a.hub.Broadcast(ctx, nodeStatus(node))
				a.engine.NodeEvent(ctx, node, protocol.EventNodeOffline)
			}
			a.Rebalance()
		}
	}
}

func (a *App) periodicRebalanceLoop(ctx context.Context) {
	ticker := time.NewTicker(45 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.Rebalance()
		}
	}
}

func (a *App) coverageFlushLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = a.engine.FlushCoverage(flushCtx)
			cancel()
		}
	}
}

func (a *App) pruneLoop(ctx context.Context) {
	if a.store == nil || a.cfg.HistoryRetention <= 0 {
		return
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-a.cfg.HistoryRetention).Unix()
			pruneCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = a.store.PruneBefore(pruneCtx, cutoff)
			cancel()
		}
	}
}

func (a *App) watchdogLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	a.healthy.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if a.pingEngine(ctx, 5*time.Second) {
				a.healthy.Store(true)
			} else {
				a.healthy.Store(false)
			}
		}
	}
}

func (a *App) pingEngine(ctx context.Context, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		a.engine.Ping()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func (a *App) planAssignments() map[string][]protocol.Assignment {
	projects := a.engine.ProjectsForAssign()
	nodes := a.registry.Snapshot()
	ap := make([]assign.Project, 0, len(projects))
	for _, p := range projects {
		ap = append(ap, assign.Project{
			ID:               p.ID,
			Enabled:          p.Enabled,
			Hot:              p.Hot,
			SaleStart:        p.SaleStart,
			Priority:         p.Priority,
			TargetIntervalMs: p.TargetIntervalMs,
			ReplicaFactor:    p.ReplicaFactor,
		})
	}
	an := make([]assign.Node, 0, len(nodes))
	for _, n := range nodes {
		an = append(an, assign.Node{
			ID:           n.NodeID,
			Region:       n.Region,
			Online:       n.Online,
			BudgetRPS:    n.BudgetRPS,
			BlockedUntil: n.BlockedUntil,
		})
	}
	return assign.Plan(ap, an, assign.Defaults{
		Now:               time.Now(),
		DefaultIntervalMs: a.cfg.DefaultIntervalMs,
		HotIntervalMs:     a.cfg.DefaultHotIntervalMs,
		DefaultReplica:    a.cfg.DefaultReplicaFactor,
		NodeBudgetRPS:     a.cfg.NodeBudgetRPS,
		HotMaxReplica:     a.cfg.HotMaxReplica,
	})
}

func mergeCoverage(base, open []model.PollCoverage, projectID, sinceSec, untilSec int64) []model.PollCoverage {
	out := append([]model.PollCoverage(nil), base...)
	for _, c := range open {
		if c.ProjectID != projectID {
			continue
		}
		if sinceSec > 0 && c.EndTs < sinceSec {
			continue
		}
		if untilSec > 0 && c.StartTs > untilSec {
			continue
		}
		replaced := false
		for i := range out {
			if out[i].ProjectID == c.ProjectID && out[i].StartTs == c.StartTs {
				if c.EndTs > out[i].EndTs {
					out[i].EndTs = c.EndTs
				}
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, c)
		}
	}
	return out
}

func ceilDiv(n, d int64) int64 {
	if d <= 0 {
		return n
	}
	return (n + d - 1) / d
}

func nodeStatus(n model.Node) protocol.NodeStatus {
	return protocol.NodeStatus{
		Type:        protocol.TypeNodeStatus,
		Version:     protocol.Version,
		NodeID:      n.NodeID,
		NodeName:    n.NodeName,
		Region:      n.Region,
		Online:      n.Online,
		InFlight:    n.InFlight,
		SoftLimited: n.SoftLimited,
		HardBlocked: n.HardBlocked,
		LastSeen:    n.LastSeen,
		SkewMs:      n.SkewMs,
	}
}
