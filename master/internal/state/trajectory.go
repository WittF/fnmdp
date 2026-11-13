package state

import (
	"sort"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/store"
)

// BuildTrajectory reconstructs a per-SKU step function from sparse change events
// and masks it with project-level poll coverage. Timestamps in TrajectoryQuery and
// TrajectoryPoint are epoch milliseconds; state_events and poll_coverage storage
// remain second/microsecond based.
func BuildTrajectory(current model.SKU, events []model.Event, coverage []model.PollCoverage, q store.TrajectoryQuery, liveGrace time.Duration, nowMs int64) []model.TrajectoryPoint {
	if q.IntervalMs <= 0 || q.UntilMs < q.SinceMs {
		return []model.TrajectoryPoint{}
	}
	events = append([]model.Event(nil), events...)
	sort.Slice(events, func(i, j int) bool {
		ti, tj := eventTimeMs(events[i]), eventTimeMs(events[j])
		if ti != tj {
			return ti < tj
		}
		return events[i].ID < events[j].ID
	})
	intervals := coverageIntervals(coverage, q, liveGrace, nowMs)
	points := make([]model.TrajectoryPoint, 0, int((q.UntilMs-q.SinceMs)/q.IntervalMs)+1)
	for t := q.SinceMs; t <= q.UntilMs; t += q.IntervalMs {
		end := t + q.IntervalMs
		observed := overlapsCoverage(intervals, t, end)
		point := model.TrajectoryPoint{T: t, Observed: observed}
		if observed {
			state := stateAt(events, current, t)
			point.Flag = state.Flag
			point.Clickable = state.Clickable
			point.FlagText = state.FlagText
			point.Samples = eventSamples(events, intervals, t, end)
		}
		points = append(points, point)
	}
	return points
}

type coverageInterval struct {
	startMs int64
	endMs   int64
}

func coverageIntervals(rows []model.PollCoverage, q store.TrajectoryQuery, liveGrace time.Duration, nowMs int64) []coverageInterval {
	if len(rows) == 0 {
		return nil
	}
	rows = append([]model.PollCoverage(nil), rows...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].StartTs != rows[j].StartTs {
			return rows[i].StartTs < rows[j].StartTs
		}
		return rows[i].EndTs < rows[j].EndTs
	})
	latestEnd := int64(0)
	for _, c := range rows {
		if c.EndTs > latestEnd {
			latestEnd = c.EndTs
		}
	}
	graceSec := int64(liveGrace / time.Second)
	out := make([]coverageInterval, 0, len(rows))
	for _, c := range rows {
		if c.ProjectID != q.ProjectID || c.StartTs <= 0 || c.EndTs <= 0 {
			continue
		}
		startMs := c.StartTs * 1000
		endMs := c.EndTs * 1000
		// The newest open interval may trail wall-clock now by one keepalive/reconcile
		// cycle. Extend it only while it is still within the configured live grace; old
		// closed intervals keep their real last-observed end so historical gaps stay visible.
		if c.EndTs == latestEnd && graceSec > 0 && (q.UntilMs/1000)-c.EndTs <= graceSec {
			// Live-edge forward-fill only goes up to master's real clock, never to the
			// client-supplied untilMs: the latter rides an unbounded monotonic forward
			// clock ratchet and would paint the SKU's current state into never-observed
			// future buckets (the "solid connected block" bug).
			limit := q.UntilMs
			if nowMs > 0 && nowMs < limit {
				limit = nowMs
			}
			if limit > endMs { // never shrink below the real last-observed end (c.EndTs*1000)
				endMs = limit
			}
		}
		if endMs < q.SinceMs || startMs > q.UntilMs+q.IntervalMs {
			continue
		}
		if startMs < q.SinceMs {
			startMs = q.SinceMs
		}
		if endMs > q.UntilMs+q.IntervalMs {
			endMs = q.UntilMs + q.IntervalMs
		}
		if len(out) > 0 && startMs <= out[len(out)-1].endMs {
			if endMs > out[len(out)-1].endMs {
				out[len(out)-1].endMs = endMs
			}
			continue
		}
		out = append(out, coverageInterval{startMs: startMs, endMs: endMs})
	}
	return out
}

func overlapsCoverage(intervals []coverageInterval, startMs, endMs int64) bool {
	for _, c := range intervals {
		if startMs <= c.endMs && endMs > c.startMs {
			return true
		}
		if c.startMs > endMs {
			return false
		}
	}
	return false
}

func coveredAt(intervals []coverageInterval, t int64) bool {
	for _, c := range intervals {
		if t >= c.startMs && t <= c.endMs {
			return true
		}
		if c.startMs > t {
			return false
		}
	}
	return false
}

func eventTimeMs(e model.Event) int64 {
	if !e.CreatedAt.IsZero() {
		return e.CreatedAt.UnixMilli()
	}
	return e.ServerTs * 1000
}

func stateAt(events []model.Event, current model.SKU, t int64) model.TrajectoryState {
	state := model.TrajectoryState{
		Flag:      current.Flag,
		Clickable: current.Clickable,
		FlagText:  current.FlagText,
	}
	if len(events) == 0 {
		return state
	}
	first := events[0]
	if t < eventTimeMs(first) {
		return model.TrajectoryState{
			Flag:      first.OldFlag,
			Clickable: first.OldClickable,
			// old_flag_text was never stored. For flag=4 this can render as ordinary
			// sold-out instead of temporary sold-out before the first event.
			FlagText: "",
		}
	}
	if t >= eventTimeMs(events[len(events)-1]) {
		return state
	}
	for _, ev := range events {
		if eventTimeMs(ev) > t {
			old := oldEventState(ev)
			if state.Flag != old.Flag || state.Clickable != old.Clickable {
				return old
			}
			break
		}
		state = newEventState(ev)
	}
	return state
}

func oldEventState(ev model.Event) model.TrajectoryState {
	return model.TrajectoryState{
		Flag:      ev.OldFlag,
		Clickable: ev.OldClickable,
	}
}

func newEventState(ev model.Event) model.TrajectoryState {
	return model.TrajectoryState{
		Flag:      ev.NewFlag,
		Clickable: ev.NewClickable,
		FlagText:  ev.FlagText,
	}
}

func eventSamples(events []model.Event, intervals []coverageInterval, startMs, endMs int64) []model.TrajectoryState {
	var out []model.TrajectoryState
	for _, ev := range events {
		t := eventTimeMs(ev)
		if t < startMs {
			continue
		}
		if t >= endMs {
			break
		}
		if !coveredAt(intervals, t) {
			continue
		}
		out = append(out, newEventState(ev))
	}
	return out
}
