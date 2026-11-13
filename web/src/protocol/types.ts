// Hand-written TS mirror of the message contract. Verified against
// protocol/schema.json (the single source of truth) by types.test.ts, the same
// way shared/protocol/consistency_test.go verifies the Go side. Keep field names
// and VERSION in lockstep with the schema.

export const VERSION = 8;

export type Severity = "critical" | "notable" | "info" | "ops";
export type RateKind = "soft_limited" | "hard_blocked";

export type EventType =
  // SKU level
  | "sku_restock"
  | "sku_onsale"
  | "sku_added"
  | "sku_soldout"
  | "sku_price_change"
  | "sku_threshold_change"
  | "sku_low_stock"
  | "sku_removed"
  // Screen level
  | "screen_onsale"
  | "screen_soldout"
  | "screen_added"
  | "screen_removed"
  | "day_soldout"
  // Project level
  | "project_hot_on"
  | "project_hot_off"
  | "project_onsale"
  | "project_presale"
  | "project_soldout"
  | "project_window_open"
  | "project_price_band_change"
  | "project_status_change"
  | "wish_milestone"
  // Ops
  | "risk_412"
  | "risk_429"
  | "node_online"
  | "node_offline"
  | "monitor_added"
  | "monitor_removed";

export interface Fingerprint {
  flag: number;
  clickable: boolean;
  price_fen: number;
  // Available count = min(real stock, per-order limit). 0 = sold out / not-yet
  // on sale (tell apart by flag). Below limit_num it is the exact remaining count.
  num: number;
}

export interface Assignment {
  project_id: number;
  phase: number;
  phase_mod: number;
  interval_ms: number;
}

export interface SkuDiff {
  screen_id: number;
  sku_id: number;
  desc: string;
  flag_text: string;
  // Per-order purchase cap (static_limit.num). Derive 充足 (num>=limit) vs
  // 告急 (0<num<limit) from num vs limit_num.
  limit_num: number;
  old: Fingerprint | null;
  new: Fingerprint | null;
}

export interface ProjectMeta {
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
}

export interface ScreenDiff {
  screen_id: number;
  name: string;
  start_time: number;
  flag: number;
  flag_text: string;
  clickable: boolean;
  sale_start: number;
  sale_end: number;
  removed: boolean;
}

// ── node → master ──────────────────────────────────────────────────────────

export interface Register {
  type: "register";
  version: number;
  node_id: string;
  node_name: string;
  region: string;
  capabilities: string[];
}

export interface Heartbeat {
  type: "heartbeat";
  version: number;
  node_id: string;
  ts: number;
  in_flight: number;
  soft_limited: number;
  hard_blocked: number;
}

export interface PollResult {
  type: "poll_result";
  version: number;
  node_id: string;
  project_id: number;
  source?: string;
  server_ts: number;
  fetched_at: number;
  diff: SkuDiff[];
  project: ProjectMeta | null;
  screens: ScreenDiff[];
}

export interface RateEvent {
  type: "rate_event";
  version: number;
  node_id: string;
  project_id: number;
  status_code: number;
  kind: RateKind;
  sec_request_id: string;
  ts: number;
}

// ── master → node ──────────────────────────────────────────────────────────

export interface Assign {
  type: "assign";
  version: number;
  node_id: string;
  assignments: Assignment[];
}

export interface AssignDelta {
  type: "assign_delta";
  version: number;
  node_id: string;
  added: Assignment[];
  removed: number[];
}

export interface Config {
  type: "config";
  version: number;
  min_interval_ms: number;
  soft_backoff_ms: number;
  hard_cooldown_ms: number;
  user_agent: string;
  keepalive_ms: number;
}

// ── master → frontend ──────────────────────────────────────────────────────

export interface Snapshot {
  type: "snapshot";
  version: number;
  server_ts: number;
  projects: unknown[];
  nodes: unknown[];
}

export interface StateUpdate {
  type: "state_update";
  version: number;
  project_id: number;
  screen_id: number;
  sku_id: number;
  flag: number;
  flag_text: string;
  clickable: boolean;
  price_fen: number;
  num: number;
  limit_num: number;
  server_ts: number;
}

export interface Event {
  type: "event";
  version: number;
  event_type: EventType;
  severity: Severity;
  project_id: number;
  screen_id: number;
  sku_id: number;
  old: Fingerprint | null;
  new: Fingerprint | null;
  price_fen: number;
  flag_text: string;
  node_id: string;
  details?: Record<string, unknown>;
  server_ts: number;
}

export interface NodeStatus {
  type: "node_status";
  version: number;
  node_id: string;
  node_name: string;
  region: string;
  online: boolean;
  in_flight: number;
  soft_limited: number;
  hard_blocked: number;
  last_seen: number;
  skew_ms: number;
}

// Discriminated union over the `type` field for dispatch on both ends.
export type Message =
  | Register
  | Heartbeat
  | PollResult
  | RateEvent
  | Assign
  | AssignDelta
  | Config
  | Snapshot
  | StateUpdate
  | Event
  | NodeStatus;

// MESSAGE_FIELDS is the declared field set per message type, asserted against
// the schema in types.test.ts. Keep in sync with the interfaces above.
export const MESSAGE_FIELDS: Record<string, string[]> = {
  register: ["type", "version", "node_id", "node_name", "region", "capabilities"],
  heartbeat: ["type", "version", "node_id", "ts", "in_flight", "soft_limited", "hard_blocked"],
  poll_result: ["type", "version", "node_id", "project_id", "source", "server_ts", "fetched_at", "diff", "project", "screens"],
  rate_event: ["type", "version", "node_id", "project_id", "status_code", "kind", "sec_request_id", "ts"],
  assign: ["type", "version", "node_id", "assignments"],
  assign_delta: ["type", "version", "node_id", "added", "removed"],
  config: ["type", "version", "min_interval_ms", "soft_backoff_ms", "hard_cooldown_ms", "user_agent", "keepalive_ms"],
  snapshot: ["type", "version", "server_ts", "projects", "nodes"],
  state_update: ["type", "version", "project_id", "screen_id", "sku_id", "flag", "flag_text", "clickable", "price_fen", "num", "limit_num", "server_ts"],
  event: ["type", "version", "event_type", "severity", "project_id", "screen_id", "sku_id", "old", "new", "price_fen", "flag_text", "node_id", "details", "server_ts"],
  node_status: ["type", "version", "node_id", "node_name", "region", "online", "in_flight", "soft_limited", "hard_blocked", "last_seen", "skew_ms"],
};

export const DEFINITION_FIELDS: Record<string, string[]> = {
  fingerprint: ["flag", "clickable", "price_fen", "num"],
  assignment: ["project_id", "phase", "phase_mod", "interval_ms"],
  sku_diff: ["screen_id", "sku_id", "desc", "flag_text", "limit_num", "old", "new"],
  project_meta: ["name", "flag", "flag_text", "hot", "wish_count", "price_low", "price_high", "salable_low", "salable_high", "status", "is_sale", "sale_start", "sale_end", "count_down", "server_ts"],
  screen_diff: ["screen_id", "name", "start_time", "flag", "flag_text", "clickable", "sale_start", "sale_end", "removed"],
};

export const SEVERITIES: Severity[] = ["critical", "notable", "info", "ops"];
export const RATE_KINDS: RateKind[] = ["soft_limited", "hard_blocked"];
export const EVENT_TYPES: EventType[] = [
  "sku_restock", "sku_onsale", "sku_added", "sku_soldout", "sku_price_change",
  "sku_threshold_change", "sku_low_stock", "sku_removed", "screen_onsale", "screen_soldout",
  "screen_added", "screen_removed", "day_soldout", "project_hot_on",
  "project_hot_off", "project_onsale", "project_presale", "project_soldout",
  "project_window_open", "project_price_band_change", "project_status_change",
  "wish_milestone", "risk_412", "risk_429", "node_online", "node_offline",
  "monitor_added", "monitor_removed",
];
