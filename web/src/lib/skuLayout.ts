// Per-project SKU board layout: the single-column block order plus user-defined
// aggregation groups. Persisted to localStorage per project. The stored shape only
// references skuKeys/group-ids; `resolveLayout` reconciles it against the SKUs that
// actually exist right now — new SKUs append as ungrouped blocks, vanished SKUs and
// emptied groups drop out — so the UI never renders stale entries.
import { atom, map } from "nanostores";
import type { RestSku, RestScreen } from "../api/rest";
import { $skus, projectSkus, skuKey } from "../store";
import { centerAnchor, DEFAULT_INTERVAL_MS } from "./statusgrid";
import { setGlobalWin, setGroupWin } from "./statusWindow";

export interface Group {
  id: string;
  name: string;
  members: string[]; // ordered skuKeys
}

export type Block = { kind: "sku"; key: string } | { kind: "group"; id: string };

export interface Layout {
  order: Block[];
  groups: Group[];
}

// Resolved (render-ready) blocks carry the live SKU objects.
export type ResolvedBlock =
  | { kind: "sku"; key: string; sku: RestSku }
  | { kind: "group"; id: string; name: string; members: { key: string; sku: RestSku }[] };

const keyOf = (s: RestSku) => skuKey(s.project_id, s.screen_id, s.sku_id);

// Pure: reconcile a stored layout (or none) against the current SKU set. Exported
// for unit testing.
export function resolveLayout(layout: Layout | undefined, skus: RestSku[]): ResolvedBlock[] {
  const byKey = new Map(skus.map((s) => [keyOf(s), s]));
  const used = new Set<string>();
  const groups = new Map((layout?.groups ?? []).map((g) => [g.id, g]));
  const blocks: ResolvedBlock[] = [];

  for (const b of layout?.order ?? []) {
    if (b.kind === "sku") {
      const sku = byKey.get(b.key);
      if (sku && !used.has(b.key)) {
        used.add(b.key);
        blocks.push({ kind: "sku", key: b.key, sku });
      }
    } else {
      const g = groups.get(b.id);
      if (!g) continue;
      const members = g.members
        .filter((k) => byKey.has(k) && !used.has(k))
        .map((k) => {
          used.add(k);
          return { key: k, sku: byKey.get(k)! };
        });
      if (members.length > 0) blocks.push({ kind: "group", id: g.id, name: g.name, members });
    }
  }

  // Append SKUs not placed by the stored order, in screen/sku order.
  const rest = skus
    .filter((s) => !used.has(keyOf(s)))
    .sort((a, b) => a.screen_id - b.screen_id || a.sku_id - b.sku_id);
  for (const s of rest) blocks.push({ kind: "sku", key: keyOf(s), sku: s });

  return blocks;
}

// ── persistent store ────────────────────────────────────────────────────────

export const $layouts = map<Record<string, Layout>>({});
const loaded = new Set<string>();
const keyFor = (pid: number) => `fnmdp.skuLayout.${pid}.v1`;

export function ensureLoaded(pid: number): void {
  const p = String(pid);
  if (loaded.has(p)) return;
  loaded.add(p);
  try {
    const raw = localStorage.getItem(keyFor(pid));
    if (raw) $layouts.setKey(p, JSON.parse(raw) as Layout);
  } catch {
    /* ignore */
  }
}

// First-visit default: a project with no saved layout opens aggregated by screen
// (not a flat SKU list). Reads the LIVE store after ensureLoaded so a layout just
// hydrated from localStorage is never overwritten; a deliberate "全部解散" persists
// an empty layout, which counts as customized and is left untouched.
export function ensureDefaultLayout(pid: number, screens: RestScreen[], skus: RestSku[]): void {
  ensureLoaded(pid);
  if ($layouts.get()[String(pid)]) return; // hydrated or already customized
  if (skus.length === 0 || screens.length === 0) return; // wait for data
  autoGroupByScreen(pid, screens, skus);
}

function save(pid: number, layout: Layout): void {
  $layouts.setKey(String(pid), layout);
  try {
    localStorage.setItem(keyFor(pid), JSON.stringify(layout));
  } catch {
    /* ignore */
  }
}

let seq = 0;
function newId(): string {
  seq += 1;
  return `g${Date.now().toString(36)}${seq}`;
}

function toLayout(blocks: ResolvedBlock[]): Layout {
  const order: Block[] = [];
  const groups: Group[] = [];
  for (const b of blocks) {
    if (b.kind === "sku") {
      order.push({ kind: "sku", key: b.key });
    } else {
      order.push({ kind: "group", id: b.id });
      groups.push({ id: b.id, name: b.name, members: b.members.map((m) => m.key) });
    }
  }
  return { order, groups };
}

// Apply a transform over the CURRENT resolved blocks, then persist. Resolving first
// keeps mutations consistent even before the layout has ever been materialised.
function mutate(pid: number, fn: (blocks: ResolvedBlock[]) => ResolvedBlock[]): void {
  const skus = projectSkus(pid, $skus.get());
  save(pid, toLayout(fn(resolveLayout($layouts.get()[String(pid)], skus))));
}

function skuIndex(blocks: ResolvedBlock[]): Map<string, RestSku> {
  const m = new Map<string, RestSku>();
  for (const b of blocks) {
    if (b.kind === "sku") m.set(b.key, b.sku);
    else b.members.forEach((mm) => m.set(mm.key, mm.sku));
  }
  return m;
}

// Remove the given keys from every block (top-level sku blocks + group members),
// dropping any group emptied as a result.
function without(blocks: ResolvedBlock[], keys: Set<string>): ResolvedBlock[] {
  return blocks
    .map((b) =>
      b.kind === "group" ? { ...b, members: b.members.filter((m) => !keys.has(m.key)) } : b,
    )
    .filter((b) => (b.kind === "sku" ? !keys.has(b.key) : b.members.length > 0));
}

export function setTopOrderByIds(pid: number, ids: string[]): void {
  mutate(pid, (blocks) => {
    const byId = new Map(blocks.map((b) => [b.kind === "sku" ? `sku:${b.key}` : `grp:${b.id}`, b]));
    const next = ids.map((id) => byId.get(id)).filter((b): b is ResolvedBlock => b != null);
    // Keep any block the ids list somehow missed (defensive).
    for (const b of blocks) if (!next.includes(b)) next.push(b);
    return next;
  });
}

export function setGroupMembersOrder(pid: number, groupId: string, keys: string[]): void {
  mutate(pid, (blocks) =>
    blocks.map((b) => {
      if (b.kind !== "group" || b.id !== groupId) return b;
      const byKey = new Map(b.members.map((m) => [m.key, m]));
      const members = keys.map((k) => byKey.get(k)).filter((m): m is { key: string; sku: RestSku } => m != null);
      return { ...b, members };
    }),
  );
}

export function createGroup(pid: number, memberKeys: string[], name: string): void {
  mutate(pid, (blocks) => {
    const idx = skuIndex(blocks);
    const members = memberKeys
      .filter((k) => idx.has(k))
      .map((k) => ({ key: k, sku: idx.get(k)! }));
    if (members.length === 0) return blocks;
    const keys = new Set(memberKeys);
    const at = blocks.findIndex((b) =>
      b.kind === "sku" ? keys.has(b.key) : b.members.some((m) => keys.has(m.key)),
    );
    const cleaned = without(blocks, keys);
    const group: ResolvedBlock = { kind: "group", id: newId(), name, members };
    const insertAt = at < 0 ? cleaned.length : Math.min(at, cleaned.length);
    cleaned.splice(insertAt, 0, group);
    return cleaned;
  });
}

export function addToGroup(pid: number, groupId: string, key: string): void {
  mutate(pid, (blocks) => {
    const idx = skuIndex(blocks);
    const sku = idx.get(key);
    if (!sku) return blocks;
    const cleaned = without(blocks, new Set([key]));
    return cleaned.map((b) =>
      b.kind === "group" && b.id === groupId ? { ...b, members: [...b.members, { key, sku }] } : b,
    );
  });
}

export function removeFromGroup(pid: number, groupId: string, key: string): void {
  mutate(pid, (blocks) => {
    let moved: import("../api/rest").RestSku | undefined;
    const out: ResolvedBlock[] = [];
    for (const b of blocks) {
      if (b.kind === "group" && b.id === groupId) {
        const remaining = b.members.filter((m) => {
          if (m.key === key) moved = m.sku;
          return m.key !== key;
        });
        if (remaining.length > 0) out.push({ ...b, members: remaining }); // drop group if emptied
      } else {
        out.push(b);
      }
    }
    if (moved) out.push({ kind: "sku", key, sku: moved });
    return out;
  });
}

export function renameGroup(pid: number, groupId: string, name: string): void {
  mutate(pid, (blocks) =>
    blocks.map((b) => (b.kind === "group" && b.id === groupId ? { ...b, name } : b)),
  );
}

export function dissolveGroup(pid: number, groupId: string): void {
  mutate(pid, (blocks) => {
    const out: ResolvedBlock[] = [];
    for (const b of blocks) {
      if (b.kind === "group" && b.id === groupId) {
        for (const m of b.members) out.push({ kind: "sku", key: m.key, sku: m.sku });
      } else {
        out.push(b);
      }
    }
    return out;
  });
}

export function dissolveAll(pid: number): void {
  save(pid, { order: [], groups: [] });
}

// ── focus (jump from an event) ────────────────────────────────────────────────

// Key of the SKU card to scroll to + highlight after a jump (null = none).
export const $focusedSku = atom<string | null>(null);

// Which group (if any) currently contains a SKU, per the resolved layout.
export function groupIdOfSku(pid: number, key: string): string | undefined {
  const skus = projectSkus(pid, $skus.get());
  const blocks = resolveLayout($layouts.get()[String(pid)], skus);
  for (const b of blocks) {
    if (b.kind === "group" && b.members.some((m) => m.key === key)) return b.id;
  }
  return undefined;
}

// focusSku jumps the relevant grid (its group's window if grouped, else global) to a
// window centered on `eventMs`, and flags the card for scroll-into-view + highlight.
export function focusSku(
  pid: number,
  key: string,
  eventMs: number,
  intervalMs: number = DEFAULT_INTERVAL_MS,
): void {
  const anchorMs = centerAnchor(intervalMs, eventMs);
  const gid = groupIdOfSku(pid, key);
  if (gid) setGroupWin(gid, { intervalMs, anchorMs });
  else setGlobalWin({ intervalMs, anchorMs });
  $focusedSku.set(key);
}

export function clearFocus(): void {
  if ($focusedSku.get() !== null) $focusedSku.set(null);
}

// One-click: aggregate SKUs into one group per screen (screen order preserved).
export function autoGroupByScreen(pid: number, screens: RestScreen[], skus: RestSku[]): void {
  const order = new Map(screens.map((s, i) => [s.screen_id, i]));
  const nameOf = new Map(screens.map((s) => [s.screen_id, s.name?.trim() || `场次 ${s.screen_id}`]));
  const byScreen = new Map<number, RestSku[]>();
  for (const s of skus) {
    const list = byScreen.get(s.screen_id) ?? [];
    list.push(s);
    byScreen.set(s.screen_id, list);
  }
  const groups: Group[] = [];
  const blockOrder: Block[] = [];
  const screenIds = [...byScreen.keys()].sort((a, b) => (order.get(a) ?? a) - (order.get(b) ?? b));
  for (const sid of screenIds) {
    const list = (byScreen.get(sid) ?? []).sort((a, b) => a.sku_id - b.sku_id);
    const id = newId();
    groups.push({ id, name: nameOf.get(sid) ?? `场次 ${sid}`, members: list.map(keyOf) });
    blockOrder.push({ kind: "group", id });
  }
  save(pid, { order: blockOrder, groups });
}
