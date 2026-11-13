package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/WittF/fnmdp/master/internal/registry"
	"github.com/WittF/fnmdp/shared/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type Callbacks struct {
	OnRegister   func(context.Context, protocol.Register, registry.Sender)
	OnHeartbeat  func(context.Context, protocol.Heartbeat)
	OnPollResult func(context.Context, protocol.PollResult)
	OnRateEvent  func(context.Context, protocol.RateEvent)
	OnDisconnect func(context.Context, string)
}

type Handler struct {
	NodeToken string
	BaseCtx   context.Context
	Callbacks Callbacks
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.NodeToken != "" && r.Header.Get("Authorization") != "Bearer "+h.NodeToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)
	ctx := r.Context()
	if h.BaseCtx != nil {
		go func() {
			select {
			case <-h.BaseCtx.Done():
				_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
			case <-ctx.Done():
			}
		}()
	}
	sender := newSender(conn)
	go sender.writeLoop(ctx)
	defer sender.Close()

	var nodeID string
	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			if nodeID != "" && h.Callbacks.OnDisconnect != nil {
				h.Callbacks.OnDisconnect(ctx, nodeID)
			}
			return
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			continue
		}
		switch strings.TrimSpace(envelope.Type) {
		case protocol.TypeRegister:
			var msg protocol.Register
			if json.Unmarshal(payload, &msg) == nil {
				nodeID = msg.NodeID
				if h.Callbacks.OnRegister != nil {
					h.Callbacks.OnRegister(ctx, msg, sender)
				}
			}
		case protocol.TypeHeartbeat:
			var msg protocol.Heartbeat
			if json.Unmarshal(payload, &msg) == nil && h.Callbacks.OnHeartbeat != nil {
				h.Callbacks.OnHeartbeat(ctx, msg)
			}
		case protocol.TypePollResult:
			var msg protocol.PollResult
			if json.Unmarshal(payload, &msg) == nil && h.Callbacks.OnPollResult != nil {
				h.Callbacks.OnPollResult(ctx, msg)
			}
		case protocol.TypeRateEvent:
			var msg protocol.RateEvent
			if json.Unmarshal(payload, &msg) == nil && h.Callbacks.OnRateEvent != nil {
				h.Callbacks.OnRateEvent(ctx, msg)
			}
		}
	}
}

type sender struct {
	conn   *websocket.Conn
	writeQ chan any
	done   chan struct{}
}

func newSender(conn *websocket.Conn) *sender {
	return &sender{
		conn:   conn,
		writeQ: make(chan any, 256),
		done:   make(chan struct{}),
	}
}

func (s *sender) Send(ctx context.Context, msg any) error {
	select {
	case s.writeQ <- msg:
		return nil
	case <-s.done:
		return errors.New("ingest: sender closed")
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("ingest: write queue full")
	}
}

func (s *sender) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
		_ = s.conn.Close(websocket.StatusNormalClosure, "")
	}
}

func (s *sender) writeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case msg := <-s.writeQ:
			if err := wsjson.Write(ctx, s.conn, msg); err != nil {
				s.Close()
				return
			}
		}
	}
}
