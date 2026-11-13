import NumberFlow from "@number-flow/react";
import { useStore } from "@nanostores/react";
import { Chip } from "@heroui/react";
import { KPI } from "@heroui-pro/react";
import type { Severity } from "../protocol/types";
import { type StatusKind, type Stock, statusText, stockLabel } from "../lib/flag";
import { formatAge, formatCountdown, formatDateTime, secondsUntil } from "../lib/time";
import { priceBand } from "../lib/price";
import { useTick } from "../lib/hooks";
import { $conn } from "../store";

type ChipColor = "default" | "accent" | "success" | "warning" | "danger";

// Map domain semantics → HeroUI Chip colors. Color is reserved for status
// (beszel ethos): buyable=green, 余票紧张=amber, soldout=red, the rest neutral.
const STATUS_COLOR: Record<StatusKind, ChipColor> = {
  buyable: "success",
  tight: "warning",
  presale: "default",
  soldout: "danger",
  unavailable: "default",
  ended: "default",
  unknown: "default",
};

const SEVERITY_COLOR: Record<Severity, ChipColor> = {
  critical: "danger",
  notable: "warning",
  info: "default",
  ops: "accent",
};

const SEVERITY_LABEL: Record<Severity, string> = {
  critical: "紧急",
  notable: "重要",
  info: "信息",
  ops: "运维",
};

const CONN = {
  connecting: { t: "连接中", c: "bg-warning", pulse: true },
  online: { t: "实时在线", c: "bg-success", pulse: false },
  reconnecting: { t: "重连中", c: "bg-warning", pulse: true },
  offline: { t: "离线", c: "bg-danger", pulse: false },
} as const;

export function ConnBadge() {
  const conn = useStore($conn);
  const m = CONN[conn];
  return (
    <span className="inline-flex items-center gap-1.5 text-xs text-muted">
      <span className={`h-2 w-2 rounded-full ${m.c} ${m.pulse ? "animate-pulse" : ""}`} />
      {m.t}
    </span>
  );
}

export function HotBadge({ hot }: { hot: boolean }) {
  if (!hot) return null;
  // Upstream marks high-demand projects as hot; surface it as a badge.
  return (
    <Chip color="warning" variant="soft" size="sm">
      🔥 热门
    </Chip>
  );
}

export function StatusBadge({ kind, text }: { kind: StatusKind; text?: string }) {
  return (
    <Chip color={STATUS_COLOR[kind]} variant="soft" size="sm">
      {text ?? statusText[kind]}
    </Chip>
  );
}

export function StockBadge({ stock }: { stock: Stock }) {
  if (stock.kind === "unknown") return null;
  const label = stockLabel(stock);
  if (!label) return null;
  const color =
    stock.kind === "ample" ? "text-success" : stock.kind === "low" ? "text-warning" : "text-danger";
  const weight = stock.kind === "low" ? "font-semibold" : "";
  return <span className={`text-xs tabular-nums ${color} ${weight}`}>{label}</span>;
}

export function PriceBand({ low, high }: { low: number; high: number }) {
  return <span className="text-xs tabular-nums text-foreground/80">{priceBand(low, high)}</span>;
}

export function WishCount({ value }: { value: number }) {
  if (value <= 0) return null;
  return (
    <span className="inline-flex items-center gap-1 text-xs text-muted">
      <span>想去</span>
      <NumberFlow value={value} className="font-semibold tabular-nums text-foreground" />
    </span>
  );
}

export function AgeStamp({ ts, label = "更新" }: { ts: number; label?: string }) {
  useTick(15000);
  if (ts <= 0) return null;
  return (
    <span className="text-xs tabular-nums text-muted" title={formatDateTime(ts)}>
      {label} {formatAge(ts)}
    </span>
  );
}

// Countdown renders time-to-sale off the server clock; the tick only re-renders.
export function Countdown({ saleStart }: { saleStart: number }) {
  useTick(1000);
  if (saleStart <= 0) return null;
  const left = secondsUntil(saleStart);
  if (left <= 0) return <span className="text-xs text-success">已开售</span>;
  return (
    <span className="inline-flex items-center gap-1 text-xs">
      <span className="text-muted">距开售</span>
      <span className="font-semibold tabular-nums text-foreground">{formatCountdown(left)}</span>
    </span>
  );
}

export function SeverityChip({ severity }: { severity: Severity }) {
  return (
    <Chip color={SEVERITY_COLOR[severity]} variant="soft" size="sm">
      {SEVERITY_LABEL[severity]}
    </Chip>
  );
}

// Stat is a compact KPI card used in the overview / health summaries. It wraps
// HeroUI Pro's KPI: pure-number values flow through KPI.Value (tabular, animated
// formatting); composite values like "5/8" render as a styled node.
export function Stat({
  label,
  value,
  tone,
  hint,
}: {
  label: string;
  value: React.ReactNode;
  tone?: "success" | "warning" | "danger";
  hint?: string;
}) {
  const toneClass =
    tone === "success"
      ? "text-success"
      : tone === "warning"
        ? "text-warning"
        : tone === "danger"
          ? "text-danger"
          : "text-foreground";
  return (
    <KPI>
      <KPI.Header>
        <KPI.Title>{label}</KPI.Title>
      </KPI.Header>
      <KPI.Content>
        {typeof value === "number" ? (
          <KPI.Value className={toneClass} maximumFractionDigits={0} value={value} />
        ) : (
          <span className={`text-2xl font-semibold tabular-nums ${toneClass}`}>{value}</span>
        )}
      </KPI.Content>
      {hint && <KPI.Footer>{hint}</KPI.Footer>}
    </KPI>
  );
}
