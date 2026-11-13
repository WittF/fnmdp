package dedup

import (
	"sync"

	"github.com/WittF/fnmdp/shared/getv2"
	"github.com/WittF/fnmdp/shared/protocol"
	"github.com/WittF/fnmdp/shared/state"
)

type L1 struct {
	mu             sync.Mutex
	items          map[string]record
	projects       map[int64]projectRecord
	screens        map[string]screenRecord
	pendingItems   map[string]record
	pendingScreens map[string]screenRecord
}

type record struct {
	projectID int64
	screenID  int64
	skuID     int64
	desc      string
	fp        state.StateFingerprint
}

type projectRecord struct {
	projectID int64
	fp        state.ProjectFingerprint
}

type screenRecord struct {
	projectID int64
	screenID  int64
	fp        state.ScreenFingerprint
}

type DiffResult struct {
	Send    bool
	Project protocol.ProjectMeta
	Screens []protocol.ScreenDiff
	Skus    []protocol.SkuDiff
}

func (r DiffResult) Changed() bool {
	return r.Send
}

func New() *L1 {
	return &L1{
		items:          map[string]record{},
		projects:       map[int64]projectRecord{},
		screens:        map[string]screenRecord{},
		pendingItems:   map[string]record{},
		pendingScreens: map[string]screenRecord{},
	}
}

func (l *L1) Diff(ps getv2.ProjectState) DiffResult {
	l.mu.Lock()
	defer l.mu.Unlock()

	var result DiffResult
	seenScreens := make(map[string]struct{}, len(ps.Screens))
	for _, screen := range ps.Screens {
		seenScreens[state.ScreenDedupKey(ps.ProjectID, screen.ScreenID)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ps.Skus))
	for _, sku := range ps.Skus {
		seen[state.DedupKey(sku.ProjectID, sku.ScreenID, sku.SkuID)] = struct{}{}
	}
	if l.suspectScreenShrink(ps.ProjectID, seenScreens, seen) {
		return DiffResult{}
	}

	projectFP := state.ProjectStateFingerprint(ps)
	projectChanged := false
	if prev, ok := l.projects[ps.ProjectID]; !ok || state.ProjectChanged(prev.fp, projectFP) {
		projectChanged = true
	}
	l.projects[ps.ProjectID] = projectRecord{projectID: ps.ProjectID, fp: projectFP}

	for _, screen := range ps.Screens {
		key := state.ScreenDedupKey(ps.ProjectID, screen.ScreenID)
		seenScreens[key] = struct{}{}
		delete(l.pendingScreens, key)
		cur := state.ScreenStateFingerprint(screen)
		prev, ok := l.screens[key]
		if !ok || state.ScreenChanged(prev.fp, cur) {
			result.Screens = append(result.Screens, screenToProtocol(screen))
		}
		l.screens[key] = screenRecord{
			projectID: ps.ProjectID,
			screenID:  screen.ScreenID,
			fp:        cur,
		}
	}
	for key, prev := range l.screens {
		if prev.projectID != ps.ProjectID {
			continue
		}
		if _, ok := seenScreens[key]; ok {
			continue
		}
		if _, pending := l.pendingScreens[key]; pending {
			result.Screens = append(result.Screens, protocol.ScreenDiff{
				ScreenID: prev.screenID,
				Removed:  true,
			})
			delete(l.screens, key)
			delete(l.pendingScreens, key)
			continue
		}
		l.pendingScreens[key] = prev
	}

	for _, sku := range ps.Skus {
		key := state.DedupKey(sku.ProjectID, sku.ScreenID, sku.SkuID)
		seen[key] = struct{}{}
		delete(l.pendingItems, key)
		cur := state.Fingerprint(sku)
		curProto := toProtocol(cur)
		prev, ok := l.items[key]
		switch {
		case !ok:
			result.Skus = append(result.Skus, protocol.SkuDiff{
				ScreenID: sku.ScreenID,
				SkuID:    sku.SkuID,
				Desc:     sku.Desc,
				FlagText: sku.FlagText,
				LimitNum: sku.LimitNum,
				Old:      nil,
				New:      &curProto,
			})
		case state.Changed(prev.fp, cur):
			oldProto := toProtocol(prev.fp)
			result.Skus = append(result.Skus, protocol.SkuDiff{
				ScreenID: sku.ScreenID,
				SkuID:    sku.SkuID,
				Desc:     sku.Desc,
				FlagText: sku.FlagText,
				LimitNum: sku.LimitNum,
				Old:      &oldProto,
				New:      &curProto,
			})
		}
		l.items[key] = record{
			projectID: sku.ProjectID,
			screenID:  sku.ScreenID,
			skuID:     sku.SkuID,
			desc:      sku.Desc,
			fp:        cur,
		}
	}

	for key, prev := range l.items {
		if prev.projectID != ps.ProjectID {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		if _, pending := l.pendingItems[key]; pending {
			oldProto := toProtocol(prev.fp)
			result.Skus = append(result.Skus, protocol.SkuDiff{
				ScreenID: prev.screenID,
				SkuID:    prev.skuID,
				Desc:     prev.desc,
				FlagText: "",
				LimitNum: prev.fp.LimitNum,
				Old:      &oldProto,
				New:      nil,
			})
			delete(l.items, key)
			delete(l.pendingItems, key)
			continue
		}
		l.pendingItems[key] = prev
	}
	if projectChanged || len(result.Screens) > 0 || len(result.Skus) > 0 {
		result.Send = true
		result.Project = projectToProtocol(ps)
		return result
	}
	return DiffResult{}
}

func (l *L1) suspectScreenShrink(projectID int64, seenScreens, seenSkus map[string]struct{}) bool {
	if projectID <= 0 {
		return false
	}
	missingScreen := false
	allMissingScreensPending := true
	for key, prev := range l.screens {
		if prev.projectID != projectID {
			continue
		}
		if _, ok := seenScreens[key]; ok {
			continue
		}
		missingScreen = true
		if _, pending := l.pendingScreens[key]; !pending {
			allMissingScreensPending = false
		}
	}
	if !missingScreen || allMissingScreensPending {
		return false
	}
	for key, prev := range l.screens {
		if prev.projectID != projectID {
			continue
		}
		if _, ok := seenScreens[key]; !ok {
			l.pendingScreens[key] = prev
		}
	}
	for key, prev := range l.items {
		if prev.projectID != projectID {
			continue
		}
		if _, ok := seenSkus[key]; !ok {
			l.pendingItems[key] = prev
		}
	}
	return true
}

func toProtocol(fp state.StateFingerprint) protocol.Fingerprint {
	return protocol.Fingerprint{
		Flag:      fp.Flag,
		Clickable: fp.Clickable,
		PriceFen:  fp.PriceFen,
		Num:       fp.Num,
	}
}

func projectToProtocol(ps getv2.ProjectState) protocol.ProjectMeta {
	return protocol.ProjectMeta{
		Name:        ps.Name,
		Flag:        ps.Flag,
		FlagText:    ps.FlagText,
		Hot:         ps.HotProject,
		WishCount:   ps.WishCount,
		PriceLow:    ps.PriceLowFen,
		PriceHigh:   ps.PriceHighFen,
		SalableLow:  ps.SalablePriceLow,
		SalableHigh: ps.SalablePriceHigh,
		Status:      ps.Status,
		IsSale:      ps.IsSale,
		SaleStart:   ps.SaleStart,
		SaleEnd:     ps.SaleEnd,
		CountDown:   ps.CountDown,
		ServerTs:    ps.ServerTs,
	}
}

func screenToProtocol(s getv2.ScreenState) protocol.ScreenDiff {
	return protocol.ScreenDiff{
		ScreenID:  s.ScreenID,
		Name:      s.Name,
		StartTime: s.StartTime,
		Flag:      s.Flag,
		FlagText:  s.FlagText,
		Clickable: s.Clickable,
		SaleStart: s.SaleStart,
		SaleEnd:   s.SaleEnd,
	}
}
