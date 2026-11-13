// Pure logic for the status-timeline grid ("格子状态图").
// Rebuilds a per-SKU status-over-time series from /api/history events + the
// current snapshot, then buckets it into fixed-count cells. Cells only show real
// observations; empty buckets stay no-data so the chart does not imply a state
// was observed continuously.
import type { RestEvent } from "../api/rest";
import { statusKind } from "./flag";

export type GridStatus =
  | "available" // 可售
  | "presale" // 即将开售 / 未开售
  | "soldout" // 已售罄 / 不可售
  | "ended" // 已结束 / 停售
  | "nodata"; // 无上报

export interface StatusPoint {
  time: number; // epoch ms
  status: GridStatus;
}

export interface GridCell {
  time: number; // bucket start, epoch ms
  status: GridStatus;
  hasData: boolean; // true = real observation in bucket; false = no observation
  filled: boolean; // true = no observation but status carried forward (inferred-held)
}

export const gridStatusLabel: Record<GridStatus, string> = {
  available: "可售",
  presale: "即将开售",
  soldout: "已售罄",
  ended: "已结束",
  nodata: "无上报",
};

// Color CSS vars (defined in themes/sky.css) so the grid follows the theme but
// keeps domain hues recognizable across light/dark.
export const gridStatusVar: Record<GridStatus, string> = {
  available: "var(--grid-available)",
  presale: "var(--grid-coming)",
  soldout: "var(--grid-soldout)",
  ended: "var(--grid-suspended)",
  nodata: "var(--grid-nodata)",
};

// Lower wins when several statuses fall in one bucket: surface the most
// actionable state (可售 > 已罄 > 即将开售 > 结束 > 无数据).
const PRIORITY: Record<GridStatus, number> = {
  available: 1,
  soldout: 2,
  presale: 3,
  ended: 4,
  nodata: 5,
};

export function bestGridStatus(statuses: GridStatus[]): GridStatus {
  let best: GridStatus = "nodata";
  for (const status of statuses) {
    if (PRIORITY[status] < PRIORITY[best]) best = status;
  }
  return best;
}

export function gridStatusOf(flag: number, clickable: boolean, flagText: string): GridStatus {
  switch (statusKind(flag, clickable, flagText)) {
    case "buyable":
    case "tight":
      return "available";
    case "presale":
      return "presale";
    case "soldout":
    case "unavailable":
      return "soldout";
    case "ended":
      return "ended";
    default:
      return "nodata";
  }
}

// pointsFromEvents derives the post-change status at each event time (server_ts in
// seconds → ms), sorted ascending. Events that don't change status (e.g. a price
// change) still record the status holding at that moment — harmless and correct.
export function pointsFromEvents(events: RestEvent[]): StatusPoint[] {
  const pts: StatusPoint[] = events.map((e) => ({
    time: e.server_ts * 1000,
    status: gridStatusOf(e.new_flag, e.new_clickable, e.flag_text),
  }));
  pts.sort((a, b) => a.time - b.time);
  return pts;
}

// mergePoint inserts an observation keeping the series sorted and dropping an
// exact (time,status) duplicate. Used to fold in the current snapshot + live WS
// state updates. Returns a new array (caller swaps it into state).
export function mergePoint(points: StatusPoint[], p: StatusPoint): StatusPoint[] {
  if (points.some((q) => q.time === p.time && q.status === p.status)) return points;
  const next = [...points, p];
  next.sort((a, b) => a.time - b.time);
  return next;
}

export interface BuildOpts {
  now: number; // epoch ms (server clock)
  intervalMs: number; // ms per cell
  cellCount: number;
  points: StatusPoint[]; // sorted ascending
  // fillUntil (epoch ms): forward-fill empty buckets with the last known status up
  // to this liveness high-water mark (project last_poll_ts). Buckets past it stay
  // no-data so a genuine collection gap is still visible. Omit/0 disables fill.
  fillUntil?: number;
}

// buildGrid buckets the status series into cells aligned to the interval, ending
// at `now`. Each cell takes the highest-priority status of any observation inside
// it. Buckets without an observation inherit the last known status (forward-fill,
// marked filled=true) as long as they fall at/under fillUntil — the SKU state is a
// step function that holds until the next change; past fillUntil they are no-data.
export function buildGrid({ now, intervalMs, cellCount, points, fillUntil = 0 }: BuildOpts): GridCell[] {
  const alignedNow = Math.floor(now / intervalMs) * intervalMs;
  const windowMs = cellCount * intervalMs;
  const start = alignedNow - windowMs + intervalMs;
  const cells: GridCell[] = [];

  // Seed the carry with the last state observed BEFORE the window opens, so a SKU
  // that has been stable since before the visible range still forward-fills from
  // the first cell. points are sorted ascending.
  let carried: GridStatus | null = null;
  for (const p of points) {
    if (p.time >= start) break;
    carried = p.status;
  }

  for (let i = 0; i < cellCount; i++) {
    const cellStart = start + i * intervalMs;
    // Last cell extends to `now` so the freshest change still lands somewhere.
    const upper = i === cellCount - 1 ? Math.max(cellStart + intervalMs, now + 1) : cellStart + intervalMs;
    let best: StatusPoint | null = null; // most actionable status → what the cell shows
    let latest: StatusPoint | null = null; // chronologically last → what carries forward
    for (const p of points) {
      if (p.time < cellStart || p.time >= upper) continue;
      if (
        best === null ||
        PRIORITY[p.status] < PRIORITY[best.status] ||
        (PRIORITY[p.status] === PRIORITY[best.status] && p.time >= best.time)
      ) {
        best = p;
      }
      if (latest === null || p.time >= latest.time) latest = p;
    }
    if (best) {
      // Show the most actionable status in the bucket, but carry the state that
      // actually holds afterwards — the last observation by time, not the priority
      // winner (e.g. available→soldout in one bucket shows 可售 but carries 已售罄).
      carried = (latest ?? best).status;
      cells.push({ time: cellStart, status: best.status, hasData: true, filled: false });
    } else if (carried !== null && cellStart <= fillUntil) {
      // Inferred-held: no observation, but a prior state holds and collection was
      // still live through this bucket.
      cells.push({ time: cellStart, status: carried, hasData: false, filled: true });
    } else {
      cells.push({ time: cellStart, status: "nodata", hasData: false, filled: false });
    }
  }
  return cells;
}

export const STATUS_GRID_CELL_COUNT = 100;

const MINUTE_MS = 60_000;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;
const intervalForWindow = (windowMs: number) => windowMs / STATUS_GRID_CELL_COUNT;

// Window presets (ms per cell). The
// labels assume STATUS_GRID_CELL_COUNT cells: 3m = 1.8s/cell, 10m = 6s/cell, etc.
export interface WindowPreset {
  label: string; // short, e.g. "10分钟"
  intervalMs: number;
}

export const WINDOW_PRESETS: WindowPreset[] = [
  { label: "1分钟", intervalMs: intervalForWindow(1 * MINUTE_MS) },
  { label: "3分钟", intervalMs: intervalForWindow(3 * MINUTE_MS) },
  { label: "10分钟", intervalMs: intervalForWindow(10 * MINUTE_MS) },
  { label: "30分钟", intervalMs: intervalForWindow(30 * MINUTE_MS) },
  { label: "2小时", intervalMs: intervalForWindow(2 * HOUR_MS) },
  { label: "6小时", intervalMs: intervalForWindow(6 * HOUR_MS) },
  { label: "24小时", intervalMs: intervalForWindow(24 * HOUR_MS) },
  { label: "3天", intervalMs: intervalForWindow(3 * DAY_MS) },
  { label: "7天", intervalMs: intervalForWindow(7 * DAY_MS) },
];

export const DEFAULT_INTERVAL_MS = intervalForWindow(10 * MINUTE_MS); // 10-minute window

// ── Navigable window math (observability-style pan/zoom timeline) ──────────────
// The grid is a fixed `cellCount` window whose right edge `endMs` is either "live"
// (server-now) or a frozen history anchor. Buckets are interval-wide and the window
// spans exactly cellCount buckets, so endMs lands on the last bucket's start.

export function trajectoryWindow(
  intervalMs: number,
  endMs: number,
  cellCount: number = STATUS_GRID_CELL_COUNT,
): { sinceMs: number; untilMs: number } {
  return { sinceMs: endMs - intervalMs * (cellCount - 1), untilMs: endMs };
}

// Half-window pan step (ms): ← 更早 / 更新 → move by half the visible span.
export function panStepMs(intervalMs: number, cellCount: number = STATUS_GRID_CELL_COUNT): number {
  return Math.floor((intervalMs * cellCount) / 2);
}

// centerAnchor returns the window-end anchor (epoch ms) that places `centerMs` in
// the middle of a cellCount window at the given interval — used to jump a grid so a
// specific event's change time sits dead center.
export function centerAnchor(
  intervalMs: number,
  centerMs: number,
  cellCount: number = STATUS_GRID_CELL_COUNT,
): number {
  return Math.round(centerMs + (intervalMs * (cellCount - 1)) / 2);
}

// panAnchor resolves a pan from the current window end. dir = -1 (older) / +1
// (newer). Returns the next history anchor (epoch ms), or null to snap back to
// live once a forward pan reaches/passes now.
export function panAnchor(
  currentEndMs: number,
  nowMs: number,
  intervalMs: number,
  dir: -1 | 1,
  cellCount: number = STATUS_GRID_CELL_COUNT,
): number | null {
  const next = currentEndMs + dir * panStepMs(intervalMs, cellCount);
  if (next >= nowMs) return null;
  return next;
}

// cellCountFor computes how many cells fit a container width (reference geometry:
// 10px cell + 2px gap), clamped to [15, 120].
export function cellCountFor(containerWidth: number, cellPx = 10, gapPx = 2): number {
  if (containerWidth <= 0) return 60;
  const n = Math.floor((containerWidth + gapPx) / (cellPx + gapPx));
  return Math.max(15, Math.min(120, n));
}
