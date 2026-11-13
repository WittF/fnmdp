package state

import (
	"testing"

	"github.com/WittF/fnmdp/shared/getv2"
)

func TestDedupKeyStable(t *testing.T) {
	k1 := DedupKey(1001701, 220001, 330001)
	k2 := DedupKey(1001701, 220001, 330001)
	if k1 != k2 {
		t.Errorf("DedupKey not stable: %q vs %q", k1, k2)
	}
	if k1 == DedupKey(1001701, 220001, 330002) {
		t.Error("different SKUs collided")
	}
	if want := "1001701:220001:330001"; k1 != want {
		t.Errorf("DedupKey = %q, want %q", k1, want)
	}
}

func TestScreenDedupKeyStable(t *testing.T) {
	k1 := ScreenDedupKey(1001701, 220001)
	k2 := ScreenDedupKey(1001701, 220001)
	if k1 != k2 {
		t.Errorf("ScreenDedupKey not stable: %q vs %q", k1, k2)
	}
	if k1 == ScreenDedupKey(1001701, 220002) {
		t.Error("different screens collided")
	}
	if want := "1001701:220001"; k1 != want {
		t.Errorf("ScreenDedupKey = %q, want %q", k1, want)
	}
}

func TestFingerprintAndChanged(t *testing.T) {
	base := getv2.SkuState{Flag: 4, Clickable: false, PriceFen: 88000}
	fp := Fingerprint(base)

	// Restock: sold-out → on-sale (the highest-value monitor event, spec §2.4).
	restock := Fingerprint(getv2.SkuState{Flag: 2, Clickable: true, PriceFen: 88000})
	if !Changed(fp, restock) {
		t.Error("restock should be a change")
	}

	// Price change alone is tracked.
	priceOnly := Fingerprint(getv2.SkuState{Flag: 4, Clickable: false, PriceFen: 90000})
	if !Changed(fp, priceOnly) {
		t.Error("price change should be a change")
	}

	// Display-only fields (Desc/FlagText/ServerTs) must NOT register as a change.
	cosmetic := Fingerprint(getv2.SkuState{
		Flag: 4, Clickable: false, PriceFen: 88000,
		Desc: "renamed", FlagText: "暂时售罄", ServerTs: 1750100099,
	})
	if Changed(fp, cosmetic) {
		t.Error("cosmetic-only diff should NOT be a change")
	}
}

func TestProjectAndScreenFingerprintChanged(t *testing.T) {
	project := ProjectStateFingerprint(getv2.ProjectState{
		Flag: 2, HotProject: false, Status: 1, IsSale: 1,
		PriceLowFen: 12800, PriceHighFen: 138000,
		SalablePriceLow: 12800, SalablePriceHigh: 88000,
		SaleStart: 1750000000, SaleEnd: 1751000000,
	})
	hot := ProjectStateFingerprint(getv2.ProjectState{
		Flag: 2, HotProject: true, Status: 1, IsSale: 1,
		PriceLowFen: 12800, PriceHighFen: 138000,
		SalablePriceLow: 12800, SalablePriceHigh: 88000,
		SaleStart: 1750000000, SaleEnd: 1751000000,
	})
	if !ProjectChanged(project, hot) {
		t.Error("project heat change should be tracked")
	}

	// Regression guard: the mall fallback omits priceLow/priceHigh (→ 0). A poll
	// that differs ONLY in PriceLowFen/PriceHighFen must NOT count as a change,
	// else every fallback poll churns a phantom project_price_band_change.
	priceOnly := ProjectStateFingerprint(getv2.ProjectState{
		Flag: 2, PriceLowFen: 0, PriceHighFen: 0,
		SalablePriceLow: 12800, SalablePriceHigh: 88000,
	})
	priceFull := ProjectStateFingerprint(getv2.ProjectState{
		Flag: 2, PriceLowFen: 12800, PriceHighFen: 138000,
		SalablePriceLow: 12800, SalablePriceHigh: 88000,
	})
	if ProjectChanged(priceOnly, priceFull) {
		t.Error("price_low/price_high-only change must be ignored (mall fallback omits them)")
	}
	// The salable band IS tracked: a salable change must still register.
	salableChanged := ProjectStateFingerprint(getv2.ProjectState{
		Flag: 2, SalablePriceLow: 12800, SalablePriceHigh: 32800,
	})
	if !ProjectChanged(priceOnly, salableChanged) {
		t.Error("salable price band change should be tracked")
	}

	screen := ScreenStateFingerprint(getv2.ScreenState{Flag: 1, Clickable: false})
	onsale := ScreenStateFingerprint(getv2.ScreenState{Flag: 2, Clickable: true})
	if !ScreenChanged(screen, onsale) {
		t.Error("screen sale change should be tracked")
	}
}
