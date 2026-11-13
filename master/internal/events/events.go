package events

import (
	"strings"

	"github.com/WittF/fnmdp/shared/protocol"
)

type Level string

const (
	LevelProject Level = "project"
	LevelScreen  Level = "screen"
	LevelSKU     Level = "sku"
	LevelOps     Level = "ops"
)

// ClassifySKU maps an old→new SKU fingerprint transition to an event. limitNum is
// the current per-order purchase cap (static_limit.num), used to detect the
// low-stock (顶格→跌破限购) crossing — the highest-value early sold-out warning
// (spec §5.1). It is also a more sensitive restock signal: num 0→≥1 (someone
// cancelled) is caught even when the coarse sale_flag still lags.
func ClassifySKU(old, new *protocol.Fingerprint, flagText string, limitNum int) (protocol.EventType, protocol.Severity, bool) {
	switch {
	case old == nil && new != nil:
		if new.Flag == 2 || new.Clickable {
			return protocol.EventSkuAdded, protocol.SeverityCritical, true
		}
		return protocol.EventSkuAdded, protocol.SeverityInfo, true
	case old != nil && new == nil:
		return protocol.EventSkuRemoved, protocol.SeverityInfo, true
	case old == nil || new == nil:
		return "", "", false
	}
	if old.PriceFen != new.PriceFen {
		return protocol.EventSkuPriceChange, protocol.SeverityNotable, true
	}
	// Restock: sold-out→buyable (flag/clickable), or the more sensitive num 0→≥1.
	if old.Flag == 4 && (new.Flag == 2 || new.Clickable) {
		return protocol.EventSkuRestock, protocol.SeverityCritical, true
	}
	if old.Num == 0 && new.Num >= 1 && (new.Clickable || new.Flag == 2) {
		return protocol.EventSkuRestock, protocol.SeverityCritical, true
	}
	if old.Flag == 1 && (new.Flag == 2 || new.Clickable) {
		return protocol.EventSkuOnsale, protocol.SeverityCritical, true
	}
	if !old.Clickable && new.Clickable {
		return protocol.EventSkuOnsale, protocol.SeverityCritical, true
	}
	if old.Flag == 2 && new.Flag == 4 {
		return protocol.EventSkuSoldout, protocol.SeverityNotable, true
	}
	// Low stock: inventory just dropped below the purchase cap while still buyable.
	// From here num is the exact remaining count — the early sold-out warning.
	if limitNum > 0 && new.Num > 0 && new.Num < limitNum && old.Num >= limitNum {
		return protocol.EventSkuLowStock, protocol.SeverityNotable, true
	}
	if old.Flag != new.Flag || old.Clickable != new.Clickable {
		// A buyable new state is an 开售; 售罄 wording is a 售罄. Anything else is an
		// unsupported/non-buyable transition (e.g. →不可售(5)/停售(3)) — stay silent
		// rather than emit a phantom 开售.
		if new.Clickable || new.Flag == 2 {
			return protocol.EventSkuOnsale, protocol.SeverityNotable, true
		}
		if strings.Contains(flagText, "售罄") {
			return protocol.EventSkuSoldout, protocol.SeverityNotable, true
		}
		return "", "", false
	}
	return "", "", false
}

func ClassifyScreen(oldFlag int, oldClickable bool, existsBefore bool, newFlag int, newClickable bool, removed bool) (protocol.EventType, protocol.Severity, bool) {
	if removed {
		// Removing a screen the master never tracked (existsBefore=false) is a
		// duplicate cross-node report — the first node's removal already emitted the
		// event and deleted it. Stay silent, mirroring ClassifySKU's old==nil guard.
		if !existsBefore {
			return "", "", false
		}
		return protocol.EventScreenRemoved, protocol.SeverityInfo, true
	}
	if !existsBefore {
		return protocol.EventScreenAdded, protocol.SeverityInfo, true
	}
	if oldFlag != 2 && (newFlag == 2 || (!oldClickable && newClickable)) {
		return protocol.EventScreenOnsale, protocol.SeverityCritical, true
	}
	if oldFlag == 2 && newFlag == 4 {
		return protocol.EventScreenSoldout, protocol.SeverityNotable, true
	}
	return "", "", false
}

func ClassifyProject(old, cur ProjectComparable, existsBefore bool) (protocol.EventType, protocol.Severity, bool) {
	if !existsBefore {
		return protocol.EventMonitorAdded, protocol.SeverityOps, true
	}
	if !old.Hot && cur.Hot {
		return protocol.EventProjectHotOn, protocol.SeverityNotable, true
	}
	if old.Hot && !cur.Hot {
		return protocol.EventProjectHotOff, protocol.SeverityInfo, true
	}
	if old.Flag != 2 && cur.Flag == 2 {
		return protocol.EventProjectOnsale, protocol.SeverityCritical, true
	}
	if old.Flag == 2 && cur.Flag == 4 {
		return protocol.EventProjectSoldout, protocol.SeverityNotable, true
	}
	// getV2 can briefly return project sale_flag_number=5 ("不可售") around
	// deploy/restart/fallback edges while the SKU/screen states remain normal.
	// Treat unsupported project flags as non-authoritative so they do not churn
	// 1↔5 "上下架" alerts or follow-on recovery alerts.
	if !authoritativeProjectFlag(old.Flag) || !authoritativeProjectFlag(cur.Flag) {
		return "", "", false
	}
	// Flag is the authoritative project state (未开售/在售/售罄/结束). Status/IsSale
	// (项目是否上架可售) are excluded: near-static and absent from the mall fallback
	// (→ 0), so comparing them churned phantom 上下架 events on fallback/restart.
	if old.Flag != cur.Flag {
		return protocol.EventProjectStatusChange, protocol.SeverityNotable, true
	}
	// Only the salable band drives this event. PriceLow/PriceHigh (full catalog
	// range) are excluded: the mall fallback omits them (→ 0) while getV2 returns
	// them, so comparing them flipped value↔0 on every fallback poll and churned
	// phantom 价格带变动. A genuine catalog reprice still surfaces as sku_price_change.
	if old.SalableLow != cur.SalableLow || old.SalableHigh != cur.SalableHigh {
		return protocol.EventProjectPriceBandChange, protocol.SeverityNotable, true
	}
	return "", "", false
}

type ProjectComparable struct {
	Flag        int
	Hot         bool
	SalableLow  int
	SalableHigh int
}

func authoritativeProjectFlag(flag int) bool {
	switch flag {
	case 1, 2, 4, 8:
		return true
	default:
		return false
	}
}
