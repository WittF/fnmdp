package collector

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	nodededup "github.com/WittF/fnmdp/node/internal/dedup"
	"github.com/WittF/fnmdp/node/internal/ratelimit"
	"github.com/WittF/fnmdp/shared/getv2"
	"github.com/WittF/fnmdp/shared/protocol"
)

type Fetcher interface {
	Fetch(context.Context, int64) (*getv2.RawResponse, error)
}

type sourceFetcher interface {
	FetchWithSource(context.Context, int64) (*getv2.RawResponse, string, error)
}

type managedRateFetcher interface {
	sourceFetcher
	ConfigureRate(softBackoff, hardCooldown time.Duration)
}

type Sink interface {
	Send(context.Context, any) error
}

type ConfigFunc func() ClientConfigSnapshot

type ClientConfigSnapshot struct {
	UserAgent    string
	SoftBackoff  time.Duration
	HardCooldown time.Duration
	// Keepalive forces a floor poll_result at least this often even when the SKU
	// state is unchanged. 0 disables the floor (pure diff-only).
	Keepalive time.Duration
}

type Collector struct {
	nodeID  string
	fetcher Fetcher
	client  *getv2.Client
	dedup   *nodededup.L1
	rate    *ratelimit.State
	sink    Sink
	config  ConfigFunc

	mu           sync.Mutex
	lastReported map[int64]time.Time // per-project last time we actually sent a poll_result
}

func New(nodeID string, fetcher Fetcher, sink Sink, rate *ratelimit.State, config ConfigFunc) *Collector {
	if config == nil {
		config = func() ClientConfigSnapshot { return ClientConfigSnapshot{} }
	}
	c := &Collector{
		nodeID:       nodeID,
		fetcher:      fetcher,
		dedup:        nodededup.New(),
		rate:         rate,
		sink:         sink,
		config:       config,
		lastReported: map[int64]time.Time{},
	}
	if gc, ok := fetcher.(*getv2.Client); ok {
		c.client = gc
	}
	return c
}

func (c *Collector) Poll(ctx context.Context, projectID int64) {
	cfg := c.config()
	managed := false
	if mf, ok := c.fetcher.(managedRateFetcher); ok {
		mf.ConfigureRate(cfg.SoftBackoff, cfg.HardCooldown)
		if uf, ok := c.fetcher.(userAgentConfigurer); ok {
			uf.ConfigureUserAgent(cfg.UserAgent)
		}
		managed = true
	} else if uf, ok := c.fetcher.(userAgentConfigurer); ok {
		uf.ConfigureUserAgent(cfg.UserAgent)
	} else if c.client != nil && cfg.UserAgent != "" {
		c.client.UserAgent = cfg.UserAgent
	}

	raw, source, err := c.fetch(ctx, projectID, cfg)
	now := time.Now()
	if err != nil {
		c.handleError(ctx, projectID, err, now, !managed)
		return
	}
	if !managed {
		c.rate.RecordSuccess(now)
	}

	ps := getv2.Normalize(raw)
	diff := c.dedup.Diff(ps)
	// Diff() always advances L1 state; we still gate whether to SEND. Unchanged polls
	// are normally suppressed (diff-only), but a floor/keepalive report goes out at
	// least every cfg.Keepalive so the master can keep per-project liveness fresh and
	// the frontend timeline stays continuous instead of showing a no-data gap.
	if !diff.Changed() && !c.keepaliveDue(projectID, now, cfg.Keepalive) {
		return
	}
	msg := protocol.PollResult{
		Type:      protocol.TypePollResult,
		Version:   protocol.Version,
		NodeID:    c.nodeID,
		ProjectID: projectID,
		Source:    source,
		ServerTs:  ps.ServerTs,
		FetchedAt: now.Unix(),
	}
	// A floor report carries an empty diff (Project/Screens nil): it proves "polled,
	// unchanged" without re-sending state or triggering project/screen event paths.
	if diff.Changed() {
		msg.Diff = diff.Skus
		msg.Project = &diff.Project
		msg.Screens = diff.Screens
	}
	_ = c.sink.Send(ctx, msg)
	c.mu.Lock()
	c.lastReported[projectID] = now
	c.mu.Unlock()
}

// keepaliveDue reports whether the floor interval has elapsed since this project
// last had any poll_result sent. keepalive<=0 disables the floor entirely.
func (c *Collector) keepaliveDue(projectID int64, now time.Time, keepalive time.Duration) bool {
	if keepalive <= 0 {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	last, ok := c.lastReported[projectID]
	return !ok || now.Sub(last) >= keepalive
}

func (c *Collector) fetch(ctx context.Context, projectID int64, cfg ClientConfigSnapshot) (*getv2.RawResponse, string, error) {
	if mf, ok := c.fetcher.(managedRateFetcher); ok {
		return mf.FetchWithSource(ctx, projectID)
	}
	c.rate.Configure(cfg.SoftBackoff, cfg.HardCooldown)
	if err := c.rate.Wait(ctx); err != nil {
		return nil, "", err
	}
	c.rate.Begin()
	defer c.rate.End()
	if sf, ok := c.fetcher.(sourceFetcher); ok {
		return sf.FetchWithSource(ctx, projectID)
	}
	raw, err := c.fetcher.Fetch(ctx, projectID)
	return raw, "getv2", err
}

func (c *Collector) handleError(ctx context.Context, projectID int64, err error, now time.Time, record bool) {
	var re *getv2.RateError
	switch {
	case errors.Is(err, getv2.ErrSoftLimited):
		if record {
			c.rate.RecordSoft(now)
		}
		status, sec := rateDetails(err, &re)
		_ = c.sink.Send(ctx, protocol.RateEvent{
			Type:         protocol.TypeRateEvent,
			Version:      protocol.Version,
			NodeID:       c.nodeID,
			ProjectID:    projectID,
			StatusCode:   status,
			Kind:         protocol.RateSoftLimited,
			SecRequestID: sec,
			Ts:           now.Unix(),
		})
	case errors.Is(err, getv2.ErrHardBlocked):
		if record {
			c.rate.RecordHard(now)
		}
		status, sec := rateDetails(err, &re)
		_ = c.sink.Send(ctx, protocol.RateEvent{
			Type:         protocol.TypeRateEvent,
			Version:      protocol.Version,
			NodeID:       c.nodeID,
			ProjectID:    projectID,
			StatusCode:   status,
			Kind:         protocol.RateHardBlocked,
			SecRequestID: sec,
			Ts:           now.Unix(),
		})
	default:
		log.Printf("collector poll error: node_id=%s project_id=%d err=%v", c.nodeID, projectID, err)
	}
}

func rateDetails(err error, re **getv2.RateError) (int, string) {
	if errors.As(err, re) {
		return (*re).StatusCode, (*re).SecRequestID
	}
	return 0, ""
}
