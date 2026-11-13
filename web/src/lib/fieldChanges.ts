// getV2-format field diff for one event. The wire event already carries the full
// old/new Fingerprint ({flag, clickable, price_fen, num}) — see protocol/types —
// so "which fields changed" is derived here on the frontend, no protocol change.
// Field names and raw values mirror the upstream getV2 JSON (sale_flag.number /
// clickable / num / price) so the diff reads like the source payload rather than
// a friendly summary. flag_text/desc/limit_num are deliberately out of scope:
// they're not carried as old/new on the wire (shared/state.StateFingerprint).
import type { FeedEvent } from "../store/normalize";
import type { Fingerprint } from "../protocol/types";
import { flagLabel } from "./flag";

export interface FieldChange {
  key: string; // raw getV2 field name, e.g. "sale_flag.number"
  oldText: string;
  newText: string;
}

const MISSING = "—";

// sale_flag.number shows the raw enum plus its derived label (派生自 number,
// 不依赖 flag_text), e.g. "4 (已售罄)".
function flagCell(fp: Fingerprint): string {
  return `${fp.flag} (${flagLabel(fp.flag)})`;
}

// fieldChanges lists the getV2 fields that differ between old and new. Only
// changed fields are emitted (true git-diff). When one side is null (an added or
// removed SKU) every relevant field is listed with the missing side as "—",
// mirroring git's whole-file add/delete. num/price are SKU-scoped: project and
// screen fingerprints don't carry meaningful inventory/price, so they're only
// compared when sku_id > 0.
export function fieldChanges(e: FeedEvent): FieldChange[] {
  const out: FieldChange[] = [];
  const row = (key: string, ov: string, nv: string) => out.push({ key, oldText: ov, newText: nv });

  // hotProject is not part of the wire Fingerprint. The event_type fully encodes
  // the toggle, so it can still be shown in getV2 form without diffing details.
  if (e.event_type === "project_hot_on") row("hotProject", "false", "true");
  else if (e.event_type === "project_hot_off") row("hotProject", "true", "false");

  // Fingerprint-derived fields. Skip entirely when neither side is present, else
  // a both-null event would emit a bogus "— → —" row.
  const { old: o, new: n } = e;
  if (o || n) {
    // sale_flag.number — authoritative sale state, compared for every event
    if (!o || !n || o.flag !== n.flag) {
      row("sale_flag.number", o ? flagCell(o) : MISSING, n ? flagCell(n) : MISSING);
    }
    // clickable — buyability, compared for every event
    if (!o || !n || o.clickable !== n.clickable) {
      row("clickable", o ? String(o.clickable) : MISSING, n ? String(n.clickable) : MISSING);
    }
    if (e.sku_id > 0) {
      // num — core inventory signal (min(stock, limit))
      if (!o || !n || o.num !== n.num) {
        row("num", o ? String(o.num) : MISSING, n ? String(n.num) : MISSING);
      }
      // price — getV2 `price` is in fen; show the raw value, not the ¥ form
      if (!o || !n || o.price_fen !== n.price_fen) {
        row("price", o ? String(o.price_fen) : MISSING, n ? String(n.price_fen) : MISSING);
      }
    }
  }
  return out;
}
