// Shared status-grid time window. Lifts the per-SKU `intervalMs` (粒度) + pan/
// anchor (范围) out of each StatusGrid into one place so a single control drives
// every grid — and lets an aggregation group override the global window for just
// its members. The granularity is persisted as a preference; the pan anchor is
// session-only (reset to live on project switch).
import { atom, map } from "nanostores";
import { DEFAULT_INTERVAL_MS, panAnchor } from "./statusgrid";
import { serverNow } from "./time";

export interface Win {
  intervalMs: number;
  anchorMs: number | null; // null = live (window end tracks server-now)
}

const KEY = "fnmdp.statusInterval.v1";

function loadInterval(): number {
  try {
    const raw = localStorage.getItem(KEY);
    if (raw) {
      const n = Number(raw);
      if (Number.isFinite(n) && n > 0) return n;
    }
  } catch {
    /* fall through to default */
  }
  return DEFAULT_INTERVAL_MS;
}

// Global window for all ungrouped SKUs.
export const $global = atom<Win>({ intervalMs: loadInterval(), anchorMs: null });
$global.subscribe((w) => {
  try {
    localStorage.setItem(KEY, String(w.intervalMs));
  } catch {
    /* private mode / quota */
  }
});

// Per-group overrides (session-only). Cleared on project switch via resetAnchors.
export const $groupWin = map<Record<string, Win>>({});

// Resolve the effective window for a block: a group's override if present, else global.
export function pickWin(global: Win, groupWin: Record<string, Win>, groupId?: string): Win {
  if (groupId && groupWin[groupId]) return groupWin[groupId];
  return global;
}

export function setGlobalWin(patch: Partial<Win>): void {
  $global.set({ ...$global.get(), ...patch });
}

// Patch a group's window — creates the override (seeded from global) on first edit.
export function setGroupWin(id: string, patch: Partial<Win>): void {
  const cur = $groupWin.get()[id] ?? $global.get();
  $groupWin.setKey(id, { ...cur, ...patch });
}

export function clearGroupWin(id: string): void {
  const next = { ...$groupWin.get() };
  delete next[id];
  $groupWin.set(next);
}

export function hasGroupWin(id: string): boolean {
  return id in $groupWin.get();
}

// Pan a window by half its span; dir = -1 (older) / +1 (newer). Snaps to live.
export function panWin(win: Win, dir: -1 | 1): Win {
  const nowMs = serverNow() * 1000;
  return { ...win, anchorMs: panAnchor(win.anchorMs ?? nowMs, nowMs, win.intervalMs, dir) };
}

// Back to live on project switch: drop the global anchor + all group overrides
// (group ids are layout-scoped per project, so stale overrides must not leak).
export function resetAnchors(): void {
  setGlobalWin({ anchorMs: null });
  $groupWin.set({});
}
