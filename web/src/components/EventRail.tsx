import type { ReactNode } from "react";
import { useStore } from "@nanostores/react";
import { motion } from "motion/react";
import { Link } from "react-router-dom";
import { HoverCard, Timeline } from "@heroui-pro/react";
import { $projects, $skus, skuKey } from "../store";
import type { FeedEvent } from "../store/normalize";
import type { Fingerprint, Severity } from "../protocol/types";
import { eventLabel, flagLabel, statusKind, statusText } from "../lib/flag";
import { yuan } from "../lib/price";
import { formatClock, formatDateTime } from "../lib/time";
import { fieldChanges } from "../lib/fieldChanges";
import { GetV2Diff } from "./GetV2Diff";

// Severity → Timeline marker status. Mirrors SeverityChip's color mapping
// (critical=danger, notable=warning) but ops drops to muted since the timeline
// rail already carries the accent and ops events are low-signal.
const SEVERITY_STATUS: Record<Severity, "danger" | "warning" | "default" | "muted"> = {
  critical: "danger",
  notable: "warning",
  info: "default",
  ops: "muted",
};

// Compact severity tag for the dense Events grid: a colored dot + short label.
// A full soft Chip per row flooded the column with tan; the dot carries the
// color and the label stays muted so the list reads calm.
const SEVERITY_DOT: Record<Severity, string> = {
  critical: "bg-danger",
  notable: "bg-warning",
  info: "bg-muted",
  ops: "bg-accent",
};
const SEVERITY_SHORT: Record<Severity, string> = {
  critical: "紧急",
  notable: "重要",
  info: "信息",
  ops: "运维",
};

function CrosshairIcon() {
  return (
    <svg
      aria-hidden
      width="13"
      height="13"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
    >
      <circle cx="12" cy="12" r="7" />
      <line x1="12" y1="1.5" x2="12" y2="4.5" />
      <line x1="12" y1="19.5" x2="12" y2="22.5" />
      <line x1="1.5" y1="12" x2="4.5" y2="12" />
      <line x1="19.5" y1="12" x2="22.5" y2="12" />
    </svg>
  );
}

function SeverityTag({ severity }: { severity: Severity }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-muted">
      <span className={`h-2 w-2 shrink-0 rounded-full ${SEVERITY_DOT[severity]}`} />
      {SEVERITY_SHORT[severity]}
    </span>
  );
}

function projectName(e: FeedEvent, projects: Record<string, { name?: string }>): string {
  if (e.project_id <= 0) return "系统事件";
  const p = projects[String(e.project_id)];
  return p?.name?.trim() ? p.name : `项目 #${e.project_id}`;
}

function detailString(e: FeedEvent, key: string): string {
  const v = e.details?.[key];
  return typeof v === "string" && v.trim() ? v.trim() : "";
}

function detailNumber(e: FeedEvent, key: string): number | null {
  const v = e.details?.[key];
  return typeof v === "number" && Number.isFinite(v) ? v : null;
}

function statusName(fp: Fingerprint | null, flagText: string): string {
  if (!fp) return "无状态";
  const kind = statusKind(fp.flag, fp.clickable, flagText);
  return kind === "unknown" ? flagLabel(fp.flag) : statusText[kind];
}

function statusMove(e: FeedEvent): string {
  if (e.old && e.new) {
    const oldStatus = statusName(e.old, e.flag_text);
    const newStatus = statusName(e.new, e.flag_text);
    if (oldStatus !== newStatus) return `${oldStatus} → ${newStatus}`;
    if (e.old.clickable !== e.new.clickable) {
      return `${e.old.clickable ? "可点" : "不可点"} → ${e.new.clickable ? "可点" : "不可点"}`;
    }
    return newStatus;
  }
  if (e.new) return statusName(e.new, e.flag_text);
  if (e.old) return `移除前 ${statusName(e.old, e.flag_text)}`;
  return "";
}

function stockMove(e: FeedEvent): string {
  if (e.old && e.new && e.old.num !== e.new.num) return `余票 ${e.old.num} → ${e.new.num}`;
  if (e.new && e.new.num > 0) return `余票 ${e.new.num}`;
  if (e.new && e.new.num === 0 && ["sku_soldout", "sku_low_stock"].includes(e.event_type)) return "余票 0";
  return "";
}

function priceMove(e: FeedEvent): string {
  if (e.old && e.new && e.old.price_fen !== e.new.price_fen) {
    return `¥${yuan(e.old.price_fen)} → ¥${yuan(e.new.price_fen)}`;
  }
  if (e.price_fen > 0 && ["sku_added", "sku_onsale", "sku_restock"].includes(e.event_type)) {
    return `¥${yuan(e.price_fen)}`;
  }
  return "";
}

function compact(parts: Array<string | null | undefined>): string {
  return parts.filter((p): p is string => !!p && p.trim().length > 0).join(" · ");
}

// eventDetail renders a short, event-specific summary so the feed explains what
// changed instead of only naming the event class. The `extra` line is dropped
// when it merely repeats something already in the summary (e.g. flag_text "已售罄"
// after a "… → 已售罄" transition).
function eventDetail(e: FeedEvent): { summary: string; extra: string } {
  const { summary, extra } = eventDetailRaw(e);
  return { summary, extra: extra && !summary.includes(extra) ? extra : "" };
}

function eventDetailRaw(e: FeedEvent): { summary: string; extra: string } {
  const transition = statusMove(e);
  const stock = stockMove(e);
  const price = priceMove(e);
  switch (e.event_type) {
    case "node_online":
    case "node_offline": {
      const node = detailString(e, "node_name") || e.node_id;
      const region = detailString(e, "region");
      return {
        summary: node ? `节点 ${node}` : "",
        extra: region ? `区域 ${region}` : "",
      };
    }
    case "risk_412":
    case "risk_429": {
      const statusCode = detailNumber(e, "status_code");
      const requestID = detailString(e, "sec_request_id");
      return {
        summary: compact([statusCode ? `HTTP ${statusCode}` : "", e.node_id ? `节点 ${e.node_id}` : ""]),
        extra: requestID ? `x-sec-request-id ${requestID}` : "",
      };
    }
    case "sku_low_stock":
      return { summary: compact([stock || (e.new ? `余票 ${e.new.num}` : ""), transition]), extra: "低于限购阈值" };
    case "sku_restock":
      return { summary: compact([stock || "回流可买", transition, price]), extra: e.flag_text };
    case "sku_price_change":
      return { summary: compact([price, transition]), extra: e.flag_text };
    case "sku_added":
    case "sku_removed":
    case "sku_onsale":
    case "sku_soldout":
    case "sku_threshold_change":
      return { summary: compact([transition, stock, price]), extra: e.flag_text };
    case "screen_onsale":
    case "screen_soldout":
    case "screen_added":
    case "screen_removed":
      return { summary: transition, extra: e.flag_text };
    case "project_onsale":
    case "project_presale":
    case "project_soldout":
    case "project_status_change":
      return { summary: transition, extra: e.flag_text };
    case "project_hot_on":
      return { summary: "热门：否 → 是", extra: "" };
    case "project_hot_off":
      return { summary: "热门：是 → 否", extra: "" };
    default:
      return { summary: compact([transition, stock, price]), extra: e.flag_text };
  }
}

// Shared column template so the virtualized Events list reads like a DataGrid:
// 级别 | 事件(+detail) | 项目 | 时间. The header row in Events.tsx reuses it.
export const EVENT_ROW_GRID =
  "grid grid-cols-[72px_minmax(0,1.5fr)_minmax(0,1fr)_88px] items-center gap-2 px-3";

function scopeParts(e: FeedEvent, skuDesc: string): string[] {
  const parts: string[] = [];
  if (e.screen_id > 0) parts.push(`场次 ${e.screen_id}`);
  if (e.sku_id > 0) parts.push(skuDesc || `SKU ${e.sku_id}`);
  // Node-scoped events already name the node in the summary; only surface the
  // reporting node for project-scoped events so it isn't duplicated.
  if (e.node_id && e.project_id > 0) parts.push(`节点 ${e.node_id}`);
  return parts;
}

function EventScope({
  e,
  projects,
  className = "",
}: {
  e: FeedEvent;
  projects: Record<string, { name?: string }>;
  className?: string;
}) {
  const name = projectName(e, projects);
  // Events carry only the numeric sku_id (the WS event message has no desc), so
  // resolve the ticket name from the $skus snapshot map. Key-scoped subscription:
  // $skus churns on every state_update/last_event_ts write and a whole-map
  // subscription would re-render every visible feed row on any SKU change.
  const key = skuKey(e.project_id, e.screen_id, e.sku_id);
  const skus = useStore($skus, { keys: [key] });
  const skuDesc = e.sku_id > 0 ? (skus[key]?.desc?.trim() ?? "") : "";
  const scope = scopeParts(e, skuDesc).join(" · ");
  const content = (
    <>
      <span className="truncate">{name}</span>
      {scope && <span className="truncate text-muted/70">{scope}</span>}
    </>
  );
  if (e.project_id <= 0) {
    return <span className={`flex min-w-0 flex-col ${className}`}>{content}</span>;
  }
  return (
    <Link
      to={`/project/${e.project_id}`}
      className={`flex min-w-0 flex-col hover:text-link hover:underline ${className}`}
    >
      {content}
    </Link>
  );
}

// DiffHoverCard wraps an event's headline (label + summary) so hovering it pops a
// getV2-format field diff. When nothing changed (e.g. ops events with no old/new)
// it renders the headline plain — no empty popover, no hover affordance.
function DiffHoverCard({ e, children }: { e: FeedEvent; children: ReactNode }) {
  const changes = fieldChanges(e);
  if (changes.length === 0) return <>{children}</>;
  return (
    <HoverCard openDelay={120} closeDelay={120}>
      <HoverCard.Trigger>
        <span className="cursor-help underline decoration-dotted decoration-1 decoration-muted/30 underline-offset-[3px]">
          {children}
        </span>
      </HoverCard.Trigger>
      <HoverCard.Content className="max-w-sm">
        <HoverCard.Arrow />
        <GetV2Diff changes={changes} />
      </HoverCard.Content>
    </HoverCard>
  );
}

// EventItem renders one columnar feed row for the Events firehose (DataGrid-style
// framing). The Overview/ProjectDetail sidebar uses EventRail (Timeline) instead.
export function EventItem({ e }: { e: FeedEvent }) {
  const projects = useStore($projects);
  const d = eventDetail(e);
  return (
    <div
      className={`${EVENT_ROW_GRID} border-b border-separator/50 py-2 text-sm transition-colors hover:bg-surface-hover`}
    >
      <SeverityTag severity={e.severity} />
      <div className="min-w-0">
        <div className="truncate">
          <DiffHoverCard e={e}>
            <span className="font-medium text-foreground">{eventLabel[e.event_type] ?? e.event_type}</span>
            {d.summary && <span className="ml-2 text-xs text-foreground/70">{d.summary}</span>}
          </DiffHoverCard>
        </div>
        {d.extra && <div className="truncate text-xs text-muted/70">{d.extra}</div>}
      </div>
      <EventScope e={e} projects={projects} className="text-xs text-muted" />
      <time className="text-right text-[10px] tabular-nums text-muted/70" title={formatDateTime(e.server_ts)}>
        {formatClock(e.server_ts)}
      </time>
    </div>
  );
}

// EventRail is the sidebar feed (Overview + ProjectDetail). It renders the live
// event stream as a HeroUI Pro Timeline — the rail/marker carry severity color
// while each row keeps the event label, detail suffix, project link, and clock.
export function EventRail({
  events,
  emptyHint,
  onFocus,
}: {
  events: FeedEvent[];
  emptyHint?: string;
  // When provided, sku-scoped events show a "定位" button that jumps the SKU board.
  onFocus?: (e: FeedEvent) => void;
}) {
  const projects = useStore($projects);
  if (events.length === 0) {
    return (
      <div className="px-3 py-6 text-center text-xs text-muted/70">{emptyHint ?? "暂无事件"}</div>
    );
  }
  return (
    <Timeline density="compact" size="sm" className="px-1">
      {events.map((e) => {
        const d = eventDetail(e);
        return (
          <Timeline.Item key={`${e.id}:${e.seq}`} status={SEVERITY_STATUS[e.severity]}>
            <Timeline.Marker aria-hidden="true" />
            <Timeline.Content>
              <motion.div
                layout
                initial={{ opacity: 0, y: -6 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.2 }}
                className="flex items-start justify-between gap-2"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <DiffHoverCard e={e}>
                      <span className="text-xs font-medium text-foreground">
                        {eventLabel[e.event_type] ?? e.event_type}
                      </span>
                      {d.summary && <span className="ml-2 text-xs text-foreground/70">{d.summary}</span>}
                    </DiffHoverCard>
                  </div>
                  {d.extra && <div className="truncate text-xs text-muted/70">{d.extra}</div>}
                  <EventScope e={e} projects={projects} className="text-xs text-muted" />
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <time className="text-[10px] tabular-nums text-muted/70" title={formatDateTime(e.server_ts)}>
                    {formatClock(e.server_ts)}
                  </time>
                  {onFocus && e.sku_id > 0 && (
                    <button
                      type="button"
                      onClick={() => onFocus(e)}
                      className="text-muted/60 transition-colors hover:text-accent"
                      title="在票档状态中定位并居中此变化"
                      aria-label="定位到票档"
                    >
                      <CrosshairIcon />
                    </button>
                  )}
                </div>
              </motion.div>
            </Timeline.Content>
          </Timeline.Item>
        );
      })}
    </Timeline>
  );
}
