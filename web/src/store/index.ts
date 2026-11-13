// Central reactive state (nanostores). First paint comes from REST /api/snapshot
// (the only full source — the WS snapshot omits screens/skus); WS increments then
// keep it live, and a 30s REST reconcile refreshes project-level aggregates the
// WS never pushes. Maps are keyed for O(1) updates.
import { atom, map } from "nanostores";
import type { RestNode, RestProject, RestScreen, RestSku, RestSnapshot, RestEvent } from "../api/rest";
import type { NodeStatus, StateUpdate } from "../protocol/types";
import { eventKey, fromRestEvent, fromWsEvent, type FeedEvent } from "./normalize";
import type { Event as WsEvent } from "../protocol/types";
import { syncServerClock } from "../lib/time";

export const FEED_CAP = 500;

export type ConnState = "connecting" | "online" | "reconnecting" | "offline";

export const $projects = map<Record<string, RestProject>>({});
export const $screens = map<Record<string, RestScreen>>({});
export const $skus = map<Record<string, RestSku>>({});
export const $nodes = map<Record<string, RestNode>>({});
export const $feed = atom<FeedEvent[]>([]);
export const $conn = atom<ConnState>("connecting");

export const skuKey = (pid: number, screen: number, sku: number): string => `${pid}:${screen}:${sku}`;
export const screenKey = (pid: number, screen: number): string => `${pid}:${screen}`;

// applySnapshot replaces the authoritative maps wholesale (used on first paint and
// on each reconcile). Feed/conn are untouched.
export function applySnapshot(snap: RestSnapshot): void {
  const projects: Record<string, RestProject> = {};
  for (const p of snap.projects ?? []) projects[String(p.project_id)] = p;
  const screens: Record<string, RestScreen> = {};
  for (const s of snap.screens ?? []) screens[screenKey(s.project_id, s.screen_id)] = s;
  const skus: Record<string, RestSku> = {};
  for (const s of snap.skus ?? []) skus[skuKey(s.project_id, s.screen_id, s.sku_id)] = s;
  const nodes: Record<string, RestNode> = {};
  for (const n of snap.nodes ?? []) nodes[n.node_id] = n;

  $projects.set(projects);
  $screens.set(screens);
  $skus.set(skus);
  $nodes.set(nodes);
  // NB: do NOT sync the server clock from snapshot entity server_ts — those are
  // last-update times (often minutes stale), not the current server clock. The
  // clock is set from the WS snapshot message + live state_update/event times.
}

// applyStateUpdate merges one SKU's authoritative state (WS state_update). Desc and
// screen/project context are preserved from the existing row when present.
export function applyStateUpdate(su: StateUpdate): void {
  const key = skuKey(su.project_id, su.screen_id, su.sku_id);
  const prev = $skus.get()[key];
  $skus.setKey(key, {
    project_id: su.project_id,
    screen_id: su.screen_id,
    sku_id: su.sku_id,
    desc: prev?.desc ?? "",
    flag: su.flag,
    flag_text: su.flag_text,
    clickable: su.clickable,
    price_fen: su.price_fen,
    num: su.num,
    limit_num: su.limit_num,
    server_ts: su.server_ts,
    last_event_ts: prev?.last_event_ts ?? 0,
    updated_at: prev?.updated_at ?? "",
  });
  syncServerClock(su.server_ts);
}

export function applyNodeStatus(ns: NodeStatus): void {
  const prev = $nodes.get()[ns.node_id];
  $nodes.setKey(ns.node_id, {
    node_id: ns.node_id,
    node_name: ns.node_name,
    region: ns.region,
    online: ns.online,
    in_flight: ns.in_flight,
    soft_limited: ns.soft_limited,
    hard_blocked: ns.hard_blocked,
    last_seen: ns.last_seen,
    skew_ms: ns.skew_ms,
    budget_rps: prev?.budget_rps,
    assignments: prev?.assignments ?? null,
  });
}

// ── feed ────────────────────────────────────────────────────────────────────

const newEventListeners = new Set<(e: FeedEvent) => void>();

// onNewEvent registers a callback fired for every freshly-arriving WS event (used
// by notifications). Returns an unsubscribe.
export function onNewEvent(cb: (e: FeedEvent) => void): () => void {
  newEventListeners.add(cb);
  return () => newEventListeners.delete(cb);
}

function prepend(ev: FeedEvent): boolean {
  const cur = $feed.get();
  if (cur.some((e) => e.id === ev.id)) return false; // dedup REST backfill vs WS
  const next = [ev, ...cur];
  if (next.length > FEED_CAP) next.length = FEED_CAP; // drop oldest, cap memory
  $feed.set(next);
  return true;
}

// pushWsEvent prepends a live WS event and notifies listeners (for notifications).
// It deliberately does NOT drive the global server clock: ops events carry the
// master's wall-clock and rate events the node's clock, both of which lead the
// poll-`server_ts` coverage timeline. Feeding them into the monotonic-forward
// clock (serverNow) ratcheted "现在" permanently into the future and, when
// polling stalled (429/412/disconnect), kept pushing it. The clock is fed only
// from poll-derived sources — `snapshot` (ws.ts) and `state_update` (applyStateUpdate).
export function pushWsEvent(ev: WsEvent): void {
  const fe = fromWsEvent(ev);
  applyLastEventTs(fe);
  if (prepend(fe)) {
    for (const cb of newEventListeners) cb(fe);
  }
}

function newerTs(next: number, prev?: number): number {
  return Math.max(prev ?? 0, next);
}

function applyLastEventTs(ev: FeedEvent): void {
  if (ev.severity === "ops" || ev.project_id <= 0 || ev.server_ts <= 0) return;
  const project = $projects.get()[String(ev.project_id)];
  if (project) {
    $projects.setKey(String(ev.project_id), {
      ...project,
      last_event_ts: newerTs(ev.server_ts, project.last_event_ts),
    });
  }
  if (ev.screen_id > 0) {
    const key = screenKey(ev.project_id, ev.screen_id);
    const screen = $screens.get()[key];
    if (screen) {
      $screens.setKey(key, {
        ...screen,
        last_event_ts: newerTs(ev.server_ts, screen.last_event_ts),
      });
    }
  }
  if (ev.sku_id > 0) {
    const key = skuKey(ev.project_id, ev.screen_id, ev.sku_id);
    const sku = $skus.get()[key];
    if (sku) {
      $skus.setKey(key, {
        ...sku,
        last_event_ts: newerTs(ev.server_ts, sku.last_event_ts),
      });
    }
  }
}

// backfillEvents seeds the feed from REST history (newest first), deduped. Does
// NOT fire notification listeners — only live WS events should notify.
export function backfillEvents(rows: RestEvent[]): void {
  const cur = $feed.get();
  const seen = new Set(cur.map((e) => e.id));
  const add: FeedEvent[] = [];
  for (const row of rows) {
    const key = eventKey(row);
    if (seen.has(key)) continue;
    seen.add(key);
    add.push(fromRestEvent(row));
  }
  const merged = [...cur, ...add].sort((a, b) =>
    b.server_ts !== a.server_ts ? b.server_ts - a.server_ts : b.seq - a.seq,
  );
  if (merged.length > FEED_CAP) merged.length = FEED_CAP;
  $feed.set(merged);
}

// ── selectors ─────────────────────────────────────────────────────────────────

export function projectScreens(pid: number, screens: Record<string, RestScreen>): RestScreen[] {
  return Object.values(screens)
    .filter((s) => s.project_id === pid)
    .sort((a, b) => a.start_time - b.start_time || a.screen_id - b.screen_id);
}

export function projectSkus(pid: number, skus: Record<string, RestSku>): RestSku[] {
  return Object.values(skus).filter((s) => s.project_id === pid);
}

export function screenSkus(pid: number, screen: number, skus: Record<string, RestSku>): RestSku[] {
  return Object.values(skus)
    .filter((s) => s.project_id === pid && s.screen_id === screen)
    .sort((a, b) => a.sku_id - b.sku_id);
}
