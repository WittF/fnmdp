package store

import (
	"context"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
)

const (
	DefaultEventLimit   = 100
	MaxEventLimit       = 200
	MaxHistoryLimit     = 2000
	MaxTrajectoryPoints = 2000
)

type EventFilter struct {
	ProjectID       int64
	ScreenID        int64
	SkuID           int64
	EventType       string
	Severity        string
	SinceTs         int64     // unix seconds, filter created_at >= SinceTs (0 = no lower bound)
	UntilTs         int64     // unix seconds, filter created_at <= UntilTs (0 = no upper bound)
	BeforeCreatedAt time.Time // keyset cursor: return rows older than (BeforeCreatedAt, BeforeID)
	BeforeID        int64
	Limit           int
	MaxLimit        int // optional per-call cap; 0 = MaxEventLimit
}

type TrajectoryQuery struct {
	ProjectID  int64
	ScreenID   int64
	SkuID      int64
	SinceMs    int64
	UntilMs    int64
	IntervalMs int64
}

type Store interface {
	EnsureProject(context.Context, int64) error
	UpsertProject(context.Context, model.Project) error
	UpsertScreen(context.Context, model.Screen) error
	DeleteScreen(context.Context, int64, int64) error
	UpsertSKU(context.Context, model.SKU) error
	DeleteSKU(context.Context, int64, int64, int64) error
	InsertEvent(context.Context, model.Event) error
	UpsertCoverage(context.Context, model.PollCoverage) error
	Coverage(context.Context, int64, int64, int64) ([]model.PollCoverage, error)
	PruneBefore(context.Context, int64) error
	Snapshot(context.Context) (model.Snapshot, error)
	Events(context.Context, EventFilter) ([]model.Event, error)
	Close()
}

func nowIfZero(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t
}

func eventLimit(f EventFilter) int {
	capLimit := f.MaxLimit
	if capLimit <= 0 {
		capLimit = MaxEventLimit
	}
	limit := f.Limit
	if f.Limit <= 0 {
		limit = DefaultEventLimit
	}
	if limit > capLimit {
		return capLimit
	}
	return limit
}
