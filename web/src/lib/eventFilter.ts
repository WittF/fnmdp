// Global, persistent event-type filter for the project-detail timeline. The
// timeline mixes ticketing signals (onsale/restock/soldout…) with noisy ops/risk
// events (429 soft-limit, node up/down). Users pick which types to show; the
// preference lives in localStorage and applies to every project's timeline.
//
// The persisted model stores the HIDDEN set (not the visible set) so that any
// event type added to the protocol later defaults to visible — no migration.
import { atom } from "nanostores";
import type { EventType } from "../protocol/types";

export interface EventFilter {
  hidden: EventType[];
}

// Default-hidden = the whole ops/risk group. Opening the page immediately quiets
// 429 spam while keeping all ticketing events visible.
export const DEFAULT_HIDDEN: EventType[] = [
  "risk_412",
  "risk_429",
  "node_online",
  "node_offline",
  "monitor_added",
  "monitor_removed",
];

// Event types grouped for the filter UI. Flattened, this must equal EVENT_TYPES
// (guarded by eventFilter.test.ts) so the picker is exhaustive. Order mirrors
// EVENT_TYPES.
export const EVENT_GROUPS: { key: string; label: string; types: EventType[] }[] = [
  {
    key: "sku",
    label: "票档",
    types: [
      "sku_restock",
      "sku_onsale",
      "sku_added",
      "sku_soldout",
      "sku_price_change",
      "sku_threshold_change",
      "sku_low_stock",
      "sku_removed",
    ],
  },
  {
    key: "screen",
    label: "场次",
    types: ["screen_onsale", "screen_soldout", "screen_added", "screen_removed", "day_soldout"],
  },
  {
    key: "project",
    label: "项目",
    types: [
      "project_hot_on",
      "project_hot_off",
      "project_onsale",
      "project_presale",
      "project_soldout",
      "project_window_open",
      "project_price_band_change",
      "project_status_change",
      "wish_milestone",
    ],
  },
  {
    key: "ops",
    label: "运维 / 风控",
    types: ["risk_412", "risk_429", "node_online", "node_offline", "monitor_added", "monitor_removed"],
  },
];

const KEY = "fnmdp.eventFilter.v1";

function load(): EventFilter {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<EventFilter>;
      if (Array.isArray(parsed.hidden)) return { hidden: parsed.hidden };
    }
  } catch {
    /* fall through to defaults */
  }
  return { hidden: [...DEFAULT_HIDDEN] };
}

export const $eventFilter = atom<EventFilter>(load());
$eventFilter.subscribe((f) => {
  try {
    localStorage.setItem(KEY, JSON.stringify(f));
  } catch {
    /* private mode / quota — prefs just won't persist */
  }
});

export function setHidden(hidden: EventType[]): void {
  $eventFilter.set({ hidden });
}

export function showAll(): void {
  setHidden([]);
}

export function isVisible(t: EventType, f: EventFilter = $eventFilter.get()): boolean {
  return !f.hidden.includes(t);
}
