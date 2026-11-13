package events

import (
	"testing"

	"github.com/WittF/fnmdp/shared/protocol"
)

func TestClassifySKU(t *testing.T) {
	old := &protocol.Fingerprint{Flag: 4, Clickable: false, PriceFen: 8800}
	restock := &protocol.Fingerprint{Flag: 2, Clickable: true, PriceFen: 8800}
	eventType, severity, ok := ClassifySKU(old, restock, "在售", 0)
	if !ok || eventType != protocol.EventSkuRestock || severity != protocol.SeverityCritical {
		t.Fatalf("restock = %s/%s/%v", eventType, severity, ok)
	}

	price := &protocol.Fingerprint{Flag: 4, Clickable: false, PriceFen: 9900}
	eventType, severity, ok = ClassifySKU(old, price, "已售罄", 0)
	if !ok || eventType != protocol.EventSkuPriceChange || severity != protocol.SeverityNotable {
		t.Fatalf("price = %s/%s/%v", eventType, severity, ok)
	}

	eventType, severity, ok = ClassifySKU(restock, old, "已售罄", 0)
	if !ok || eventType != protocol.EventSkuSoldout || severity != protocol.SeverityNotable {
		t.Fatalf("soldout = %s/%s/%v", eventType, severity, ok)
	}
}

func TestClassifySKUInventory(t *testing.T) {
	// num restock: 0→≥1 (someone cancelled) — sensitive even when sale_flag lags.
	soldout := &protocol.Fingerprint{Flag: 2, Clickable: true, PriceFen: 8800, Num: 0}
	back := &protocol.Fingerprint{Flag: 2, Clickable: true, PriceFen: 8800, Num: 3}
	if et, sev, ok := ClassifySKU(soldout, back, "在售", 8); !ok || et != protocol.EventSkuRestock || sev != protocol.SeverityCritical {
		t.Fatalf("num restock = %s/%s/%v", et, sev, ok)
	}

	// low stock: dropped below the per-order cap (顶格→告急) while still buyable.
	ample := &protocol.Fingerprint{Flag: 2, Clickable: true, PriceFen: 8800, Num: 8}
	low := &protocol.Fingerprint{Flag: 2, Clickable: true, PriceFen: 8800, Num: 5}
	if et, sev, ok := ClassifySKU(ample, low, "在售", 8); !ok || et != protocol.EventSkuLowStock || sev != protocol.SeverityNotable {
		t.Fatalf("low stock = %s/%s/%v", et, sev, ok)
	}

	// further drain inside the low band is not a discrete event (state_update only).
	lower := &protocol.Fingerprint{Flag: 2, Clickable: true, PriceFen: 8800, Num: 4}
	if et, _, ok := ClassifySKU(low, lower, "在售", 8); ok {
		t.Fatalf("intra-band drain should be silent, got %s", et)
	}
}

func TestClassifySKUIgnoresUnsupportedFlagTransition(t *testing.T) {
	// A SKU drifting into 不可售(5) must not emit a phantom 开售: the flag changed but
	// the new state is not buyable and the wording isn't 售罄.
	unavailable := &protocol.Fingerprint{Flag: 5, Clickable: false, PriceFen: 8800}
	presale := &protocol.Fingerprint{Flag: 1, Clickable: false, PriceFen: 8800}
	if et, _, ok := ClassifySKU(presale, unavailable, "不可售", 0); ok {
		t.Fatalf("未开售→不可售 should be silent, got %s", et)
	}
	soldout := &protocol.Fingerprint{Flag: 4, Clickable: false, PriceFen: 8800}
	if et, _, ok := ClassifySKU(soldout, unavailable, "不可售", 0); ok {
		t.Fatalf("已售罄→不可售 should be silent, got %s", et)
	}
	// Sanity: a genuine 不可售→在售 (clickable) still fires 开售.
	onsale := &protocol.Fingerprint{Flag: 2, Clickable: true, PriceFen: 8800}
	if et, _, ok := ClassifySKU(unavailable, onsale, "在售", 0); !ok || et != protocol.EventSkuOnsale {
		t.Fatalf("不可售→在售 = %s/%v, want sku_onsale", et, ok)
	}
}

func TestClassifyScreen(t *testing.T) {
	// A genuine removal of a tracked screen emits screen_removed.
	et, sev, ok := ClassifyScreen(2, true, true, 0, false, true)
	if !ok || et != protocol.EventScreenRemoved || sev != protocol.SeverityInfo {
		t.Fatalf("removal of tracked screen = %s/%s/%v", et, sev, ok)
	}
	// A removal for a screen the master no longer tracks (existsBefore=false) is a
	// duplicate cross-node report — stay silent so N nodes yield one screen_removed.
	if _, _, ok := ClassifyScreen(0, false, false, 0, false, true); ok {
		t.Fatal("removal of untracked screen should not emit")
	}
	// First sighting of a screen is still an add (regression guard for the removed branch).
	if et, _, ok := ClassifyScreen(0, false, false, 1, false, false); !ok || et != protocol.EventScreenAdded {
		t.Fatalf("first sighting = %s/%v", et, ok)
	}
}

func TestClassifyProject(t *testing.T) {
	old := ProjectComparable{Flag: 1, Hot: false, SalableLow: 100}
	cur := ProjectComparable{Flag: 2, Hot: false, SalableLow: 100}
	eventType, severity, ok := ClassifyProject(old, cur, true)
	if !ok || eventType != protocol.EventProjectOnsale || severity != protocol.SeverityCritical {
		t.Fatalf("onsale = %s/%s/%v", eventType, severity, ok)
	}

	cur = ProjectComparable{Flag: 1, Hot: true, SalableLow: 100}
	eventType, severity, ok = ClassifyProject(old, cur, true)
	if !ok || eventType != protocol.EventProjectHotOn {
		t.Fatalf("hot = %s/%s/%v", eventType, severity, ok)
	}
}

func TestClassifyProjectIgnoresTransientUnsupportedFlag(t *testing.T) {
	old := ProjectComparable{Flag: 1, Hot: true, SalableLow: 38000, SalableHigh: 138000}
	transient := ProjectComparable{Flag: 5, Hot: true, SalableLow: 0, SalableHigh: 0}
	if et, _, ok := ClassifyProject(old, transient, true); ok {
		t.Fatalf("transient unsupported flag should be silent, got %s", et)
	}
	if et, _, ok := ClassifyProject(transient, old, true); ok {
		t.Fatalf("recovery from transient unsupported flag should be silent, got %s", et)
	}
}

func TestClassifyProjectPriceBand(t *testing.T) {
	// Salable band change → project_price_band_change.
	old := ProjectComparable{Flag: 4, SalableLow: 12800, SalableHigh: 88800}
	cur := ProjectComparable{Flag: 4, SalableLow: 12800, SalableHigh: 32800}
	eventType, severity, ok := ClassifyProject(old, cur, true)
	if !ok || eventType != protocol.EventProjectPriceBandChange || severity != protocol.SeverityNotable {
		t.Fatalf("salable change = %s/%s/%v, want project_price_band_change/notable/true", eventType, severity, ok)
	}

	// Regression guard: salable band identical, only the (excluded) catalog band
	// differs — must NOT emit. ProjectComparable no longer carries PriceLow/High,
	// so a mall fallback that zeroed them collapses to "no change" here.
	same := ProjectComparable{Flag: 4, SalableLow: 12800, SalableHigh: 88800}
	_, _, ok = ClassifyProject(same, same, true)
	if ok {
		t.Fatal("identical salable band must not emit project_price_band_change")
	}
}
