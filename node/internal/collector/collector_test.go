package collector

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/WittF/fnmdp/node/internal/ratelimit"
	"github.com/WittF/fnmdp/shared/getv2"
	"github.com/WittF/fnmdp/shared/protocol"
)

type fakeFetcher struct {
	mu    sync.Mutex
	raws  []*getv2.RawResponse
	errs  []error
	calls int
}

type fakeMallFetcher struct {
	mu      sync.Mutex
	raw     *getv2.RawResponse
	err     error
	itemsID int64
	calls   int
}

func (f *fakeMallFetcher) FetchByItemsID(ctx context.Context, itemsID int64) (*getv2.RawResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.itemsID = itemsID
	if f.err != nil {
		return nil, f.err
	}
	return f.raw, nil
}

func (f *fakeMallFetcher) Fetch(ctx context.Context, projectID int64) (*getv2.RawResponse, error) {
	return f.FetchByItemsID(ctx, projectID)
}

func (f *fakeMallFetcher) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeMallFetcher) ItemsID() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.itemsID
}

func (f *fakeFetcher) Fetch(ctx context.Context, projectID int64) (*getv2.RawResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	i := f.calls - 1
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i < len(f.raws) {
		return f.raws[i], nil
	}
	return f.raws[len(f.raws)-1], nil
}

func (f *fakeFetcher) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type memorySink struct {
	mu   sync.Mutex
	msgs []any
}

func (s *memorySink) Send(ctx context.Context, msg any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *memorySink) Messages() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]any(nil), s.msgs...)
}

func TestCollectorSendsOnlyChangedPollResults(t *testing.T) {
	raw := rawWithSku(100, 10, 1, 4, false, 8800, "已售罄")
	fetcher := &fakeFetcher{raws: []*getv2.RawResponse{raw, raw}}
	sink := &memorySink{}
	c := New("node-1", fetcher, sink, ratelimit.New(time.Millisecond, time.Second), func() ClientConfigSnapshot {
		return ClientConfigSnapshot{SoftBackoff: time.Millisecond, HardCooldown: time.Second}
	})

	c.Poll(context.Background(), 100)
	c.Poll(context.Background(), 100)

	msgs := sink.Messages()
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1: %+v", len(msgs), msgs)
	}
	pr, ok := msgs[0].(protocol.PollResult)
	if !ok {
		t.Fatalf("message type = %T, want PollResult", msgs[0])
	}
	if len(pr.Diff) != 1 || pr.Diff[0].New == nil || pr.Diff[0].FlagText != "已售罄" {
		t.Fatalf("poll result = %+v", pr)
	}
	if pr.Source != "getv2" {
		t.Fatalf("source = %q, want getv2", pr.Source)
	}
	if pr.Project == nil || pr.Project.Name != "Project" || pr.Project.FlagText != "在售" {
		t.Fatalf("project meta = %+v", pr.Project)
	}
	if len(pr.Screens) != 1 || pr.Screens[0].Name != "Day 1" || pr.Screens[0].Removed {
		t.Fatalf("screens = %+v", pr.Screens)
	}
}

func TestCollectorFloorReportsOnKeepalive(t *testing.T) {
	raw := rawWithSku(100, 10, 1, 4, false, 8800, "已售罄")
	fetcher := &fakeFetcher{raws: []*getv2.RawResponse{raw, raw}}
	sink := &memorySink{}
	c := New("node-1", fetcher, sink, ratelimit.New(time.Millisecond, time.Second), func() ClientConfigSnapshot {
		return ClientConfigSnapshot{SoftBackoff: time.Millisecond, HardCooldown: time.Second, Keepalive: time.Nanosecond}
	})

	c.Poll(context.Background(), 100) // changed → full report
	time.Sleep(time.Millisecond)      // let the (1ns) keepalive elapse
	c.Poll(context.Background(), 100) // unchanged, keepalive due → floor report

	msgs := sink.Messages()
	if len(msgs) != 2 {
		t.Fatalf("messages = %d, want 2 (change + floor): %+v", len(msgs), msgs)
	}
	if first := msgs[0].(protocol.PollResult); len(first.Diff) == 0 || first.Project == nil {
		t.Fatalf("first message should be a full change report: %+v", first)
	}
	floor := msgs[1].(protocol.PollResult)
	if len(floor.Diff) != 0 || floor.Project != nil || len(floor.Screens) != 0 {
		t.Fatalf("floor report must carry empty diff/project/screens: %+v", floor)
	}
	if floor.ProjectID != 100 || floor.FetchedAt == 0 {
		t.Fatalf("floor report must still carry liveness fields (project_id, fetched_at): %+v", floor)
	}
}

func TestCollectorHardBlockPreventsRequestsDuringCooldown(t *testing.T) {
	fetcher := &fakeFetcher{
		raws: []*getv2.RawResponse{rawWithSku(100, 10, 1, 2, true, 8800, "在售")},
		errs: []error{&getv2.RateError{
			StatusCode:   http.StatusPreconditionFailed,
			Sentinel:     getv2.ErrHardBlocked,
			SecRequestID: "sec-1",
		}},
	}
	sink := &memorySink{}
	c := New("node-1", fetcher, sink, ratelimit.New(time.Millisecond, 50*time.Millisecond), func() ClientConfigSnapshot {
		return ClientConfigSnapshot{SoftBackoff: time.Millisecond, HardCooldown: 50 * time.Millisecond}
	})

	c.Poll(context.Background(), 100)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	c.Poll(ctx, 100)

	if calls := fetcher.Calls(); calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", calls)
	}
	msgs := sink.Messages()
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	ev, ok := msgs[0].(protocol.RateEvent)
	if !ok {
		t.Fatalf("message type = %T, want RateEvent", msgs[0])
	}
	if ev.Kind != protocol.RateHardBlocked || ev.StatusCode != http.StatusPreconditionFailed || ev.SecRequestID != "sec-1" {
		t.Fatalf("rate event = %+v", ev)
	}
	if !errors.Is(fetcher.errs[0], getv2.ErrHardBlocked) {
		t.Fatal("test fixture must wrap ErrHardBlocked")
	}
}

func TestCollectorLogsNonRateErrors(t *testing.T) {
	fetcher := &fakeFetcher{errs: []error{errors.New("getv2: decode body: bad sale_flag")}}
	sink := &memorySink{}
	c := New("node-1", fetcher, sink, ratelimit.New(time.Millisecond, time.Second), func() ClientConfigSnapshot {
		return ClientConfigSnapshot{SoftBackoff: time.Millisecond, HardCooldown: time.Second}
	})

	var buf bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	}()

	c.Poll(context.Background(), 100)

	if msgs := sink.Messages(); len(msgs) != 0 {
		t.Fatalf("messages = %+v, want none", msgs)
	}
	got := buf.String()
	if !strings.Contains(got, "node_id=node-1") ||
		!strings.Contains(got, "project_id=100") ||
		!strings.Contains(got, "decode body") {
		t.Fatalf("log = %q", got)
	}
}

func TestFallbackFetcherUsesMallAfterPrimaryRateLimit(t *testing.T) {
	primary := &fakeFetcher{
		errs: []error{&getv2.RateError{
			StatusCode: http.StatusTooManyRequests,
			Sentinel:   getv2.ErrSoftLimited,
		}},
		raws: []*getv2.RawResponse{rawWithSku(100, 10, 1, 2, true, 8800, "在售")},
	}
	mall := &fakeMallFetcher{raw: rawWithSku(100, 10, 1, 2, true, 8800, "在售")}
	primaryRate := ratelimit.New(time.Millisecond, time.Second)
	fetcher := NewFallbackFetcher(primary, "getv2", mall, "mall", primaryRate, ratelimit.New(time.Millisecond, time.Second))

	raw, source, err := fetcher.FetchWithSource(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if source != "mall" || raw == nil {
		t.Fatalf("source/raw = %q %+v", source, raw)
	}
	if primary.Calls() != 1 || mall.Calls() != 1 || mall.ItemsID() != 100 {
		t.Fatalf("primary calls=%d mall calls=%d items=%d", primary.Calls(), mall.Calls(), mall.ItemsID())
	}
	if snap := primaryRate.Snapshot(time.Now()); snap.SoftHits != 1 {
		t.Fatalf("primary rate snapshot = %+v", snap)
	}
}

func TestFallbackFetcherSkipsPrimaryDuringCooldown(t *testing.T) {
	primary := &fakeFetcher{raws: []*getv2.RawResponse{rawWithSku(100, 10, 1, 2, true, 8800, "在售")}}
	mall := &fakeMallFetcher{raw: rawWithSku(100, 10, 1, 2, true, 8800, "在售")}
	primaryRate := ratelimit.New(time.Millisecond, time.Second)
	primaryRate.RecordHard(time.Now())
	fetcher := NewFallbackFetcher(primary, "getv2", mall, "mall", primaryRate, ratelimit.New(time.Millisecond, time.Second))

	_, source, err := fetcher.FetchWithSource(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if source != "mall" {
		t.Fatalf("source = %q, want mall", source)
	}
	if primary.Calls() != 0 || mall.Calls() != 1 {
		t.Fatalf("primary calls=%d mall calls=%d", primary.Calls(), mall.Calls())
	}
}

func TestCollectorManagedFallbackSendsMallSource(t *testing.T) {
	primary := &fakeFetcher{errs: []error{errors.New("temporary transport")}}
	mall := &fakeMallFetcher{raw: rawWithSku(100, 10, 1, 2, true, 8800, "在售")}
	fetcher := NewFallbackFetcher(primary, "getv2", mall, "mall", ratelimit.New(time.Millisecond, time.Second), ratelimit.New(time.Millisecond, time.Second))
	sink := &memorySink{}
	c := New("node-1", fetcher, sink, ratelimit.New(time.Millisecond, time.Second), func() ClientConfigSnapshot {
		return ClientConfigSnapshot{SoftBackoff: time.Millisecond, HardCooldown: time.Second}
	})

	c.Poll(context.Background(), 100)

	msgs := sink.Messages()
	if len(msgs) != 1 {
		t.Fatalf("messages = %+v", msgs)
	}
	pr, ok := msgs[0].(protocol.PollResult)
	if !ok || pr.Source != "mall" {
		t.Fatalf("poll result = %+v", msgs[0])
	}
}

func TestFallbackFetcherMallProjectErrorFallsBackToGetV2(t *testing.T) {
	mall := &fakeFetcher{
		errs: []error{&getv2.ProjectError{Errno: 71100001, Msg: "商品不存在"}},
	}
	getv2Secondary := &fakeFetcher{raws: []*getv2.RawResponse{rawWithSku(100, 10, 1, 2, true, 8800, "在售")}}
	fetcher := NewFallbackFetcher(mall, "mall", getv2Secondary, "getv2", ratelimit.New(time.Millisecond, time.Second), ratelimit.New(time.Millisecond, time.Second))

	raw, source, err := fetcher.FetchWithSource(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if source != "getv2" || raw == nil {
		t.Fatalf("source/raw = %q %+v", source, raw)
	}
	if mall.Calls() != 1 || getv2Secondary.Calls() != 1 {
		t.Fatalf("mall calls=%d getv2 calls=%d", mall.Calls(), getv2Secondary.Calls())
	}
}

func TestFallbackFetcherGetV2ProjectErrorIsTerminal(t *testing.T) {
	getv2Primary := &fakeFetcher{
		errs: []error{&getv2.ProjectError{Errno: 100001, Msg: "项目不存在"}},
	}
	mallSecondary := &fakeFetcher{raws: []*getv2.RawResponse{rawWithSku(100, 10, 1, 2, true, 8800, "在售")}}
	fetcher := NewFallbackFetcher(getv2Primary, "getv2", mallSecondary, "mall", ratelimit.New(time.Millisecond, time.Second), ratelimit.New(time.Millisecond, time.Second))

	_, _, err := fetcher.FetchWithSource(context.Background(), 100)
	if !errors.Is(err, getv2.ErrProjectError) {
		t.Fatalf("err = %v, want ErrProjectError", err)
	}
	if mallSecondary.Calls() != 0 {
		t.Fatalf("mall fallback calls = %d, want 0", mallSecondary.Calls())
	}
}

func TestFallbackFetcherSecondaryMinIntervalThrottlesGetV2(t *testing.T) {
	mall := &fakeFetcher{errs: []error{errors.New("mall temporary"), errors.New("mall temporary")}}
	getv2Secondary := &fakeFetcher{
		raws: []*getv2.RawResponse{
			rawWithSku(100, 10, 1, 2, true, 8800, "在售"),
			rawWithSku(100, 10, 1, 2, true, 8800, "在售"),
		},
	}
	fetcher := NewFallbackFetcher(mall, "mall", getv2Secondary, "getv2", ratelimit.New(time.Millisecond, time.Second), ratelimit.New(time.Millisecond, time.Second))
	fetcher.SetSecondaryMinInterval(time.Hour)

	if _, source, err := fetcher.FetchWithSource(context.Background(), 100); err != nil || source != "getv2" {
		t.Fatalf("first fallback source/err = %q/%v", source, err)
	}
	_, _, err := fetcher.FetchWithSource(context.Background(), 100)
	if !errors.Is(err, ErrSecondaryThrottled) {
		t.Fatalf("second fallback err = %v, want ErrSecondaryThrottled", err)
	}
	if getv2Secondary.Calls() != 1 {
		t.Fatalf("getv2 calls = %d, want 1", getv2Secondary.Calls())
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
