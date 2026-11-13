package getv2

// SkuState is the smallest unit of truth the monitor tracks (spec §2.3). Truth
// lives here — sale_flag.number + clickable, with num as the inventory signal.
type SkuState struct {
	ProjectID int64
	ScreenID  int64
	SkuID     int64
	Desc      string // ticket_list[].desc
	Flag      int    // sale_flag.number: 1未开售 2在售 4售罄 8其他
	FlagText  string // sale_flag.display_name; source wording, preserved for display
	Clickable bool
	PriceFen  int   // unit: fen; display /100 (§5.4)
	Num       int   // ticket_list[].num = min(stock, limit); core inventory signal (§5.1)
	LimitNum  int   // ticket_list[].static_limit.num = per-order purchase cap
	SaleStart int64 // ticket_list[].saleStart, seconds
	ServerTs  int64 // = data.current_time, seconds (§5.3)
}

// ScreenState is the session-level aggregate (spec §2.2). A project may be
// on-sale overall while a screen is sold out, so screen flags are carried too.
type ScreenState struct {
	ScreenID  int64
	Name      string
	StartTime int64
	Flag      int
	FlagText  string
	Clickable bool
	SaleStart int64
	SaleEnd   int64
	Skus      []SkuState
}

// ProjectState is the full normalized snapshot of one getV2 poll: project meta,
// the per-screen tree, and a flattened SKU slice for timeseries/dedup.
type ProjectState struct {
	ProjectID        int64
	Name             string
	Flag             int
	FlagText         string
	Status           int
	IsSale           int
	SaleStart        int64
	SaleEnd          int64
	CountDown        int64
	ServerTs         int64
	PriceLowFen      int
	PriceHighFen     int
	SalablePriceLow  int
	SalablePriceHigh int
	WishCount        int64
	HotProject       bool
	Screens          []ScreenState
	Skus             []SkuState // flattened across all screens
}

// Normalize flattens a raw getV2 response into a ProjectState. It is fully
// null-tolerant: missing pointers default to zero, missing slices to empty. A
// nil response or nil data yields an empty ProjectState rather than panicking.
func Normalize(raw *RawResponse) ProjectState {
	if raw == nil || raw.Data == nil {
		return ProjectState{}
	}
	d := raw.Data
	serverTs := derefInt64(d.CurrentTime)

	ps := ProjectState{
		ProjectID:        d.ID,
		Name:             d.Name,
		Flag:             derefInt(d.SaleFlagNumber),
		Status:           derefInt(d.Status),
		IsSale:           derefInt(d.IsSale),
		SaleStart:        derefInt64(d.SaleStart),
		SaleEnd:          derefInt64(d.SaleEnd),
		CountDown:        derefInt64(d.CountDown),
		ServerTs:         serverTs,
		PriceLowFen:      derefInt(d.PriceLow),
		PriceHighFen:     derefInt(d.PriceHigh),
		SalablePriceLow:  derefInt(d.SalablePriceLow),
		SalablePriceHigh: derefInt(d.SalablePriceHigh),
		HotProject:       derefBool(d.HotProject),
	}
	if d.WishInfo != nil {
		ps.WishCount = derefInt64(d.WishInfo.Count)
	}
	if d.SaleFlag != nil {
		if d.SaleFlag.Number != nil && d.SaleFlagNumber == nil {
			ps.Flag = derefInt(d.SaleFlag.Number)
		}
		ps.FlagText = d.SaleFlag.Text
	}
	if ps.FlagText == "" {
		ps.FlagText = flagText(ps.Flag)
	}

	var derivedSaleStart int64
	var derivedSaleEnd int64
	for _, rs := range d.ScreenList {
		ss := ScreenState{
			ScreenID:  rs.ID,
			Name:      rs.Name,
			StartTime: derefInt64(rs.StartTime),
			Clickable: derefBool(rs.Clickable),
			SaleStart: derefInt64(rs.SaleStart),
			SaleEnd:   derefInt64(rs.SaleEnd),
		}
		if ss.SaleStart > 0 && (derivedSaleStart == 0 || ss.SaleStart < derivedSaleStart) {
			derivedSaleStart = ss.SaleStart
		}
		if ss.SaleEnd > derivedSaleEnd {
			derivedSaleEnd = ss.SaleEnd
		}
		if rs.SaleFlag != nil {
			ss.Flag = derefInt(rs.SaleFlag.Number)
			ss.FlagText = rs.SaleFlag.DisplayName
		}
		for _, rt := range rs.TicketList {
			sku := SkuState{
				ProjectID: d.ID,
				ScreenID:  rs.ID,
				SkuID:     rt.ID,
				Desc:      rt.Desc,
				Clickable: derefBool(rt.Clickable),
				PriceFen:  derefInt(rt.Price),
				Num:       derefInt(rt.Num),
				SaleStart: derefInt64(rt.SaleStart),
				ServerTs:  serverTs,
			}
			if rt.StaticLimit != nil {
				sku.LimitNum = derefInt(rt.StaticLimit.Num)
			}
			if rt.SaleFlag != nil {
				sku.Flag = derefInt(rt.SaleFlag.Number)
				sku.FlagText = rt.SaleFlag.DisplayName
			}
			ss.Skus = append(ss.Skus, sku)
			ps.Skus = append(ps.Skus, sku)
		}
		ps.Screens = append(ps.Screens, ss)
	}
	if ps.SaleStart == 0 {
		ps.SaleStart = derivedSaleStart
	}
	if ps.SaleEnd == 0 {
		ps.SaleEnd = derivedSaleEnd
	}
	return ps
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func derefBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// flagText is a fallback label used only when the upstream display_name is absent
// (the real wording is preferred verbatim). Codes follow the SaleFlag enum.
func flagText(flag int) string {
	switch flag {
	case SaleFlagNotForSale:
		return "未开售"
	case SaleFlagOnSale:
		return "在售"
	case SaleFlagStop:
		return "停售"
	case SaleFlagSoldOut:
		return "已售罄"
	case SaleFlagUnavailable:
		return "不可售"
	case SaleFlagShortOfStock:
		return "余票紧张"
	case SaleFlagSoldOutTemp:
		return "暂时售罄"
	case SaleFlagFinished:
		return "已结束"
	case SaleFlagCancel:
		return "已取消"
	default:
		return ""
	}
}
