package wsclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/WittF/fnmdp/node/internal/assign"
	"github.com/WittF/fnmdp/node/internal/config"
	"github.com/WittF/fnmdp/node/internal/ratelimit"
	"github.com/WittF/fnmdp/shared/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type Client struct {
	cfg     config.Config
	store   *assign.Store
	runtime *config.Runtime
	rate    *ratelimit.State
	writeQ  chan any
}

func New(cfg config.Config, store *assign.Store, runtime *config.Runtime, rate *ratelimit.State) *Client {
	return &Client{
		cfg:     cfg,
		store:   store,
		runtime: runtime,
		rate:    rate,
		writeQ:  make(chan any, 2048),
	}
}

func (c *Client) Send(ctx context.Context, msg any) error {
	select {
	case c.writeQ <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("wsclient: write queue full")
	}
}

func (c *Client) Run(ctx context.Context) error {
	backoff := time.Second
	for {
		if err := c.connectOnce(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		wait := withJitter(backoff)
		if backoff < 30*time.Second {
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) connectOnce(ctx context.Context) error {
	header := http.Header{}
	if c.cfg.NodeToken != "" {
		header.Set("Authorization", "Bearer "+c.cfg.NodeToken)
	}
	conn, _, err := websocket.Dial(ctx, c.cfg.MasterURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := wsjson.Write(connCtx, conn, c.registerMessage()); err != nil {
		return err
	}

	errCh := make(chan error, 3)
	go func() { errCh <- c.readLoop(connCtx, conn) }()
	go func() { errCh <- c.writeLoop(connCtx, conn) }()
	go func() { errCh <- c.heartbeatLoop(connCtx) }()

	err = <-errCh
	cancel()
	_ = conn.Close(websocket.StatusNormalClosure, "")
	return err
}

func (c *Client) registerMessage() protocol.Register {
	return protocol.Register{
		Type:         protocol.TypeRegister,
		Version:      protocol.Version,
		NodeID:       c.cfg.NodeID,
		NodeName:     c.cfg.NodeName,
		Region:       c.cfg.Region,
		Capabilities: []string{"mall_primary", "getv2_fallback", "utls_persona", "l1_dedup", "rate_control", "standalone"},
	}
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			continue
		}
		switch envelope.Type {
		case protocol.TypeAssign:
			var msg protocol.Assign
			if json.Unmarshal(payload, &msg) == nil && c.forThisNode(msg.NodeID) {
				c.store.Replace(msg.Assignments)
			}
		case protocol.TypeAssignDelta:
			var msg protocol.AssignDelta
			if json.Unmarshal(payload, &msg) == nil && c.forThisNode(msg.NodeID) {
				c.store.ApplyDelta(msg.Added, msg.Removed)
			}
		case protocol.TypeConfig:
			var msg protocol.Config
			if json.Unmarshal(payload, &msg) == nil {
				c.runtime.ApplyConfig(msg)
			}
		}
	}
}

func (c *Client) forThisNode(nodeID string) bool {
	return nodeID == "" || nodeID == c.cfg.NodeID
}

func (c *Client) writeLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-c.writeQ:
			if err := wsjson.Write(ctx, conn, msg); err != nil {
				return err
			}
		}
	}
}

func (c *Client) heartbeatLoop(ctx context.Context) error {
	for {
		snap := c.runtime.Snapshot()
		interval := snap.HeartbeatEvery
		if interval <= 0 {
			interval = config.DefaultHeartbeatInterval
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
			rate := c.rate.Snapshot(time.Now())
			if err := c.Send(ctx, protocol.Heartbeat{
				Type:        protocol.TypeHeartbeat,
				Version:     protocol.Version,
				NodeID:      c.cfg.NodeID,
				Ts:          time.Now().Unix(),
				InFlight:    rate.InFlight,
				SoftLimited: rate.SoftLimited,
				HardBlocked: rate.HardBlocked,
			}); err != nil {
				return err
			}
		}
	}
}

func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Second
	}
	jitterCap := d / 4
	if jitterCap <= 0 {
		return d
	}
	return d + time.Duration(time.Now().UnixNano()%int64(jitterCap))
}
