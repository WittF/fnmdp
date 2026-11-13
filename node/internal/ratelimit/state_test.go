package ratelimit

import (
	"testing"
	"time"
)

func TestSoftBackoffExponentialAndSuccessReset(t *testing.T) {
	now := time.Unix(100, 0)
	s := New(time.Second, 10*time.Minute)

	if d := s.RecordSoft(now); d != time.Second {
		t.Fatalf("first backoff = %s", d)
	}
	if d := s.RecordSoft(now); d != 2*time.Second {
		t.Fatalf("second backoff = %s", d)
	}
	if d := s.RecordSoft(now); d != 4*time.Second {
		t.Fatalf("third backoff = %s", d)
	}
	if until := s.BlockedUntil(now); !until.Equal(now.Add(4 * time.Second)) {
		t.Fatalf("blocked until = %s", until)
	}

	s.RecordSuccess(now.Add(5 * time.Second))
	if until := s.BlockedUntil(now.Add(5 * time.Second)); !until.IsZero() {
		t.Fatalf("blocked after success = %s", until)
	}
	if d := s.RecordSoft(now.Add(6 * time.Second)); d != time.Second {
		t.Fatalf("backoff after reset = %s", d)
	}
}

func TestHardBlockStopsUntilCooldown(t *testing.T) {
	now := time.Unix(100, 0)
	s := New(time.Second, 10*time.Minute)

	if d := s.RecordHard(now); d != 10*time.Minute {
		t.Fatalf("hard cooldown = %s", d)
	}
	if until := s.BlockedUntil(now.Add(9 * time.Minute)); !until.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("blocked until = %s", until)
	}
	if until := s.BlockedUntil(now.Add(10*time.Minute + time.Nanosecond)); !until.IsZero() {
		t.Fatalf("blocked after cooldown = %s", until)
	}

	snap := s.Snapshot(now.Add(9 * time.Minute))
	if snap.HardBlocked != 1 || snap.SoftLimited != 0 || snap.HardHits != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}
}
