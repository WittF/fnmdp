package assign

import (
	"hash/fnv"
	"math"
	"sort"
	"time"

	"github.com/WittF/fnmdp/shared/protocol"
)

const DefaultPhaseMod = 1000

// projectPhase derives a stable per-project base offset in [0, DefaultPhaseMod)
// from the project ID, so different projects' poll grids don't all align to the
// same absolute interval boundary. Deterministic (FNV-1a) so rebalances are stable.
func projectPhase(id int64) int {
	h := fnv.New32a()
	var b [8]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(id >> (8 * i))
	}
	_, _ = h.Write(b[:])
	return int(h.Sum32() % uint32(DefaultPhaseMod))
}

type Project struct {
	ID               int64
	Enabled          bool
	Hot              bool
	SaleStart        int64
	Priority         int
	TargetIntervalMs int
	ReplicaFactor    int
}

type Node struct {
	ID           string
	Region       string
	Online       bool
	BudgetRPS    float64
	BlockedUntil time.Time
}

type Defaults struct {
	Now               time.Time
	DefaultIntervalMs int
	HotIntervalMs     int
	DefaultReplica    int
	NodeBudgetRPS     float64
	// HotMaxReplica caps how many nodes a single hot project fans out to.
	// 0 means no cap: hot projects spread across all healthy nodes (divided
	// fairly among the enabled hot projects).
	HotMaxReplica int
}

func Plan(projects []Project, nodes []Node, defaults Defaults) map[string][]protocol.Assignment {
	healthy := healthyNodes(nodes, defaults)
	out := make(map[string][]protocol.Assignment, len(healthy))
	for _, n := range healthy {
		out[n.ID] = nil
	}
	if len(healthy) == 0 {
		return out
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Priority != projects[j].Priority {
			return projects[i].Priority > projects[j].Priority
		}
		return projects[i].ID < projects[j].ID
	})

	// Count enabled hot projects so each fans out over a fair share of the pool.
	hotCount := 0
	for _, p := range projects {
		if !p.Enabled || p.ID <= 0 {
			continue
		}
		if intervalFor(p, defaults) <= 0 {
			continue
		}
		if isHot(p, defaults.Now) {
			hotCount++
		}
	}

	load := make(map[string]float64, len(healthy))
	for _, p := range projects {
		if !p.Enabled || p.ID <= 0 {
			continue
		}
		interval := intervalFor(p, defaults)
		if interval <= 0 {
			continue
		}
		replicas := replicaFor(p, defaults, len(healthy), interval, hotCount)
		if replicas <= 0 {
			continue
		}
		cost := 1000.0 / float64(interval)
		candidates := append([]Node(nil), healthy...)
		sort.Slice(candidates, func(i, j int) bool {
			if load[candidates[i].ID] != load[candidates[j].ID] {
				return load[candidates[i].ID] < load[candidates[j].ID]
			}
			if candidates[i].Region != candidates[j].Region {
				return candidates[i].Region < candidates[j].Region
			}
			return candidates[i].ID < candidates[j].ID
		})
		assigned := make([]Node, 0, replicas)
		usedRegion := map[string]bool{}
		for _, preferRegionSpread := range []bool{true, false} {
			for _, n := range candidates {
				if len(assigned) >= replicas {
					break
				}
				if containsNode(assigned, n.ID) {
					continue
				}
				if preferRegionSpread && n.Region != "" && usedRegion[n.Region] {
					continue
				}
				budget := budgetFor(n, defaults)
				if load[n.ID]+cost > budget+1e-9 {
					continue
				}
				assigned = append(assigned, n)
				if n.Region != "" {
					usedRegion[n.Region] = true
				}
			}
		}
		base := projectPhase(p.ID)
		for i, n := range assigned {
			k := len(assigned)
			// Spread replicas of THIS project evenly across the interval grid, then
			// shift the whole set by a per-project hash. Without the shift, every
			// single-replica project gets replica-phase 0 and aligns to the same
			// absolute grid boundary — bunching requests to Bilibili. The hash
			// de-syncs different projects while keeping replicas of one project
			// evenly staggered relative to each other.
			replicaPhase := int(math.Round(float64(i) * float64(DefaultPhaseMod) / float64(k)))
			phase := (replicaPhase + base) % DefaultPhaseMod
			if phase >= DefaultPhaseMod {
				phase = DefaultPhaseMod - 1
			}
			out[n.ID] = append(out[n.ID], protocol.Assignment{
				ProjectID:  p.ID,
				Phase:      phase,
				PhaseMod:   DefaultPhaseMod,
				IntervalMs: interval,
			})
			load[n.ID] += cost
		}
	}
	for nodeID := range out {
		sort.Slice(out[nodeID], func(i, j int) bool {
			return out[nodeID][i].ProjectID < out[nodeID][j].ProjectID
		})
	}
	return out
}

func healthyNodes(nodes []Node, defaults Defaults) []Node {
	now := defaults.Now
	if now.IsZero() {
		now = time.Now()
	}
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.ID == "" || !n.Online {
			continue
		}
		if !n.BlockedUntil.IsZero() && now.Before(n.BlockedUntil) {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func intervalFor(p Project, d Defaults) int {
	if p.TargetIntervalMs > 0 {
		return p.TargetIntervalMs
	}
	if isHot(p, d.Now) {
		if d.HotIntervalMs > 0 {
			return d.HotIntervalMs
		}
		return 10000
	}
	if d.DefaultIntervalMs > 0 {
		return d.DefaultIntervalMs
	}
	return 30000
}

func replicaFor(p Project, d Defaults, nodeCount, interval, hotCount int) int {
	if p.ReplicaFactor > 0 {
		return min(p.ReplicaFactor, nodeCount)
	}
	if isHot(p, d.Now) {
		// Floor keeps the old behavior so the phase-staggered effective refresh
		// stays <=1s; share fans the project out across the full healthy pool,
		// divided fairly among the enabled hot projects.
		floor := max(1, int(math.Ceil(float64(interval)/1000.0)))
		share := nodeCount
		if hotCount > 1 {
			share = nodeCount / hotCount
		}
		replicas := max(floor, share)
		if d.HotMaxReplica > 0 {
			replicas = min(replicas, d.HotMaxReplica)
		}
		return min(replicas, nodeCount)
	}
	replicas := d.DefaultReplica
	if replicas <= 0 {
		replicas = 1
	}
	return min(replicas, nodeCount)
}

func isHot(p Project, now time.Time) bool {
	if p.Hot {
		return true
	}
	if p.SaleStart <= 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	delta := time.Unix(p.SaleStart, 0).Sub(now)
	return delta > 0 && delta < 5*time.Minute
}

func budgetFor(n Node, d Defaults) float64 {
	if n.BudgetRPS > 0 {
		return n.BudgetRPS
	}
	if d.NodeBudgetRPS > 0 {
		return d.NodeBudgetRPS
	}
	return 0.9
}

func containsNode(nodes []Node, id string) bool {
	for _, n := range nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
