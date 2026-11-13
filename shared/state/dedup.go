// Package state holds the dedup primitives shared by all three tiers:
// L1 per-node timeseries, L2 cross-node Valkey window, L3 authoritative master
// state machine. This package defines the stable key + the change fingerprint.
package state

import (
	"strconv"

	"github.com/WittF/fnmdp/shared/getv2"
)

// DedupKey is the stable identity of one SKU's timeseries: project/screen/sku.
// Used as the timeseries id and the Valkey dedup-window key.
func DedupKey(projectID, screenID, skuID int64) string {
	return strconv.FormatInt(projectID, 10) + ":" +
		strconv.FormatInt(screenID, 10) + ":" +
		strconv.FormatInt(skuID, 10)
}

// ScreenDedupKey is the stable identity of one screen's timeseries.
func ScreenDedupKey(projectID, screenID int64) string {
	return strconv.FormatInt(projectID, 10) + ":" +
		strconv.FormatInt(screenID, 10)
}

// StateFingerprint is the minimal set of fields whose change is worth recording
// or alerting on. Price is included because a price change is a tracked event;
// FlagText/Desc are display-only and deliberately excluded so cosmetic wording
// tweaks don't churn the timeseries. Authoritative truth = Flag + Clickable.
// Num (available count) and LimitNum (per-order cap) are included so inventory
// drains and restocks (spec §5.1) propagate; Num is self-limiting (it stays
// constant at LimitNum while ample, only moving in the low-stock band).
type StateFingerprint struct {
	Flag      int
	Clickable bool
	PriceFen  int
	Num       int
	LimitNum  int
}

// ProjectFingerprint is the project-level set whose change should cause L1 to
// send a poll_result. Wish/countdown are intentionally not raw dimensions here;
// milestone/bucket logic can be layered on top without sending every tick.
// Status/IsSale (getV2 "项目是否上架可售", spec §2) are deliberately excluded:
// they are a near-static listed-or-not flag with no dynamic signal beyond Flag,
// and the mall fallback omits them (→ 0), so including them made every fallback
// poll and every master restart churn a phantom project_status_change.
// PriceLow/PriceHigh (the full catalog price range) are excluded for the same
// reason: the mall fallback omits priceLow/priceHigh (→ 0) while getV2 returns
// them, so on every fallback poll they flip value↔0 and churn a phantom
// project_price_band_change. The salable band (both endpoints return it) carries
// the meaningful "still-buyable price range" signal, and a genuine catalog
// reprice still surfaces as an SKU-level sku_price_change.
type ProjectFingerprint struct {
	Flag             int
	Hot              bool
	SalablePriceLow  int
	SalablePriceHigh int
	SaleStart        int64
	SaleEnd          int64
}

// ScreenFingerprint is the screen-level set whose change should cause L1 to
// include a screen_diff.
type ScreenFingerprint struct {
	Flag      int
	Clickable bool
	SaleStart int64
	SaleEnd   int64
}

// Fingerprint extracts the change-significant fields from a SkuState.
func Fingerprint(s getv2.SkuState) StateFingerprint {
	return StateFingerprint{
		Flag:      s.Flag,
		Clickable: s.Clickable,
		PriceFen:  s.PriceFen,
		Num:       s.Num,
		LimitNum:  s.LimitNum,
	}
}

// ProjectStateFingerprint extracts change-significant project-level fields.
func ProjectStateFingerprint(p getv2.ProjectState) ProjectFingerprint {
	return ProjectFingerprint{
		Flag:             p.Flag,
		Hot:              p.HotProject,
		SalablePriceLow:  p.SalablePriceLow,
		SalablePriceHigh: p.SalablePriceHigh,
		SaleStart:        p.SaleStart,
		SaleEnd:          p.SaleEnd,
	}
}

// ScreenStateFingerprint extracts change-significant screen-level fields.
func ScreenStateFingerprint(s getv2.ScreenState) ScreenFingerprint {
	return ScreenFingerprint{
		Flag:      s.Flag,
		Clickable: s.Clickable,
		SaleStart: s.SaleStart,
		SaleEnd:   s.SaleEnd,
	}
}

// Changed reports whether cur differs from prev in any tracked dimension.
func Changed(prev, cur StateFingerprint) bool {
	return prev != cur
}

func ProjectChanged(prev, cur ProjectFingerprint) bool {
	return prev != cur
}

func ScreenChanged(prev, cur ScreenFingerprint) bool {
	return prev != cur
}
