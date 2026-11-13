package dedup

import (
	"testing"

	"github.com/WittF/fnmdp/shared/getv2"
)

func TestL1DiffAddedChangedRemovedAndNoChange(t *testing.T) {
	l := New()
	ps := getv2.ProjectState{
		ProjectID: 100,
		Name:      "Project",
		Flag:      2,
		FlagText:  "在售",
		ServerTs:  1750100000,
		Screens: []getv2.ScreenState{{
			ScreenID: 10, Name: "Day 1", Flag: 2, FlagText: "在售", Clickable: true,
			SaleStart: 1750000000, SaleEnd: 1751000000,
		}},
		Skus: []getv2.SkuState{{
			ProjectID: 100, ScreenID: 10, SkuID: 1, Desc: "VIP",
			Flag: 4, FlagText: "已售罄", Clickable: false, PriceFen: 8800,
		}},
	}

	diff := l.Diff(ps)
	if !diff.Changed() || len(diff.Skus) != 1 || diff.Skus[0].Old != nil || diff.Skus[0].New == nil {
		t.Fatalf("added diff = %+v", diff)
	}
	if diff.Project.Name != "Project" || diff.Project.FlagText != "在售" {
		t.Fatalf("project meta = %+v", diff.Project)
	}
	if len(diff.Screens) != 1 || diff.Screens[0].Name != "Day 1" || diff.Screens[0].Removed {
		t.Fatalf("screen diff = %+v", diff.Screens)
	}
	if diff.Skus[0].FlagText != "已售罄" {
		t.Fatalf("flag_text = %q", diff.Skus[0].FlagText)
	}

	if diff := l.Diff(ps); diff.Changed() {
		t.Fatalf("no-change diff = %+v", diff)
	}

	ps.Skus[0].Flag = 2
	ps.Skus[0].FlagText = "在售"
	ps.Skus[0].Clickable = true
	diff = l.Diff(ps)
	if !diff.Changed() || len(diff.Skus) != 1 || diff.Skus[0].Old == nil || diff.Skus[0].New == nil {
		t.Fatalf("changed diff = %+v", diff)
	}
	if diff.Skus[0].Old.Flag != 4 || diff.Skus[0].New.Flag != 2 || diff.Skus[0].FlagText != "在售" {
		t.Fatalf("changed payload = %+v", diff.Skus[0])
	}

	ps.Skus = nil
	diff = l.Diff(ps)
	if diff.Changed() {
		t.Fatalf("first missing sku should be held as suspect, got %+v", diff)
	}
	diff = l.Diff(ps)
	if !diff.Changed() || len(diff.Skus) != 1 || diff.Skus[0].Old == nil || diff.Skus[0].New != nil {
		t.Fatalf("removed diff = %+v", diff)
	}
	if diff.Skus[0].FlagText != "" {
		t.Fatalf("removed flag_text = %q, want empty", diff.Skus[0].FlagText)
	}
}

func TestL1DiffProjectAndScreenChangesTriggerSend(t *testing.T) {
	l := New()
	ps := getv2.ProjectState{
		ProjectID: 100,
		Name:      "Project",
		Flag:      1,
		FlagText:  "未开售",
		ServerTs:  1750100000,
		Screens: []getv2.ScreenState{{
			ScreenID: 10, Name: "Day 1", Flag: 1, FlagText: "未开售",
		}},
	}

	if diff := l.Diff(ps); !diff.Changed() || len(diff.Screens) != 1 {
		t.Fatalf("initial diff = %+v", diff)
	}
	if diff := l.Diff(ps); diff.Changed() {
		t.Fatalf("no-change diff = %+v", diff)
	}

	ps.HotProject = true
	diff := l.Diff(ps)
	if !diff.Changed() || !diff.Project.Hot || len(diff.Screens) != 0 || len(diff.Skus) != 0 {
		t.Fatalf("project-only diff = %+v", diff)
	}

	ps.Screens[0].Flag = 2
	ps.Screens[0].FlagText = "在售"
	ps.Screens[0].Clickable = true
	diff = l.Diff(ps)
	if !diff.Changed() || len(diff.Screens) != 1 || diff.Screens[0].Flag != 2 {
		t.Fatalf("screen-only diff = %+v", diff)
	}

	ps.Screens = nil
	diff = l.Diff(ps)
	if diff.Changed() {
		t.Fatalf("first missing screen should be held as suspect, got %+v", diff)
	}
	diff = l.Diff(ps)
	if !diff.Changed() || len(diff.Screens) != 1 || !diff.Screens[0].Removed {
		t.Fatalf("screen-removed diff = %+v", diff)
	}
}

func TestL1DiffMissingThenRecoveredDoesNotRemove(t *testing.T) {
	l := New()
	ps := getv2.ProjectState{
		ProjectID: 100,
		Name:      "Project",
		Flag:      2,
		FlagText:  "在售",
		ServerTs:  1750100000,
		Screens: []getv2.ScreenState{{
			ScreenID: 10, Name: "Day 1", Flag: 2, FlagText: "在售", Clickable: true,
		}},
		Skus: []getv2.SkuState{{
			ProjectID: 100, ScreenID: 10, SkuID: 1, Desc: "VIP",
			Flag: 2, FlagText: "在售", Clickable: true, PriceFen: 8800,
		}},
	}

	if diff := l.Diff(ps); !diff.Changed() {
		t.Fatalf("initial diff = %+v", diff)
	}
	partial := ps
	partial.Screens = nil
	partial.Skus = nil
	if diff := l.Diff(partial); diff.Changed() {
		t.Fatalf("suspect partial should not emit removals: %+v", diff)
	}
	if diff := l.Diff(ps); diff.Changed() {
		t.Fatalf("recovered full poll should clear pending removals without add/remove: %+v", diff)
	}
}

func TestL1DiffScreenShrinkDoesNotChurnProjectSaleStart(t *testing.T) {
	l := New()
	full := getv2.ProjectState{
		ProjectID: 100,
		Name:      "Project",
		Flag:      2,
		FlagText:  "在售",
		SaleStart: 1000,
		SaleEnd:   3000,
		ServerTs:  1750100000,
		Screens: []getv2.ScreenState{
			{ScreenID: 10, Name: "Early", Flag: 2, FlagText: "在售", Clickable: true, SaleStart: 1000, SaleEnd: 2000},
			{ScreenID: 11, Name: "Late", Flag: 2, FlagText: "在售", Clickable: true, SaleStart: 2000, SaleEnd: 3000},
		},
	}
	if diff := l.Diff(full); !diff.Changed() {
		t.Fatalf("initial diff = %+v", diff)
	}

	shrunk := full
	shrunk.SaleStart = 2000
	shrunk.Screens = full.Screens[1:]
	if diff := l.Diff(shrunk); diff.Changed() {
		t.Fatalf("first shrink should be a no-op, not project sale_start churn: %+v", diff)
	}
	diff := l.Diff(shrunk)
	if !diff.Changed() || len(diff.Screens) != 1 || !diff.Screens[0].Removed {
		t.Fatalf("second shrink should confirm removal: %+v", diff)
	}
}
