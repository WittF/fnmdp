package assign

import (
	"fmt"
	"testing"
	"time"

	"github.com/WittF/fnmdp/shared/protocol"
)

func TestPlanPhasesAndBudget(t *testing.T) {
	projects := []Project{
		{ID: 1, Enabled: true, TargetIntervalMs: 10000, ReplicaFactor: 3},
		{ID: 2, Enabled: true, TargetIntervalMs: 30000, ReplicaFactor: 1},
	}
	nodes := []Node{
		{ID: "a", Online: true, BudgetRPS: 0.2, Region: "sg"},
		{ID: "b", Online: true, BudgetRPS: 0.2, Region: "jp"},
		{ID: "c", Online: true, BudgetRPS: 0.2, Region: "us"},
	}
	plan := Plan(projects, nodes, Defaults{Now: time.Unix(1000, 0), NodeBudgetRPS: 0.2})

	var phases []int
	for _, assignments := range plan {
		for _, a := range assignments {
			if a.ProjectID == 1 {
				phases = append(phases, a.Phase)
			}
		}
	}
	if len(phases) != 3 {
		t.Fatalf("project replicas = %d, want 3: %#v", len(phases), plan)
	}
	seen := map[int]bool{}
	for _, phase := range phases {
		seen[phase] = true
	}
	// Replicas stay evenly spread (333 apart), shifted by the project's hash base.
	base := projectPhase(1)
	for _, off := range []int{0, 333, 667} {
		want := (off + base) % DefaultPhaseMod
		if !seen[want] {
			t.Fatalf("phases = %v, missing %d (base=%d)", phases, want, base)
		}
	}
	for nodeID, assignments := range plan {
		var load float64
		for _, a := range assignments {
			load += 1000.0 / float64(a.IntervalMs)
		}
		if load > 0.2+1e-9 {
			t.Fatalf("node %s load %.3f exceeds budget", nodeID, load)
		}
	}
}

func makeNodes(n int) []Node {
	nodes := make([]Node, 0, n)
	for i := 0; i < n; i++ {
		nodes = append(nodes, Node{
			ID:        fmt.Sprintf("node-%02d", i),
			Online:    true,
			BudgetRPS: 0.9,
			Region:    "region-a",
		})
	}
	return nodes
}

func projectCount(plan map[string][]protocol.Assignment, projectID int64) int {
	count := 0
	for _, assignments := range plan {
		for _, a := range assignments {
			if a.ProjectID == projectID {
				count++
			}
		}
	}
	return count
}

// Hot projects with no explicit ReplicaFactor must fan out across ALL healthy
// nodes, divided fairly among the enabled hot projects.
func TestPlanHotFanOutAllNodes(t *testing.T) {
	nodes := makeNodes(40)
	projects := []Project{
		{ID: 1, Enabled: true, Hot: true},
		{ID: 2, Enabled: true, Hot: true},
	}
	plan := Plan(projects, nodes, Defaults{Now: time.Unix(1000, 0), HotIntervalMs: 10000, NodeBudgetRPS: 0.9})

	for _, n := range nodes {
		if len(plan[n.ID]) == 0 {
			t.Fatalf("node %s got no assignment; not all nodes used", n.ID)
		}
	}
	if c := projectCount(plan, 1); c != 20 {
		t.Fatalf("project 1 replicas = %d, want 20", c)
	}
	if c := projectCount(plan, 2); c != 20 {
		t.Fatalf("project 2 replicas = %d, want 20", c)
	}
}

// Uneven node count: floor division leaves a spare node, never over-subscribes.
func TestPlanHotFairDivisionUneven(t *testing.T) {
	nodes := makeNodes(41)
	projects := []Project{
		{ID: 1, Enabled: true, Hot: true},
		{ID: 2, Enabled: true, Hot: true},
	}
	plan := Plan(projects, nodes, Defaults{Now: time.Unix(1000, 0), HotIntervalMs: 10000, NodeBudgetRPS: 0.9})

	if c := projectCount(plan, 1); c != 20 {
		t.Fatalf("project 1 replicas = %d, want 20 (41/2 floor)", c)
	}
	if c := projectCount(plan, 2); c != 20 {
		t.Fatalf("project 2 replicas = %d, want 20 (41/2 floor)", c)
	}
	for nodeID, assignments := range plan {
		var load float64
		for _, a := range assignments {
			load += 1000.0 / float64(a.IntervalMs)
		}
		if load > 0.9+1e-9 {
			t.Fatalf("node %s load %.3f exceeds budget", nodeID, load)
		}
	}
}

// HOT_MAX_REPLICA caps fan-out so the operator can trade refresh speed for
// total request volume.
func TestPlanHotMaxReplicaCap(t *testing.T) {
	nodes := makeNodes(40)
	projects := []Project{
		{ID: 1, Enabled: true, Hot: true},
		{ID: 2, Enabled: true, Hot: true},
	}
	plan := Plan(projects, nodes, Defaults{Now: time.Unix(1000, 0), HotIntervalMs: 10000, NodeBudgetRPS: 0.9, HotMaxReplica: 12})

	if c := projectCount(plan, 1); c != 12 {
		t.Fatalf("project 1 replicas = %d, want 12 (capped)", c)
	}
	if c := projectCount(plan, 2); c != 12 {
		t.Fatalf("project 2 replicas = %d, want 12 (capped)", c)
	}
}

// An explicit ReplicaFactor override wins over the hot fair-share fan-out.
func TestPlanReplicaFactorOverrideHonored(t *testing.T) {
	nodes := makeNodes(40)
	projects := []Project{{ID: 1, Enabled: true, Hot: true, ReplicaFactor: 5}}
	plan := Plan(projects, nodes, Defaults{Now: time.Unix(1000, 0), HotIntervalMs: 10000, NodeBudgetRPS: 0.9})

	if c := projectCount(plan, 1); c != 5 {
		t.Fatalf("project 1 replicas = %d, want 5 (override)", c)
	}
}

// Phase staggering stays evenly spread at high replica counts.
func TestPlanHotPhaseSpreadHighK(t *testing.T) {
	nodes := makeNodes(20)
	projects := []Project{{ID: 1, Enabled: true, Hot: true}}
	plan := Plan(projects, nodes, Defaults{Now: time.Unix(1000, 0), HotIntervalMs: 10000, NodeBudgetRPS: 0.9})

	seen := map[int]bool{}
	for _, assignments := range plan {
		for _, a := range assignments {
			if a.ProjectID == 1 {
				seen[a.Phase] = true
			}
		}
	}
	if len(seen) != 20 {
		t.Fatalf("distinct phases = %d, want 20: %v", len(seen), seen)
	}
	base := projectPhase(1)
	for i := 0; i < 20; i++ {
		want := (i*50 + base) % DefaultPhaseMod // round(i*1000/20), shifted by hash base
		if !seen[want] {
			t.Fatalf("missing phase %d; phases = %v (base=%d)", want, seen, base)
		}
	}
}

// Different single-replica projects must NOT all collapse to phase 0 — the
// per-project hash base de-syncs them so they don't bunch at the same grid edge.
func TestPlanCrossProjectPhaseDesync(t *testing.T) {
	projects := []Project{
		{ID: 1001701, Enabled: true, TargetIntervalMs: 30000, ReplicaFactor: 1},
		{ID: 1002002, Enabled: true, TargetIntervalMs: 30000, ReplicaFactor: 1},
		{ID: 1003003, Enabled: true, TargetIntervalMs: 30000, ReplicaFactor: 1},
	}
	nodes := makeNodes(1) // all land on one node, same interval → would all be phase 0 before
	plan := Plan(projects, nodes, Defaults{Now: time.Unix(1000, 0), NodeBudgetRPS: 0.9})

	phaseByProject := map[int64]int{}
	for _, assignments := range plan {
		for _, a := range assignments {
			phaseByProject[a.ProjectID] = a.Phase
		}
	}
	if len(phaseByProject) != 3 {
		t.Fatalf("assigned projects = %d, want 3: %v", len(phaseByProject), phaseByProject)
	}
	distinct := map[int]bool{}
	for _, p := range phaseByProject {
		distinct[p] = true
	}
	if len(distinct) != 3 {
		t.Fatalf("phases not de-synced across projects: %v", phaseByProject)
	}
}

func TestPlanSkipsBlockedNode(t *testing.T) {
	nodes := []Node{
		{ID: "blocked", Online: true, BudgetRPS: 1, BlockedUntil: time.Unix(2000, 0)},
		{ID: "healthy", Online: true, BudgetRPS: 1},
	}
	plan := Plan([]Project{{ID: 1, Enabled: true, TargetIntervalMs: 10000, ReplicaFactor: 2}}, nodes, Defaults{Now: time.Unix(1000, 0)})
	if len(plan["blocked"]) != 0 {
		t.Fatalf("blocked node got assignments: %#v", plan["blocked"])
	}
	if len(plan["healthy"]) != 1 {
		t.Fatalf("healthy assignments = %#v", plan["healthy"])
	}
}
