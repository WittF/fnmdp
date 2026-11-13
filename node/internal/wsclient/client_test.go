package wsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/WittF/fnmdp/node/internal/assign"
	"github.com/WittF/fnmdp/node/internal/config"
	"github.com/WittF/fnmdp/node/internal/ratelimit"
	"github.com/WittF/fnmdp/shared/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestReconnectRegistersAgain(t *testing.T) {
	registers := make(chan protocol.Register, 2)
	requests := make(chan string, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Header.Get("Authorization")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		var reg protocol.Register
		if err := wsjson.Read(context.Background(), conn, &reg); err != nil {
			return
		}
		registers <- reg
		_ = conn.Close(websocket.StatusNormalClosure, "test")
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cfg := config.Config{
		MasterURL:         server.URL,
		NodeID:            "node-1",
		NodeName:          "node-one",
		Region:            "test",
		NodeToken:         "tok",
		HeartbeatInterval: time.Hour,
	}
	rt := config.NewRuntime()
	rt.SetHeartbeatInterval(time.Hour)
	c := New(cfg, assign.NewStore(), rt, ratelimit.New(time.Millisecond, time.Second))
	go func() { _ = c.Run(ctx) }()

	for i := 0; i < 2; i++ {
		select {
		case got := <-registers:
			if got.Type != protocol.TypeRegister || got.NodeID != "node-1" || got.Version != protocol.Version {
				t.Fatalf("register = %+v", got)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for register")
		}
		select {
		case auth := <-requests:
			if auth != "Bearer tok" {
				t.Fatalf("authorization = %q", auth)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for request")
		}
	}
}
