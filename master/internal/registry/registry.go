package registry

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/shared/protocol"
)

type Sender interface {
	Send(context.Context, any) error
	Close()
}

type Node struct {
	NodeID       string
	NodeName     string
	Region       string
	Online       bool
	InFlight     int
	SoftLimited  int
	HardBlocked  int
	LastSeen     int64
	SkewMs       int64
	BlockedUntil time.Time
	BudgetRPS    float64
	BudgetUntil  time.Time
	Assignments  []protocol.Assignment
	sender       Sender
}

type Registry struct {
	mu    sync.Mutex
	nodes map[string]*Node
}

func New() *Registry {
	return &Registry{nodes: map[string]*Node{}}
}

func (r *Registry) Register(msg protocol.Register, sender Sender) (model.Node, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().Unix()
	n, ok := r.nodes[msg.NodeID]
	if !ok {
		n = &Node{NodeID: msg.NodeID}
		r.nodes[msg.NodeID] = n
	}
	wasOnline := n.Online
	if n.sender != sender {
		if n.sender != nil {
			n.sender.Close()
		}
		n.Assignments = nil
	}
	n.NodeName = msg.NodeName
	n.Region = msg.Region
	n.Online = true
	n.LastSeen = now
	n.sender = sender
	return toModel(n), !wasOnline
}

func (r *Registry) Heartbeat(msg protocol.Heartbeat) model.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.ensure(msg.NodeID)
	n.Online = true
	// Stamp LastSeen with the master's receive time, never the node-supplied
	// msg.Ts: Reap() compares LastSeen against a master-clock cutoff, so storing a
	// node-clock value would be a cross-clock comparison — a node whose wall clock
	// drifts past the timeout would be wrongly reaped (or, if ahead, never reaped).
	// msg.Ts is kept only as a skew diagnostic surfaced via node_status.
	now := time.Now()
	n.LastSeen = now.Unix()
	if msg.Ts > 0 {
		n.SkewMs = now.UnixMilli() - msg.Ts*1000
	}
	n.InFlight = msg.InFlight
	n.SoftLimited = msg.SoftLimited
	n.HardBlocked = msg.HardBlocked
	return toModel(n)
}

func (r *Registry) RateEvent(msg protocol.RateEvent, hardCooldown time.Duration, defaultBudgetRPS float64) model.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.ensure(msg.NodeID)
	n.Online = true
	n.LastSeen = time.Now().Unix()
	switch msg.Kind {
	case protocol.RateSoftLimited:
		n.SoftLimited++
		if defaultBudgetRPS <= 0 {
			defaultBudgetRPS = 0.9
		}
		n.BudgetRPS = defaultBudgetRPS * 0.5
		if n.BudgetRPS < 0.1 {
			n.BudgetRPS = 0.1
		}
		n.BudgetUntil = time.Now().Add(time.Minute)
	case protocol.RateHardBlocked:
		n.HardBlocked++
		if hardCooldown <= 0 {
			hardCooldown = 10 * time.Minute
		}
		n.BlockedUntil = time.Now().Add(hardCooldown)
		n.BudgetRPS = 0
		n.BudgetUntil = time.Time{}
		n.Assignments = nil
	}
	return toModel(n)
}

func (r *Registry) MarkDisconnected(nodeID string) (model.Node, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.nodes[nodeID]
	if !ok {
		return model.Node{}, false
	}
	wasOnline := n.Online
	n.Online = false
	n.sender = nil
	return toModel(n), wasOnline
}

func (r *Registry) Reap(timeout time.Duration) []model.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	if timeout <= 0 {
		timeout = 36 * time.Second
	}
	cutoff := time.Now().Add(-timeout).Unix()
	var changed []model.Node
	for _, n := range r.nodes {
		if n.Online && n.LastSeen > 0 && n.LastSeen < cutoff {
			n.Online = false
			if n.sender != nil {
				n.sender.Close()
				n.sender = nil
			}
			changed = append(changed, toModel(n))
		}
	}
	return changed
}

func (r *Registry) SetAssignments(assignments map[string][]protocol.Assignment) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for nodeID, as := range assignments {
		n := r.ensure(nodeID)
		n.Assignments = append([]protocol.Assignment(nil), as...)
		sortAssignments(n.Assignments)
	}
}

func (r *Registry) SendAssignments(ctx context.Context, assignments map[string][]protocol.Assignment) error {
	r.mu.Lock()
	targets := make(map[string]struct {
		sender Sender
		items  []protocol.Assignment
	}, len(assignments))
	for _, n := range r.nodes {
		as := assignments[n.NodeID]
		normalized := append([]protocol.Assignment(nil), as...)
		sortAssignments(normalized)
		if assignmentsEqual(n.Assignments, normalized) {
			continue
		}
		n.Assignments = normalized
		if n.Online && n.sender != nil {
			targets[n.NodeID] = struct {
				sender Sender
				items  []protocol.Assignment
			}{sender: n.sender, items: append([]protocol.Assignment(nil), normalized...)}
		}
	}
	r.mu.Unlock()

	var err error
	for nodeID, target := range targets {
		msg := protocol.Assign{
			Type:        protocol.TypeAssign,
			Version:     protocol.Version,
			NodeID:      nodeID,
			Assignments: target.items,
		}
		if sendErr := target.sender.Send(ctx, msg); sendErr != nil {
			err = errors.Join(err, sendErr)
		}
	}
	return err
}

func (r *Registry) BroadcastConfig(ctx context.Context, msg protocol.Config) {
	r.mu.Lock()
	var senders []Sender
	for _, n := range r.nodes {
		if n.Online && n.sender != nil {
			senders = append(senders, n.sender)
		}
	}
	r.mu.Unlock()
	for _, sender := range senders {
		_ = sender.Send(ctx, msg)
	}
}

func (r *Registry) Snapshot() []model.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		clearExpiredBudget(n, time.Now())
		out = append(out, toModel(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

func (r *Registry) ensure(nodeID string) *Node {
	n, ok := r.nodes[nodeID]
	if !ok {
		n = &Node{NodeID: nodeID}
		r.nodes[nodeID] = n
	}
	return n
}

func toModel(n *Node) model.Node {
	assignments := make([]int64, 0, len(n.Assignments))
	for _, a := range n.Assignments {
		assignments = append(assignments, a.ProjectID)
	}
	sort.Slice(assignments, func(i, j int) bool { return assignments[i] < assignments[j] })
	return model.Node{
		NodeID:       n.NodeID,
		NodeName:     n.NodeName,
		Region:       n.Region,
		Online:       n.Online,
		InFlight:     n.InFlight,
		SoftLimited:  n.SoftLimited,
		HardBlocked:  n.HardBlocked,
		LastSeen:     n.LastSeen,
		SkewMs:       n.SkewMs,
		BlockedUntil: n.BlockedUntil,
		BudgetRPS:    n.BudgetRPS,
		Assignments:  assignments,
	}
}

func clearExpiredBudget(n *Node, now time.Time) {
	if n.BudgetRPS > 0 && !n.BudgetUntil.IsZero() && now.After(n.BudgetUntil) {
		n.BudgetRPS = 0
		n.BudgetUntil = time.Time{}
	}
}

func sortAssignments(assignments []protocol.Assignment) {
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].ProjectID != assignments[j].ProjectID {
			return assignments[i].ProjectID < assignments[j].ProjectID
		}
		if assignments[i].IntervalMs != assignments[j].IntervalMs {
			return assignments[i].IntervalMs < assignments[j].IntervalMs
		}
		return assignments[i].Phase < assignments[j].Phase
	})
}

func assignmentsEqual(a, b []protocol.Assignment) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]protocol.Assignment(nil), a...)
	bb := append([]protocol.Assignment(nil), b...)
	sortAssignments(aa)
	sortAssignments(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
