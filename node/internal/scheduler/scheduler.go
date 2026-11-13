package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/WittF/fnmdp/node/internal/assign"
)

type Source interface {
	Snapshot() []assign.Assignment
}

type MinIntervalSource interface {
	MinInterval() time.Duration
}

type MinIntervalFunc func() time.Duration

func (f MinIntervalFunc) MinInterval() time.Duration { return f() }

type Handler func(context.Context, int64)

type Scheduler struct {
	source      Source
	minInterval MinIntervalSource
	handler     Handler
	tick        time.Duration
	queue       chan task
	done        chan int64
}

type task struct {
	projectID int64
	due       time.Time
	interval  time.Duration
}

func New(source Source, minInterval MinIntervalSource, handler Handler) *Scheduler {
	return &Scheduler{
		source:      source,
		minInterval: minInterval,
		handler:     handler,
		tick:        25 * time.Millisecond,
		queue:       make(chan task, 1024),
		done:        make(chan int64, 1024),
	}
}

func (s *Scheduler) SetTickForTest(d time.Duration) {
	if d > 0 {
		s.tick = d
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.worker(ctx)
	}()

	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()

	next := map[int64]time.Time{}
	queued := map[int64]bool{}
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case now := <-ticker.C:
			assignments := s.source.Snapshot()
			active := make(map[int64]assign.Assignment, len(assignments))
			for _, a := range assignments {
				active[a.ProjectID] = a
				due, ok := next[a.ProjectID]
				if !ok {
					due = NextTrigger(now, a.Interval, a.Phase, a.PhaseMod)
					next[a.ProjectID] = due
					continue
				}
				if now.Before(due) || queued[a.ProjectID] {
					continue
				}
				if now.After(due.Add(a.Interval)) {
					next[a.ProjectID] = NextTrigger(now, a.Interval, a.Phase, a.PhaseMod)
					continue
				}
				select {
				case s.queue <- task{projectID: a.ProjectID, due: due, interval: a.Interval}:
					queued[a.ProjectID] = true
					for !next[a.ProjectID].After(now) {
						next[a.ProjectID] = next[a.ProjectID].Add(a.Interval)
					}
				default:
					next[a.ProjectID] = NextTrigger(now, a.Interval, a.Phase, a.PhaseMod)
				}
			}
			for id := range next {
				if _, ok := active[id]; !ok {
					delete(next, id)
					delete(queued, id)
				}
			}
		case projectID := <-s.done:
			queued[projectID] = false
		}
	}
}

func (s *Scheduler) worker(ctx context.Context) {
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-s.queue:
			if now := time.Now(); now.After(t.due.Add(t.interval)) {
				s.markDone(ctx, t.projectID)
				continue
			}
			minInterval := s.minInterval.MinInterval()
			if minInterval > 0 && !last.IsZero() {
				wait := last.Add(minInterval).Sub(time.Now())
				if wait > 0 {
					timer := time.NewTimer(wait)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			}
			s.handler(ctx, t.projectID)
			last = time.Now()
			s.markDone(ctx, t.projectID)
		}
	}
}

func (s *Scheduler) markDone(ctx context.Context, projectID int64) {
	select {
	case s.done <- projectID:
	case <-ctx.Done():
	}
}

func NextTrigger(now time.Time, interval time.Duration, phase, phaseMod int) time.Time {
	if interval <= 0 {
		return now
	}
	if phaseMod <= 0 {
		phaseMod = assign.DefaultPhaseMod
	}
	phase %= phaseMod
	if phase < 0 {
		phase += phaseMod
	}
	n := now.UnixNano()
	step := int64(interval)
	gridStart := time.Unix(0, (n/step)*step)
	offset := time.Duration(int64(interval) * int64(phase) / int64(phaseMod))
	next := gridStart.Add(offset)
	if !next.After(now) {
		next = next.Add(interval)
	}
	return next
}
