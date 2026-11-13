package ratelimit

import (
	"context"
	"sync"
	"time"
)

const maxSoftBackoff = 30 * time.Second

type State struct {
	mu           sync.Mutex
	softBase     time.Duration
	hardCooldown time.Duration
	softDelay    time.Duration
	softUntil    time.Time
	hardUntil    time.Time
	inFlight     int
	softHits     int
	hardHits     int
}

func New(softBase, hardCooldown time.Duration) *State {
	if softBase <= 0 {
		softBase = time.Second
	}
	if hardCooldown <= 0 {
		hardCooldown = 10 * time.Minute
	}
	return &State{softBase: softBase, hardCooldown: hardCooldown}
}

func (s *State) Configure(softBase, hardCooldown time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if softBase > 0 {
		s.softBase = softBase
	}
	if hardCooldown > 0 {
		s.hardCooldown = hardCooldown
	}
}

func (s *State) Wait(ctx context.Context) error {
	for {
		until := s.BlockedUntil(time.Now())
		if until.IsZero() {
			return nil
		}
		timer := time.NewTimer(time.Until(until))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *State) BlockedUntil(now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blockedUntilLocked(now)
}

func (s *State) blockedUntilLocked(now time.Time) time.Time {
	var until time.Time
	if s.hardUntil.After(now) {
		until = s.hardUntil
	}
	if s.softUntil.After(now) && s.softUntil.After(until) {
		until = s.softUntil
	}
	return until
}

func (s *State) Begin() {
	s.mu.Lock()
	s.inFlight++
	s.mu.Unlock()
}

func (s *State) End() {
	s.mu.Lock()
	if s.inFlight > 0 {
		s.inFlight--
	}
	s.mu.Unlock()
}

func (s *State) RecordSoft(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.softHits++
	if s.softDelay <= 0 {
		s.softDelay = s.softBase
	} else {
		s.softDelay *= 2
	}
	if s.softDelay > maxSoftBackoff {
		s.softDelay = maxSoftBackoff
	}
	s.softUntil = now.Add(s.softDelay)
	return s.softDelay
}

func (s *State) RecordHard(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hardHits++
	s.hardUntil = now.Add(s.hardCooldown)
	s.softUntil = time.Time{}
	s.softDelay = 0
	return s.hardCooldown
}

func (s *State) RecordSuccess(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hardUntil.After(now) {
		s.hardUntil = time.Time{}
	}
	if !s.softUntil.After(now) {
		s.softUntil = time.Time{}
	}
	s.softDelay = 0
}

func (s *State) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{
		InFlight:     s.inFlight,
		SoftLimited:  boolToInt(s.softUntil.After(now)),
		HardBlocked:  boolToInt(s.hardUntil.After(now)),
		SoftHits:     s.softHits,
		HardHits:     s.hardHits,
		BlockedUntil: s.blockedUntilLocked(now),
	}
}

type Snapshot struct {
	InFlight     int
	SoftLimited  int
	HardBlocked  int
	SoftHits     int
	HardHits     int
	BlockedUntil time.Time
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
