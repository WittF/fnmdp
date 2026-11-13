package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/WittF/fnmdp/master/internal/config"
	"github.com/WittF/fnmdp/master/internal/dedup"
	masterstate "github.com/WittF/fnmdp/master/internal/state"
	"github.com/WittF/fnmdp/master/internal/store"
	"github.com/WittF/fnmdp/shared/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestIngestToLiveAndStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	st := store.NewMemory()
	engine, err := masterstate.New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(ctx, config.Config{
		NodeToken:            "token",
		DefaultIntervalMs:    10000,
		NodeBudgetRPS:        0.9,
		MinIntervalMs:        1,
		SoftBackoffMs:        1,
		HardCooldownMs:       50,
		HeartbeatTimeout:     time.Hour,
		DefaultReplicaFactor: 1,
	}, st, engine)
	if err := app.ConfigureProject(100, 0, 10000, 1, true); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(app.Handler(ctx))
	defer ts.Close()

	live, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/live", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer live.CloseNow()
	var snap protocol.Snapshot
	if err := wsjson.Read(ctx, live, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Type != protocol.TypeSnapshot {
		t.Fatalf("snapshot = %+v", snap)
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer token")
	node, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/ingest", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer node.CloseNow()
	if err := wsjson.Write(ctx, node, protocol.Register{
		Type:     protocol.TypeRegister,
		Version:  protocol.Version,
		NodeID:   "node-1",
		NodeName: "node-one",
		Region:   "test",
	}); err != nil {
		t.Fatal(err)
	}

	if err := wsjson.Write(ctx, node, pollResult(100, 10, 1, 4, false, 8800, 1000)); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, node, pollResult(100, 10, 1, 2, true, 8800, 1001)); err != nil {
		t.Fatal(err)
	}
	// Duplicate and stale reports should be idempotent/no-op after L3.
	if err := wsjson.Write(ctx, node, pollResult(100, 10, 1, 2, true, 8800, 1001)); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Write(ctx, node, pollResult(100, 10, 1, 1, false, 8800, 999)); err != nil {
		t.Fatal(err)
	}

	if ev := readLiveEvent(t, ctx, live, protocol.EventSkuRestock); ev.ProjectID != 100 || ev.SkuID != 1 {
		t.Fatalf("restock event = %+v", ev)
	}

	events, err := st.Events(ctx, store.EventFilter{ProjectID: 100, EventType: string(protocol.EventSkuRestock), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("restock events = %+v", events)
	}
	snapModel, err := st.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapModel.SKUs) != 1 || snapModel.SKUs[0].Flag != 2 || !snapModel.SKUs[0].Clickable {
		t.Fatalf("sku state = %+v", snapModel.SKUs)
	}
}

func TestIngestClosesOnAppContextCancel(t *testing.T) {
	appCtx, stop := context.WithCancel(context.Background())
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	st := store.NewMemory()
	engine, err := masterstate.New(ctx, st, dedup.NewMemoryWindow(), nil)
	if err != nil {
		t.Fatal(err)
	}
	app := New(appCtx, config.Config{NodeToken: "token"}, st, engine)

	ts := httptest.NewServer(app.Handler(appCtx))
	defer ts.Close()

	header := http.Header{}
	header.Set("Authorization", "Bearer token")
	node, _, err := websocket.Dial(ctx, wsURL(ts.URL)+"/ingest", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer node.CloseNow()

	stop()

	readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
	defer readCancel()
	_, _, err = node.Read(readCtx)
	if err == nil {
		t.Fatal("ingest connection stayed open after app context cancellation")
	}
	if readCtx.Err() != nil {
		t.Fatal("timed out waiting for ingest connection to close")
	}
	if got := websocket.CloseStatus(err); got != websocket.StatusGoingAway {
		t.Fatalf("close status = %v, want %v", got, websocket.StatusGoingAway)
	}
}

func readLiveEvent(t *testing.T, ctx context.Context, conn *websocket.Conn, want protocol.EventType) protocol.Event {
	t.Helper()
	for {
		_, payload, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(payload, &envelope) != nil || envelope.Type != protocol.TypeEvent {
			continue
		}
		var ev protocol.Event
		if err := json.Unmarshal(payload, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.EventType == want {
			return ev
		}
	}
}

func pollResult(projectID, screenID, skuID int64, flag int, clickable bool, price int, serverTs int64) protocol.PollResult {
	flagText := "已售罄"
	if flag == 2 {
		flagText = "在售"
	}
	if flag == 1 {
		flagText = "未开售"
	}
	return protocol.PollResult{
		Type:      protocol.TypePollResult,
		Version:   protocol.Version,
		NodeID:    "node-1",
		ProjectID: projectID,
		ServerTs:  serverTs,
		FetchedAt: serverTs,
		Project: &protocol.ProjectMeta{
			Name:     "Project",
			Flag:     2,
			FlagText: "在售",
			ServerTs: serverTs,
		},
		Screens: []protocol.ScreenDiff{{
			ScreenID:  screenID,
			Name:      "Day 1",
			Flag:      2,
			FlagText:  "在售",
			Clickable: true,
		}},
		Diff: []protocol.SkuDiff{{
			ScreenID: screenID,
			SkuID:    skuID,
			Desc:     "VIP",
			FlagText: flagText,
			New: &protocol.Fingerprint{
				Flag:      flag,
				Clickable: clickable,
				PriceFen:  price,
			},
		}},
	}
}

func wsURL(url string) string {
	return "ws" + strings.TrimPrefix(url, "http")
}
