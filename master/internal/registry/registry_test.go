package registry

import (
	"context"
	"testing"
	"time"

	"github.com/WittF/fnmdp/shared/protocol"
)

type fakeSender struct {
	msgs []any
}

func (s *fakeSender) Send(ctx context.Context, msg any) error {
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *fakeSender) Close() {}

func TestSendAssignmentsOnlyChangedNodes(t *testing.T) {
	r := New()
	s := &fakeSender{}
	r.Register(protocol.Register{NodeID: "n1"}, s)

	assignments := map[string][]protocol.Assignment{
		"n1": {{ProjectID: 100, PhaseMod: 1000, IntervalMs: 30000}},
	}
	if err := r.SendAssignments(context.Background(), assignments); err != nil {
		t.Fatal(err)
	}
	if err := r.SendAssignments(context.Background(), assignments); err != nil {
		t.Fatal(err)
	}
	if len(s.msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(s.msgs))
	}

	assignments["n1"] = []protocol.Assignment{{ProjectID: 101, PhaseMod: 1000, IntervalMs: 30000}}
	if err := r.SendAssignments(context.Background(), assignments); err != nil {
		t.Fatal(err)
	}
	if len(s.msgs) != 2 {
		t.Fatalf("messages = %d, want 2 after change", len(s.msgs))
	}
}

func TestHeartbeatStampsMasterClockResistsSkew(t *testing.T) {
	r := New()
	r.Register(protocol.Register{NodeID: "n1"}, &fakeSender{})

	// Node wall clock is an hour behind the master. LastSeen must be stamped with
	// the master's receive time (not msg.Ts), so Reap's master-clock cutoff sees a
	// fresh node; msg.Ts is retained only as a skew diagnostic.
	skewedTs := time.Now().Add(-time.Hour).Unix()
	node := r.Heartbeat(protocol.Heartbeat{NodeID: "n1", Ts: skewedTs})
	if delta := time.Now().Unix() - node.LastSeen; delta < 0 || delta > 2 {
		t.Fatalf("LastSeen=%d not stamped with master clock (delta=%ds)", node.LastSeen, delta)
	}
	if node.SkewMs < (50 * time.Minute).Milliseconds() {
		t.Fatalf("skew_ms=%d, want ~1h", node.SkewMs)
	}
	if changed := r.Reap(36 * time.Second); len(changed) != 0 {
		t.Fatalf("reaped %d nodes despite a fresh (clock-skewed) heartbeat", len(changed))
	}
}

func TestRateEventSoftLimitReducesBudget(t *testing.T) {
	r := New()
	r.Register(protocol.Register{NodeID: "n1"}, &fakeSender{})
	node := r.RateEvent(protocol.RateEvent{NodeID: "n1", Kind: protocol.RateSoftLimited}, time.Minute, 0.8)
	if node.BudgetRPS >= 0.8 || node.BudgetRPS <= 0 {
		t.Fatalf("budget = %.3f, want reduced", node.BudgetRPS)
	}
	snap := r.Snapshot()
	if len(snap) != 1 || snap[0].BudgetRPS != node.BudgetRPS {
		t.Fatalf("snapshot = %+v, node=%+v", snap, node)
	}
}
