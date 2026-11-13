package getv2

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadSample(t *testing.T) *RawResponse {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "getv2_sample.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var raw RawResponse
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return &raw
}

func TestNormalizeProjectMeta(t *testing.T) {
	ps := Normalize(loadSample(t))
	if ps.ProjectID != 1001701 {
		t.Errorf("ProjectID = %d, want 1001701", ps.ProjectID)
	}
	if ps.Name != "BML 2026 演唱会" {
		t.Errorf("Name = %q", ps.Name)
	}
	if ps.Flag != 2 {
		t.Errorf("Flag = %d, want 2", ps.Flag)
	}
	if ps.FlagText != "在售" {
		t.Errorf("FlagText = %q, want 在售", ps.FlagText)
	}
	if ps.Status != 1 || ps.IsSale != 1 {
		t.Errorf("Status/IsSale = %d/%d, want 1/1", ps.Status, ps.IsSale)
	}
	if ps.ServerTs != 1750100000 {
		t.Errorf("ServerTs = %d", ps.ServerTs)
	}
	if ps.SaleStart != 1750000000 || ps.SaleEnd != 1751000000 || ps.CountDown != 0 {
		t.Errorf("sale window/countdown = %d/%d/%d", ps.SaleStart, ps.SaleEnd, ps.CountDown)
	}
	if !ps.HotProject {
		t.Error("HotProject = false, want true")
	}
	if ps.WishCount != 45231 {
		t.Errorf("WishCount = %d", ps.WishCount)
	}
	if len(ps.Screens) != 2 {
		t.Fatalf("Screens = %d, want 2", len(ps.Screens))
	}
	if len(ps.Skus) != 4 {
		t.Fatalf("flattened Skus = %d, want 4", len(ps.Skus))
	}
	first := ps.Screens[0]
	if first.SaleStart != 1750000000 || first.SaleEnd != 1751000000 {
		t.Errorf("screen sale window = %d/%d", first.SaleStart, first.SaleEnd)
	}
}

func TestNormalizeSkuTruth(t *testing.T) {
	ps := Normalize(loadSample(t))
	bySku := map[int64]SkuState{}
	for _, s := range ps.Skus {
		bySku[s.SkuID] = s
	}

	onsale := bySku[330001]
	if onsale.Flag != 2 || !onsale.Clickable || onsale.PriceFen != 138000 {
		t.Errorf("S级 = %+v", onsale)
	}
	// Inventory signal: num = min(stock, limit); static_limit.num = per-order cap (§5.1).
	if onsale.Num != 5 || onsale.LimitNum != 8 {
		t.Errorf("S级 inventory = num %d / limit %d, want 5 / 8", onsale.Num, onsale.LimitNum)
	}
	if onsale.ServerTs != 1750100000 {
		t.Errorf("S级 ServerTs should come from data.current_time, got %d", onsale.ServerTs)
	}
	if onsale.SaleStart != 1750000000 {
		t.Errorf("S级 SaleStart = %d, want 1750000000", onsale.SaleStart)
	}

	soldout := bySku[330002]
	if soldout.Flag != 4 || soldout.Clickable {
		t.Errorf("VIP = %+v, want sold-out & not clickable", soldout)
	}
	if soldout.FlagText != "已售罄" {
		t.Errorf("VIP FlagText = %q, want 已售罄", soldout.FlagText)
	}

	// Source wording must be preserved verbatim to tell 暂时售罄 from 已售罄.
	if bySku[330003].FlagText != "暂时售罄" {
		t.Errorf("D级 FlagText = %q, want 暂时售罄", bySku[330003].FlagText)
	}
}

// TestNormalizeNullTolerance: a SKU with null sale_flag/price/clickable must
// normalize to zero values, not panic (spec §5.6).
func TestNormalizeNullTolerance(t *testing.T) {
	ps := Normalize(loadSample(t))
	var student SkuState
	for _, s := range ps.Skus {
		if s.SkuID == 330004 {
			student = s
		}
	}
	if student.SkuID != 330004 {
		t.Fatal("学生票 SKU not found")
	}
	if student.Flag != 0 || student.Clickable || student.PriceFen != 0 {
		t.Errorf("null SKU = %+v, want zero values", student)
	}
	if student.FlagText != "" {
		t.Errorf("null SKU FlagText = %q, want empty", student.FlagText)
	}
}

func TestNormalizeProjectSaleFlagObjectFallback(t *testing.T) {
	var raw RawResponse
	body := []byte(`{
		"errno": 0,
		"data": {
			"id": 100,
			"name": "object fallback",
			"sale_flag": {"number": 8, "display_name": "已结束"}
		}
	}`)
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode object sale_flag: %v", err)
	}
	ps := Normalize(&raw)
	if ps.Flag != 8 || ps.FlagText != "已结束" {
		t.Fatalf("project = %+v, want flag 8 已结束", ps)
	}
}

func TestNormalizeDerivesProjectSaleWindowFromScreens(t *testing.T) {
	raw := &RawResponse{
		Errno: 0,
		Data: &RawData{
			ID:          100,
			Name:        "mall-shaped",
			CurrentTime: int64p(1750100000),
			ScreenList: []RawScreen{
				{
					ID:        10,
					SaleStart: int64p(1750200000),
					SaleEnd:   int64p(1751200000),
				},
				{
					ID:        11,
					SaleStart: int64p(1750000000),
					SaleEnd:   int64p(1751300000),
				},
			},
		},
	}

	ps := Normalize(raw)
	if ps.SaleStart != 1750000000 || ps.SaleEnd != 1751300000 {
		t.Fatalf("derived sale window = %d/%d, want 1750000000/1751300000", ps.SaleStart, ps.SaleEnd)
	}

	explicitStart, explicitEnd := int64(1750300000), int64(1751100000)
	raw.Data.SaleStart = &explicitStart
	raw.Data.SaleEnd = &explicitEnd
	ps = Normalize(raw)
	if ps.SaleStart != explicitStart || ps.SaleEnd != explicitEnd {
		t.Fatalf("explicit sale window = %d/%d, want %d/%d", ps.SaleStart, ps.SaleEnd, explicitStart, explicitEnd)
	}
}

func TestNormalizeNilSafe(t *testing.T) {
	if got := Normalize(nil); got.ProjectID != 0 || len(got.Skus) != 0 {
		t.Errorf("Normalize(nil) = %+v, want empty", got)
	}
	if got := Normalize(&RawResponse{}); got.ProjectID != 0 || len(got.Skus) != 0 {
		t.Errorf("Normalize(empty) = %+v, want empty", got)
	}
}

func int64p(v int64) *int64 { return &v }
