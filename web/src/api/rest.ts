// REST DTOs mirror master/internal/model (snake_case json tags). These are the
// public read-only endpoints (no auth, no CORS — dev goes through the Vite proxy).
// The WS message types live in ../protocol/types.ts; these are the REST shapes the
// snapshot/detail/events/nodes endpoints return, which protocol/types.ts leaves
// as `unknown[]` on the Snapshot message.

export interface RestProject {
  project_id: number;
  name: string;
  flag: number;
  flag_text: string;
  hot: boolean;
  wish_count: number;
  price_low: number;
  price_high: number;
  salable_low: number;
  salable_high: number;
  status: number;
  is_sale: number;
  sale_start: number;
  sale_end: number;
  count_down: number;
  server_ts: number;
  last_event_ts: number;
  last_poll_ts: number;
  priority: number;
  target_interval_ms: number;
  replica_factor: number;
  enabled: boolean;
  updated_at: string;
}

export interface RestScreen {
  project_id: number;
  screen_id: number;
  name: string;
  start_time: number;
  flag: number;
  flag_text: string;
  clickable: boolean;
  sale_start: number;
  sale_end: number;
  server_ts: number;
  last_event_ts: number;
  updated_at: string;
}

export interface RestSku {
  project_id: number;
  screen_id: number;
  sku_id: number;
  desc: string;
  flag: number;
  flag_text: string;
  clickable: boolean;
  price_fen: number;
  num: number;
  limit_num: number;
  server_ts: number;
  last_event_ts: number;
  updated_at: string;
}

export interface RestNode {
  node_id: string;
  node_name: string;
  region: string;
  online: boolean;
  in_flight: number;
  soft_limited: number;
  hard_blocked: number;
  last_seen: number;
  skew_ms: number;
  budget_rps?: number;
  assignments: number[] | null;
}

// REST events are FLAT (old_flag/new_flag/old_num/new_num/...), unlike the nested
// WS event (old/new Fingerprint). store/normalize.ts unifies both into FeedEvent.
export interface RestEvent {
  id?: number;
  project_id: number;
  screen_id: number;
  sku_id: number;
  event_type: string;
  severity: string;
  old_flag: number;
  new_flag: number;
  old_clickable: boolean;
  new_clickable: boolean;
  old_price_fen: number;
  price_fen: number;
  old_num: number;
  new_num: number;
  flag_text: string;
  server_ts: number;
  node_id: string;
  details?: Record<string, unknown>;
  created_at: string;
}

export interface RestSnapshot {
  projects: RestProject[] | null;
  screens: RestScreen[] | null;
  skus: RestSku[] | null;
  nodes: RestNode[] | null;
}

export interface TrajectoryState {
  flag: number;
  clickable: boolean;
  flag_text: string;
}

export interface TrajectoryPoint {
  t: number; // epoch ms, bucket start
  flag: number;
  clickable: boolean;
  flag_text: string;
  observed: boolean;
  samples?: TrajectoryState[];
}

export interface TrajectorySeries {
  project_id: number;
  screen_id: number;
  sku_id: number;
  points: TrajectoryPoint[];
}

export interface RestProjectDetail {
  project: RestProject;
  screens: RestScreen[] | null;
  skus: RestSku[] | null;
}

export interface EventQuery {
  project_id?: number;
  event_type?: string;
  severity?: string;
  limit?: number;
}

export interface EventPageQuery extends EventQuery {
  cursor?: string;
}

export interface RestEventPage {
  events: RestEvent[];
  has_more: boolean;
  next_cursor?: string;
}

export interface HistoryQuery {
  project_id: number;
  screen_id?: number;
  sku_id?: number;
  since?: number; // unix seconds (inclusive lower bound on created_at)
  until?: number; // unix seconds (inclusive upper bound)
  limit?: number;
}

export interface TrajectoryQuery {
  project_id: number;
  screen_id: number;
  sku_id: number;
  since_ms: number;
  until_ms: number;
  interval_ms: number;
}

export interface TrajectoriesQuery {
  project_id: number;
  since_ms: number;
  until_ms: number;
  interval_ms: number;
}

async function getJSON<T>(path: string): Promise<T> {
  const r = await fetch(path, { headers: { Accept: "application/json" } });
  if (!r.ok) throw new Error(`${path} → ${r.status}`);
  return (await r.json()) as T;
}

export const fetchSnapshot = (): Promise<RestSnapshot> => getJSON("/api/snapshot");

export const fetchProject = (id: number): Promise<RestProjectDetail> =>
  getJSON(`/api/projects/${id}`);

export const fetchNodes = (): Promise<RestNode[]> => getJSON("/api/nodes");

export function fetchEvents(q: EventQuery = {}): Promise<RestEvent[]> {
  const p = new URLSearchParams();
  if (q.project_id) p.set("project_id", String(q.project_id));
  if (q.event_type) p.set("event_type", q.event_type);
  if (q.severity) p.set("severity", q.severity);
  p.set("limit", String(q.limit ?? 100));
  return getJSON(`/api/events?${p.toString()}`);
}

export function fetchEventPage(q: EventPageQuery = {}): Promise<RestEventPage> {
  const p = new URLSearchParams();
  if (q.project_id) p.set("project_id", String(q.project_id));
  if (q.event_type) p.set("event_type", q.event_type);
  if (q.severity) p.set("severity", q.severity);
  if (q.cursor) p.set("cursor", q.cursor);
  p.set("limit", String(q.limit ?? 100));
  return getJSON(`/api/events/page?${p.toString()}`);
}

// fetchHistory backs the status-timeline grid: per-SKU state-change events over a
// time window. Returns the same flat RestEvent rows as /api/events.
export function fetchHistory(q: HistoryQuery): Promise<RestEvent[]> {
  const p = new URLSearchParams();
  p.set("project_id", String(q.project_id));
  if (q.screen_id) p.set("screen_id", String(q.screen_id));
  if (q.sku_id) p.set("sku_id", String(q.sku_id));
  if (q.since) p.set("since", String(q.since));
  if (q.until) p.set("until", String(q.until));
  p.set("limit", String(q.limit ?? 200));
  return getJSON(`/api/history?${p.toString()}`);
}

export function fetchTrajectory(q: TrajectoryQuery): Promise<TrajectoryPoint[]> {
  const p = new URLSearchParams();
  p.set("project_id", String(q.project_id));
  p.set("screen_id", String(q.screen_id));
  p.set("sku_id", String(q.sku_id));
  p.set("since_ms", String(q.since_ms));
  p.set("until_ms", String(q.until_ms));
  p.set("interval_ms", String(q.interval_ms));
  return getJSON(`/api/trajectory?${p.toString()}`);
}

export function fetchTrajectories(q: TrajectoriesQuery): Promise<TrajectorySeries[]> {
  const p = new URLSearchParams();
  p.set("project_id", String(q.project_id));
  p.set("since_ms", String(q.since_ms));
  p.set("until_ms", String(q.until_ms));
  p.set("interval_ms", String(q.interval_ms));
  return getJSON(`/api/trajectories?${p.toString()}`);
}
