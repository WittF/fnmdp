// Package getv2 is a read-only client + normalizer for bilibili 会员购's public
// project-detail endpoints. Mall item detail is the primary polling source; getV2
// remains the low-frequency independent fallback/check source.
// See docs/bilibili-getv2-monitoring.md.
package getv2

import "encoding/json"

// RawResponse mirrors the getV2 JSON envelope. All structs are deliberately
// tolerant: every optional field is a pointer or has omitempty, and unknown
// fields are ignored by encoding/json. Unauthenticated responses null out many
// fields (merchant/mid/venue traffic) and the schema drifts across versions, so
// parsing must never fail on a missing or unknown field (spec §5.6, §5.7).
type RawResponse struct {
	Errno   int      `json:"errno"`
	ErrTag  int      `json:"errtag"`
	Message string   `json:"msg"`
	Data    *RawData `json:"data"`
}

// RawData is the `data` object — project-level meta plus the screen list.
type RawData struct {
	ID               int64               `json:"id"`
	Name             string              `json:"name"`
	SaleFlagNumber   *int                `json:"sale_flag_number"`
	SaleFlag         *RawProjectSaleFlag `json:"sale_flag"`
	Status           *int                `json:"status"`
	IsSale           *int                `json:"is_sale"`
	SaleStart        *int64              `json:"sale_start"`
	SaleEnd          *int64              `json:"sale_end"`
	CurrentTime      *int64              `json:"current_time"`
	CountDown        *int64              `json:"count_down"`
	PriceLow         *int                `json:"price_low"`
	PriceHigh        *int                `json:"price_high"`
	SalablePriceLow  *int                `json:"salable_price_low"`
	SalablePriceHigh *int                `json:"salable_price_high"`
	HotProject       *bool               `json:"hotProject"`
	WishInfo         *RawWishInfo        `json:"wish_info"`
	ScreenList       []RawScreen         `json:"screen_list"`
}

// RawWishInfo carries the wishlist ("想去") count used as a heat indicator.
type RawWishInfo struct {
	Count *int64 `json:"count"`
}

// RawProjectSaleFlag is the project-level sale_flag. getV2 currently returns a
// display string here, while fallback sources may expose the older object shape.
type RawProjectSaleFlag struct {
	Number *int
	Text   string
}

// UnmarshalJSON accepts the known project-level sale_flag variants: string,
// {number, display_name}, and null.
func (f *RawProjectSaleFlag) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var text string
	if err := json.Unmarshal(b, &text); err == nil {
		f.Text = text
		return nil
	}
	var raw RawSaleFlag
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	f.Number = raw.Number
	f.Text = raw.DisplayName
	return nil
}

// RawSaleFlag is the {number, display_name} pair that appears at screen/SKU
// levels.
// number: 1=未开售 2=在售/预售 4=已售罄 8=其他/结束. display_name is the source
// wording, preserved so "暂时售罄" can be told apart from "已售罄/已结束".
type RawSaleFlag struct {
	Number      *int   `json:"number"`
	DisplayName string `json:"display_name"`
}

// RawScreen is one entry of screen_list[] — a performance session.
type RawScreen struct {
	ID         int64        `json:"id"`
	Name       string       `json:"name"`
	StartTime  *int64       `json:"start_time"`
	SaleStart  *int64       `json:"sale_start"`
	SaleEnd    *int64       `json:"sale_end"`
	SaleFlag   *RawSaleFlag `json:"saleFlag"`
	Clickable  *bool        `json:"clickable"`
	TicketList []RawTicket  `json:"ticket_list"`
}

// RawTicket is one entry of ticket_list[] — a SKU, the smallest monitored unit.
// Num = min(real stock, static_limit.num): topped out at the limit means "ample";
// below the limit it is the exact remaining count — the core inventory signal
// (§5.1, corrected 2026-06-20; the earlier "Num is always 0" assumption was wrong).
type RawTicket struct {
	ID          int64           `json:"id"`
	Desc        string          `json:"desc"`
	Price       *int            `json:"price"`
	Num         *int            `json:"num"`
	StaticLimit *RawStaticLimit `json:"static_limit"`
	SaleStart   *int64          `json:"saleStart"`
	SaleFlag    *RawSaleFlag    `json:"sale_flag"`
	Clickable   *bool           `json:"clickable"`
}

// RawStaticLimit carries static_limit.num — the per-order purchase cap that Num
// tops out at. Used to tell "ample" (num>=limit) from "low" (0<num<limit).
type RawStaticLimit struct {
	Num *int `json:"num"`
}
