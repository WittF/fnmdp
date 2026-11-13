package getv2

// SaleFlag mirrors Bilibili 会员购's SALE_FLAG_NUM enum — the integer codes carried by
// sale_flag_number (project level) and sale_flag.number (screen/SKU level) in both the
// getV2 and mall responses. This is the canonical Go definition of flag semantics; the
// TS mirror lives in web/src/lib/flag.ts (SaleFlag) — keep the two in sync.
//
// SaleFlagSoldOutTemp (8, 暂时售罄) is retired upstream: as of ~2026-06 Bilibili returns
// SaleFlagSoldOut (4) for everything sold out and no longer emits 8. The constant is
// kept so pre-retirement history stays legible and a future re-introduction is a no-op.
const (
	SaleFlagNotForSale   = 1   // 未开售
	SaleFlagOnSale       = 2   // 在售 / 可购
	SaleFlagStop         = 3   // 停售
	SaleFlagSoldOut      = 4   // 已售罄
	SaleFlagUnavailable  = 5   // 不可售
	SaleFlagShortOfStock = 6   // 余票紧张（仍可买）
	SaleFlagSoldOutTemp  = 8   // 暂时售罄（已弃用）
	SaleFlagFinished     = 102 // 已结束
	SaleFlagCancel       = 106 // 已取消
)
