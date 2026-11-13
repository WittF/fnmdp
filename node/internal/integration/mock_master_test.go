package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/WittF/fnmdp/node/internal/assign"
	"github.com/WittF/fnmdp/node/internal/collector"
	"github.com/WittF/fnmdp/node/internal/config"
	"github.com/WittF/fnmdp/node/internal/ratelimit"
	"github.com/WittF/fnmdp/node/internal/scheduler"
	"github.com/WittF/fnmdp/node/internal/wsclient"
	"github.com/WittF/fnmdp/shared/getv2"
	"github.com/WittF/fnmdp/shared/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type fakeFetcher struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeFetcher) Fetch(ctx context.Context, projectID int64) (*getv2.RawResponse, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return rawWithSku(projectID, 10, 1, 2, true, 8800, "在售"), nil
}

type minIntervalSource struct {
	runtime *config.Runtime
}

func (s minIntervalSource) MinInterval() time.Duration {
	return s.runtime.Snapshot().MinInterval
}

func TestMockMasterAssignToPollResult(t *testing.T) {
	gotPoll := make(chan protocol.PollResult, 1)
	gotRegister := make(chan protocol.Register, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()

		var reg protocol.Register
		if err := wsjson.Read(context.Background(), conn, &reg); err != nil {
			return
		}
		gotRegister <- reg
		_ = wsjson.Write(context.Background(), conn, protocol.Config{
			Type:           protocol.TypeConfig,
			Version:        protocol.Version,
			MinIntervalMs:  1,
			SoftBackoffMs:  1,
			HardCooldownMs: 50,
		})
		_ = wsjson.Write(context.Background(), conn, protocol.Assign{
			Type:    protocol.TypeAssign,
			Version: protocol.Version,
			NodeID:  "node-1",
			Assignments: []protocol.Assignment{{
				ProjectID:  100,
				Phase:      0,
				PhaseMod:   1,
				IntervalMs: 5,
			}},
		})

		for {
			_, payload, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			var envelope struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(payload, &envelope) != nil {
				continue
			}
			if envelope.Type != protocol.TypePollResult {
				continue
			}
			var pr protocol.PollResult
			if json.Unmarshal(payload, &pr) == nil {
				gotPoll <- pr
			}
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := config.Config{
		MasterURL:         server.URL,
		NodeID:            "node-1",
		NodeName:          "node-one",
		Region:            "test",
		HeartbeatInterval: time.Hour,
	}
	runtime := config.NewRuntime()
	runtime.SetHeartbeatInterval(time.Hour)
	rate := ratelimit.New(time.Millisecond, 50*time.Millisecond)
	store := assign.NewStore()
	ws := wsclient.New(cfg, store, runtime, rate)
	coll := collector.New(cfg.NodeID, &fakeFetcher{}, ws, rate, func() collector.ClientConfigSnapshot {
		snap := runtime.Snapshot()
		return collector.ClientConfigSnapshot{
			UserAgent:    snap.UserAgent,
			SoftBackoff:  snap.SoftBackoff,
			HardCooldown: snap.HardCooldown,
		}
	})
	sched := scheduler.New(store, minIntervalSource{runtime: runtime}, coll.Poll)
	sched.SetTickForTest(time.Millisecond)

	go func() { _ = ws.Run(ctx) }()
	go func() { _ = sched.Run(ctx) }()

	select {
	case reg := <-gotRegister:
		if reg.Type != protocol.TypeRegister || reg.NodeID != "node-1" {
			t.Fatalf("register = %+v", reg)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for register")
	}

	select {
	case pr := <-gotPoll:
		if pr.Type != protocol.TypePollResult || pr.ProjectID != 100 || len(pr.Diff) != 1 {
			t.Fatalf("poll_result = %+v", pr)
		}
		if pr.Project == nil || pr.Project.Name != "Project" {
			t.Fatalf("poll_result project = %+v", pr.Project)
		}
		if len(pr.Screens) != 1 || pr.Screens[0].Name != "Day 1" {
			t.Fatalf("poll_result screens = %+v", pr.Screens)
		}
		if pr.Diff[0].FlagText != "在售" || pr.Diff[0].New == nil {
			t.Fatalf("poll_result diff = %+v", pr.Diff)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for poll_result")
	}
}

func rawWithSku(projectID, screenID, skuID int64, flag int, clickable bool, price int, flagText string) *getv2.RawResponse {
	return &getv2.RawResponse{
		Errno: 0,
		Data: &getv2.RawData{
			ID:             projectID,
			Name:           "Project",
			SaleFlagNumber: intp(2),
			Status:         intp(1),
			IsSale:         intp(1),
			CurrentTime:    int64p(1750100000),
			ScreenList: []getv2.RawScreen{{
				ID:        screenID,
				Name:      "Day 1",
				SaleFlag:  &getv2.RawSaleFlag{Number: intp(2), DisplayName: "在售"},
				Clickable: boolp(true),
				TicketList: []getv2.RawTicket{{
					ID:        skuID,
					Desc:      "VIP",
					Price:     intp(price),
					Clickable: boolp(clickable),
					SaleFlag:  &getv2.RawSaleFlag{Number: intp(flag), DisplayName: flagText},
				}},
			}},
		},
	}
}

func intp(v int) *int { return &v }

func int64p(v int64) *int64 { return &v }

func boolp(v bool) *bool { return &v }
