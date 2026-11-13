package assign

import (
	"sort"
	"sync"
	"time"

	"github.com/WittF/fnmdp/shared/protocol"
)

const DefaultPhaseMod = 1000

type Assignment struct {
	ProjectID int64
	Phase     int
	PhaseMod  int
	Interval  time.Duration
}

type Store struct {
	mu sync.RWMutex
	m  map[int64]Assignment
}

func NewStore() *Store {
	return &Store{m: map[int64]Assignment{}}
}

func (s *Store) Replace(assignments []protocol.Assignment) {
	next := make(map[int64]Assignment, len(assignments))
	for _, a := range assignments {
		if normalized, ok := FromProtocol(a); ok {
			next[normalized.ProjectID] = normalized
		}
	}
	s.mu.Lock()
	s.m = next
	s.mu.Unlock()
}

func (s *Store) ApplyDelta(added []protocol.Assignment, removed []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range removed {
		delete(s.m, id)
	}
	for _, a := range added {
		if normalized, ok := FromProtocol(a); ok {
			s.m[normalized.ProjectID] = normalized
		}
	}
}

func (s *Store) Snapshot() []Assignment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Assignment, 0, len(s.m))
	for _, a := range s.m {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ProjectID < out[j].ProjectID
	})
	return out
}

func FromProtocol(a protocol.Assignment) (Assignment, bool) {
	if a.ProjectID <= 0 || a.IntervalMs <= 0 {
		return Assignment{}, false
	}
	phaseMod := a.PhaseMod
	if phaseMod <= 0 {
		phaseMod = DefaultPhaseMod
	}
	phase := a.Phase % phaseMod
	if phase < 0 {
		phase += phaseMod
	}
	return Assignment{
		ProjectID: a.ProjectID,
		Phase:     phase,
		PhaseMod:  phaseMod,
		Interval:  time.Duration(a.IntervalMs) * time.Millisecond,
	}, true
}

func ToProtocol(a Assignment) protocol.Assignment {
	phaseMod := a.PhaseMod
	if phaseMod <= 0 {
		phaseMod = DefaultPhaseMod
	}
	return protocol.Assignment{
		ProjectID:  a.ProjectID,
		Phase:      a.Phase,
		PhaseMod:   phaseMod,
		IntervalMs: int(a.Interval / time.Millisecond),
	}
}
