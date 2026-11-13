package state

import (
	"testing"
	"time"

	"github.com/WittF/fnmdp/master/internal/model"
	"github.com/WittF/fnmdp/master/internal/store"
)

func TestBuildTrajectoryNoEventsUsesCurrentStateInsideCoverage(t *testing.T) {
	base := time.Unix(1000, 0)
	current := model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 1, FlagText: "未开售"}
	points := BuildTrajectory(current, nil, []model.PollCoverage{{
		ProjectID: 100,
		StartTs:   base.Unix(),
		EndTs:     base.Add(4 * time.Second).Unix(),
	}}, store.TrajectoryQuery{
		ProjectID:  100,
		ScreenID:   10,
		SkuID:      1,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(4 * time.Second).UnixMilli(),
		IntervalMs: 1000,
	}, 0, base.Add(4*time.Second).UnixMilli())
	if len(points) != 5 {
		t.Fatalf("points = %d, want 5", len(points))
	}
	for _, p := range points {
		if !p.Observed || p.Flag != 1 || p.Clickable || p.FlagText != "未开售" {
			t.Fatalf("unexpected point: %+v", p)
		}
	}
}

func TestBuildTrajectoryReplaysEventSteps(t *testing.T) {
	base := time.Unix(1000, 0)
	events := []model.Event{{
		ProjectID:    100,
		ScreenID:     10,
		SkuID:        1,
		OldFlag:      1,
		NewFlag:      2,
		OldClickable: false,
		NewClickable: true,
		FlagText:     "在售",
		CreatedAt:    base.Add(2 * time.Second),
	}}
	current := model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 2, Clickable: true, FlagText: "在售"}
	points := BuildTrajectory(current, events, []model.PollCoverage{{
		ProjectID: 100,
		StartTs:   base.Unix(),
		EndTs:     base.Add(4 * time.Second).Unix(),
	}}, store.TrajectoryQuery{
		ProjectID:  100,
		ScreenID:   10,
		SkuID:      1,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(4 * time.Second).UnixMilli(),
		IntervalMs: 1000,
	}, 0, base.Add(4*time.Second).UnixMilli())
	if points[0].Flag != 1 || points[1].Flag != 1 {
		t.Fatalf("pre-event flags = %d/%d, want old flag 1", points[0].Flag, points[1].Flag)
	}
	if points[2].Flag != 2 || !points[2].Clickable || points[4].Flag != 2 {
		t.Fatalf("post-event points = %+v / %+v", points[2], points[4])
	}
}

func TestBuildTrajectoryMasksCoverageGaps(t *testing.T) {
	base := time.Unix(1000, 0)
	current := model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 4, FlagText: "已售罄"}
	points := BuildTrajectory(current, nil, []model.PollCoverage{
		{ProjectID: 100, StartTs: base.Unix(), EndTs: base.Add(time.Second).Unix()},
		{ProjectID: 100, StartTs: base.Add(3 * time.Second).Unix(), EndTs: base.Add(4 * time.Second).Unix()},
	}, store.TrajectoryQuery{
		ProjectID:  100,
		ScreenID:   10,
		SkuID:      1,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(4 * time.Second).UnixMilli(),
		IntervalMs: 1000,
	}, 0, base.Add(4*time.Second).UnixMilli())
	if !points[0].Observed || !points[1].Observed {
		t.Fatalf("first coverage segment not observed: %+v", points[:2])
	}
	if points[2].Observed {
		t.Fatalf("gap point observed: %+v", points[2])
	}
	if !points[3].Observed || !points[4].Observed {
		t.Fatalf("second coverage segment not observed: %+v", points[3:])
	}
}

func TestBuildTrajectoryKeepsInBucketEventSamples(t *testing.T) {
	base := time.Unix(1000, 0)
	events := []model.Event{
		{
			ProjectID:    100,
			ScreenID:     10,
			SkuID:        1,
			OldFlag:      1,
			NewFlag:      2,
			NewClickable: true,
			FlagText:     "在售",
			CreatedAt:    base.Add(1500 * time.Millisecond),
		},
		{
			ProjectID:    100,
			ScreenID:     10,
			SkuID:        1,
			OldFlag:      2,
			OldClickable: true,
			NewFlag:      4,
			FlagText:     "已售罄",
			CreatedAt:    base.Add(1600 * time.Millisecond),
		},
	}
	current := model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 4, FlagText: "已售罄"}
	points := BuildTrajectory(current, events, []model.PollCoverage{{
		ProjectID: 100,
		StartTs:   base.Unix(),
		EndTs:     base.Add(2 * time.Second).Unix(),
	}}, store.TrajectoryQuery{
		ProjectID:  100,
		ScreenID:   10,
		SkuID:      1,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(2 * time.Second).UnixMilli(),
		IntervalMs: 1000,
	}, 0, base.Add(2*time.Second).UnixMilli())
	if got := len(points[1].Samples); got != 2 {
		t.Fatalf("samples = %d, want 2: %+v", got, points[1].Samples)
	}
	if points[1].Samples[0].Flag != 2 || !points[1].Samples[0].Clickable || points[1].Samples[1].Flag != 4 {
		t.Fatalf("unexpected samples: %+v", points[1].Samples)
	}
}

func TestBuildTrajectoryUsesNextOldStateToMaskMissingIntermediateEvent(t *testing.T) {
	base := time.Unix(1000, 0)
	events := []model.Event{
		{
			ProjectID:    100,
			ScreenID:     10,
			SkuID:        1,
			OldFlag:      4,
			NewFlag:      2,
			OldClickable: false,
			NewClickable: true,
			FlagText:     "预售中",
			CreatedAt:    base,
		},
		{
			ProjectID:    100,
			ScreenID:     10,
			SkuID:        1,
			OldFlag:      4,
			NewFlag:      2,
			OldClickable: false,
			NewClickable: true,
			FlagText:     "预售中",
			CreatedAt:    base.Add(10 * time.Minute),
		},
		{
			ProjectID:    100,
			ScreenID:     10,
			SkuID:        1,
			OldFlag:      2,
			NewFlag:      4,
			OldClickable: true,
			NewClickable: false,
			FlagText:     "已售罄",
			CreatedAt:    base.Add(10*time.Minute + 30*time.Second),
		},
	}
	current := model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 4, FlagText: "已售罄"}
	points := BuildTrajectory(current, events, []model.PollCoverage{{
		ProjectID: 100,
		StartTs:   base.Unix(),
		EndTs:     base.Add(11 * time.Minute).Unix(),
	}}, store.TrajectoryQuery{
		ProjectID:  100,
		ScreenID:   10,
		SkuID:      1,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(11 * time.Minute).UnixMilli(),
		IntervalMs: 30_000,
	}, 0, base.Add(11*time.Minute).UnixMilli())

	if points[1].Flag != 4 || points[1].Clickable {
		t.Fatalf("missing intermediate soldout should be inferred before next restock: %+v", points[1])
	}
	restock := points[20]
	if restock.Flag != 2 || !restock.Clickable || len(restock.Samples) != 1 || restock.Samples[0].Flag != 2 {
		t.Fatalf("real restock bucket must stay available: %+v", restock)
	}
	soldout := points[21]
	if soldout.Flag != 4 || soldout.Clickable || len(soldout.Samples) != 1 || soldout.Samples[0].Flag != 4 {
		t.Fatalf("soldout bucket must carry the final soldout sample: %+v", soldout)
	}
}

func TestBuildTrajectoryCapsLiveGraceAtMasterNow(t *testing.T) {
	base := time.Unix(1000, 0)
	current := model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 2, Clickable: true, FlagText: "在售"}
	coverage := []model.PollCoverage{{
		ProjectID: 100,
		StartTs:   base.Unix(),
		EndTs:     base.Add(time.Second).Unix(),
	}}
	q := store.TrajectoryQuery{
		ProjectID:  100,
		ScreenID:   10,
		SkuID:      1,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(300 * time.Second).UnixMilli(), // client clock ratcheted 300s ahead
		IntervalMs: 1000,
	}
	liveGrace := 5 * time.Minute

	// master's real clock just past the last poll
	points := BuildTrajectory(current, nil, coverage, q, liveGrace, base.Add(2*time.Second).UnixMilli())
	if len(points) != 301 {
		t.Fatalf("points = %d, want 301", len(points))
	}
	// real coverage buckets stay observed
	if !points[0].Observed || !points[1].Observed {
		t.Fatalf("real coverage not observed: %+v", points[:2])
	}
	// buckets well past master now are nodata, not forward-filled to client untilMs
	// (without the cap these would all be observed -> the "solid connected block" bug)
	for _, i := range []int{3, 100, 200, 300} {
		if points[i].Observed {
			t.Fatalf("future bucket %d observed (solid-block bug): %+v", i, points[i])
		}
	}

	// No drift: when master clock == client untilMs, the live-grace edge still extends to it.
	q2 := q
	q2.UntilMs = base.Add(2 * time.Second).UnixMilli()
	noDrift := BuildTrajectory(current, nil, coverage, q2, liveGrace, base.Add(2*time.Second).UnixMilli())
	for i, p := range noDrift {
		if !p.Observed {
			t.Fatalf("no-drift bucket %d not observed: %+v", i, p)
		}
	}
}

func TestBuildTrajectoryLiveGraceNeverShrinksRealCoverage(t *testing.T) {
	base := time.Unix(1000, 0)
	current := model.SKU{ProjectID: 100, ScreenID: 10, SkuID: 1, Flag: 1, FlagText: "未开售"}
	coverage := []model.PollCoverage{{
		ProjectID: 100,
		StartTs:   base.Unix(),
		EndTs:     base.Add(4 * time.Second).Unix(),
	}}
	q := store.TrajectoryQuery{
		ProjectID:  100,
		ScreenID:   10,
		SkuID:      1,
		SinceMs:    base.UnixMilli(),
		UntilMs:    base.Add(4 * time.Second).UnixMilli(),
		IntervalMs: 1000,
	}
	// A degenerate / behind master clock must not delete real observed coverage.
	points := BuildTrajectory(current, nil, coverage, q, 5*time.Minute, base.Add(time.Second).UnixMilli())
	if len(points) != 5 {
		t.Fatalf("points = %d, want 5", len(points))
	}
	for i, p := range points {
		if !p.Observed {
			t.Fatalf("real coverage bucket %d wrongly dropped: %+v", i, p)
		}
	}
}
