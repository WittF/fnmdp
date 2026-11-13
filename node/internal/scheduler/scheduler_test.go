package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/WittF/fnmdp/node/internal/assign"
)

type staticSource struct {
	assignments []assign.Assignment
}

func (s staticSource) Snapshot() []assign.Assignment {
	return append([]assign.Assignment(nil), s.assignments...)
}

type fixedMinInterval time.Duration

func (f fixedMinInterval) MinInterval() time.Duration { return time.Duration(f) }

func TestNextTriggerPhaseSpread(t *testing.T) {
	now := time.Unix(80, 0)
	interval := 40 * time.Second
	var got []time.Duration
	for phase := 0; phase < 4; phase++ {
		next := NextTrigger(now, interval, phase, 4)
		got = append(got, next.Sub(now))
	}
	want := []time.Duration{40 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("phase %d offset = %s, want %s (all=%v)", i, got[i], want[i], got)
		}
	}
}

func TestSchedulerHonorsMinInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	source := staticSource{assignments: []assign.Assignment{
		{ProjectID: 1, Interval: 6 * time.Millisecond, PhaseMod: 1},
		{ProjectID: 2, Interval: 6 * time.Millisecond, PhaseMod: 1},
		{ProjectID: 3, Interval: 6 * time.Millisecond, PhaseMod: 1},
	}}
	var mu sync.Mutex
	var times []time.Time
	s := New(source, fixedMinInterval(10*time.Millisecond), func(ctx context.Context, projectID int64) {
		mu.Lock()
		times = append(times, time.Now())
		if len(times) >= 4 {
			cancel()
		}
		mu.Unlock()
	})
	s.SetTickForTest(time.Millisecond)

	_ = s.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(times) < 4 {
		t.Fatalf("executions = %d, want at least 4", len(times))
	}
	for i := 1; i < len(times); i++ {
		if delta := times[i].Sub(times[i-1]); delta < 9*time.Millisecond {
			t.Fatalf("delta[%d] = %s, want >= 9ms; times=%v", i, delta, times)
		}
	}
}
