// Sale-state semantics live here (docs/bilibili-getv2-monitoring.md §2.1, §5.1). Truth = flag + clickable;
// num vs limit_num gives the 充足 / 告急 / 售罄 inventory state. Flag numbers mirror
// Bilibili's SALE_FLAG_NUM enum (see SaleFlag) — keep in sync with the Go constants
// in shared/getv2/saleflag.go.
import type { EventType, Severity } from "../protocol/types";

// Bilibili 会员购 sale_flag numbers (SALE_FLAG_NUM in their front-end). 8 (暂时售罄 /
// SOLD_OUT_TEMP) is retired upstream — kept only so pre-retirement history still
// renders; everything sold-out now arrives as SoldOut(4).
export const SaleFlag = {
  NotForSale: 1, // 未开售
  OnSale: 2, // 在售 / 可购
  Stop: 3, // 停售
  SoldOut: 4, // 已售罄
  Unavailable: 5, // 不可售
  ShortOfStock: 6, // 余票紧张（仍可买）
  SoldOutTemp: 8, // 暂时售罄（已弃用）
  Finished: 102, // 已结束
  Cancel: 106, // 已取消
} as const;

export function flagLabel(flag: number): string {
  switch (flag) {
    case SaleFlag.NotForSale:
      return "未开售";
    case SaleFlag.OnSale:
      return "在售";
    case SaleFlag.Stop:
      return "停售";
    case SaleFlag.SoldOut:
      return "已售罄";
    case SaleFlag.Unavailable:
      return "不可售";
    case SaleFlag.ShortOfStock:
      return "余票紧张";
    case SaleFlag.SoldOutTemp:
      return "暂时售罄";
    case SaleFlag.Finished:
      return "已结束";
    case SaleFlag.Cancel:
      return "已取消";
    default:
      return "—";
  }
}

// buyable = clickable and in an actively-purchasable flag. flag 6 (余票紧张) is still
// buyable, just low — Bilibili shows its buy button — so it counts.
export function buyable(flag: number, clickable: boolean): boolean {
  return clickable && (flag === SaleFlag.OnSale || flag === SaleFlag.ShortOfStock);
}

export type StatusKind =
  | "buyable"
  | "tight"
  | "presale"
  | "soldout"
  | "unavailable"
  | "ended"
  | "unknown";

// statusKind maps flag/clickable to a display status. flagText is no longer used —
// "暂时售罄" was retired upstream — but the param is kept so call sites stay stable.
export function statusKind(flag: number, clickable: boolean, _flagText?: string): StatusKind {
  if (flag === SaleFlag.ShortOfStock) return "tight";
  if (buyable(flag, clickable)) return "buyable";
  if (flag === SaleFlag.NotForSale) return "presale";
  if (flag === SaleFlag.SoldOut || flag === SaleFlag.SoldOutTemp) return "soldout";
  if (flag === SaleFlag.Unavailable) return "unavailable";
  if (flag === SaleFlag.Finished || flag === SaleFlag.Stop) return "ended";
  return "unknown";
}

// Tailwind text colors per status — green=buyable, amber=余票紧张 (watch it),
// rose=sold out, sky=not yet, zinc=不可售/ended/unknown. Used for badges/lights.
export const statusText: Record<StatusKind, string> = {
  buyable: "在售可买",
  tight: "余票紧张",
  presale: "未开售",
  soldout: "已售罄",
  unavailable: "不可售",
  ended: "已结束",
  unknown: "未知",
};

export const statusColor: Record<StatusKind, string> = {
  buyable: "text-emerald-400",
  tight: "text-amber-400",
  presale: "text-sky-400",
  soldout: "text-rose-500",
  unavailable: "text-zinc-400",
  ended: "text-zinc-500",
  unknown: "text-zinc-400",
};

export const statusDot: Record<StatusKind, string> = {
  buyable: "bg-emerald-400",
  tight: "bg-amber-400",
  presale: "bg-sky-400",
  soldout: "bg-rose-500",
  unavailable: "bg-zinc-500",
  ended: "bg-zinc-600",
  unknown: "bg-zinc-500",
};

export type StockKind = "ample" | "low" | "out" | "unknown";
export interface Stock {
  kind: StockKind;
  remaining: number;
}

// stockState derives 充足 (num>=limit) / 告急 (0<num<limit, num=exact remaining) /
// 售罄 (num=0) from num vs limit_num. With no limit info it falls back to flag.
export function stockState(num: number, limit: number, flag: number): Stock {
  if (limit <= 0) {
    if (flag === 4) return { kind: "out", remaining: 0 };
    return { kind: "unknown", remaining: num > 0 ? num : 0 };
  }
  if (num <= 0) return { kind: "out", remaining: 0 };
  if (num >= limit) return { kind: "ample", remaining: num };
  return { kind: "low", remaining: num };
}

export function stockLabel(s: Stock): string {
  switch (s.kind) {
    case "ample":
      return "余票充足";
    case "low":
      return `告急 ${s.remaining} 张`;
    case "out":
      return "无票";
    default:
      return "";
  }
}

export const stockColor: Record<StockKind, string> = {
  ample: "text-emerald-400",
  low: "text-amber-400",
  out: "text-rose-500",
  unknown: "text-zinc-500",
};

// Severity ordering + colors for the live feed and event chips.
export const severityRank: Record<Severity, number> = {
  critical: 0,
  notable: 1,
  info: 2,
  ops: 3,
};

export const severityColor: Record<Severity, string> = {
  critical: "text-rose-400 bg-rose-500/10 border-rose-500/30",
  notable: "text-amber-300 bg-amber-500/10 border-amber-500/30",
  info: "text-zinc-300 bg-zinc-500/10 border-zinc-500/30",
  ops: "text-sky-300 bg-sky-500/10 border-sky-500/30",
};

export const severityIcon: Record<Severity, string> = {
  critical: "🔴",
  notable: "🟡",
  info: "⚪",
  ops: "🔧",
};

// Human label per event type, Chinese (i18n decision: zh-only).
export const eventLabel: Record<EventType, string> = {
  sku_restock: "回流",
  sku_onsale: "开售",
  sku_added: "新档放出",
  sku_soldout: "售罄",
  sku_price_change: "改价",
  sku_threshold_change: "门槛变更",
  sku_low_stock: "余量告急",
  sku_removed: "下架",
  screen_onsale: "场次开售",
  screen_soldout: "场次售罄",
  screen_added: "新增场次",
  screen_removed: "移除场次",
  day_soldout: "整日售罄",
  project_hot_on: "转为热门",
  project_hot_off: "退出热门",
  project_onsale: "项目开售",
  project_presale: "项目预售",
  project_soldout: "项目售罄",
  project_window_open: "开售窗口",
  project_price_band_change: "价格带变动",
  project_status_change: "上下架",
  wish_milestone: "热度里程碑",
  risk_412: "风控封禁(412)",
  risk_429: "软限流(429)",
  node_online: "节点上线",
  node_offline: "节点下线",
  monitor_added: "新增监控",
  monitor_removed: "移除监控",
};
