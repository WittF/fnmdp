// FeedEvent is the single internal event model. The two wire shapes differ — the
// WS `event` is nested (old/new Fingerprint), the REST `/api/events` row is flat
// (old_flag/new_flag/old_num/new_num/…) — so both are normalized here.
import type { Event as WsEvent, EventType, Fingerprint, Severity } from "../protocol/types";
import type { RestEvent, RestProject } from "../api/rest";
import { buyable } from "../lib/flag";

export interface FeedEvent {
  id: string;
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
  seq: number;
}

// A stable identity for de-duplicating the REST backfill against WS pushes.
export function eventKey(e: {
  event_type: string;
  project_id: number;
  screen_id: number;
  sku_id: number;
  server_ts: number;
  old?: Fingerprint | null;
  new?: Fingerprint | null;
  node_id?: string;
  details?: Record<string, unknown>;
}): string {
  const requestID = typeof e.details?.sec_request_id === "string" ? e.details.sec_request_id : "";
  return JSON.stringify([
    e.event_type,
    e.project_id,
    e.screen_id,
    e.sku_id,
    e.server_ts,
    fingerprintKey(e.old),
    fingerprintKey(e.new),
    e.node_id ?? "",
    requestID,
  ]);
}

function fingerprintKey(fp?: Fingerprint | null): string {
  if (!fp) return "";
  return `${fp.flag}/${fp.clickable ? 1 : 0}/${fp.price_fen}/${fp.num}`;
}

let seqCounter = 0;
export function nextSeq(): number {
  return ++seqCounter;
}

export function fromWsEvent(ev: WsEvent): FeedEvent {
  return {
    id: eventKey(ev),
    event_type: ev.event_type,
    severity: ev.severity,
    project_id: ev.project_id,
    screen_id: ev.screen_id,
    sku_id: ev.sku_id,
    old: ev.old,
    new: ev.new,
    price_fen: ev.price_fen,
    flag_text: ev.flag_text ?? "",
    node_id: ev.node_id ?? "",
    details: ev.details,
    server_ts: ev.server_ts,
    seq: nextSeq(),
  };
}

export function fromRestEvent(ev: RestEvent): FeedEvent {
  const old: Fingerprint | null =
    ev.old_flag === 0 && !ev.old_clickable && ev.old_num === 0
      ? null
      : { flag: ev.old_flag, clickable: ev.old_clickable, price_fen: ev.old_price_fen, num: ev.old_num };
  const nw: Fingerprint | null =
    ev.new_flag === 0 && !ev.new_clickable && ev.new_num === 0
      ? null
      : { flag: ev.new_flag, clickable: ev.new_clickable, price_fen: ev.price_fen, num: ev.new_num };
  return {
    id: eventKey({ ...ev, old, new: nw }),
    event_type: ev.event_type as EventType,
    severity: ev.severity as Severity,
    project_id: ev.project_id,
    screen_id: ev.screen_id,
    sku_id: ev.sku_id,
    old,
    new: nw,
    price_fen: ev.price_fen,
    flag_text: ev.flag_text,
    node_id: ev.node_id,
    details: ev.details,
    server_ts: ev.server_ts,
    seq: nextSeq(),
  };
}

// anyBuyable summarizes a project for the buyability-first sort: a project is
// "actionable" if any of its SKUs is buyable. Works off the SKU list belonging to
// the project.
export interface SkuLike {
  flag: number;
  clickable: boolean;
  flag_text: string;
}

export function anyBuyable(skus: SkuLike[]): boolean {
  return skus.some((s) => buyable(s.flag, s.clickable));
}

// projectSortRank: lower sorts first. Buyable → presale → other → soldout, then by
// descending heat so hot/popular projects float up within a tier.
export function projectSortRank(p: RestProject, skus: SkuLike[]): number {
  if (anyBuyable(skus)) return 0;
  if (p.flag === 1) return 1;
  if (p.flag === 4) return 3;
  return 2;
}
