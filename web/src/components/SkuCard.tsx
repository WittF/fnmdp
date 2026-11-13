import type { ReactNode } from "react";
import { Card } from "@heroui/react";
import type { RestScreen, RestSku, TrajectoryPoint } from "../api/rest";
import { statusKind, statusText, stockState } from "../lib/flag";
import { formatDate, shouldShowDateBesideName } from "../lib/time";
import type { Win } from "../lib/statusWindow";
import { StatusGrid } from "./StatusGrid";
import { AgeStamp, StatusBadge, StockBadge } from "./primitives";

// One SKU card for the project board. The time grid is CONTROLLED by `win` (the
// shared global control or an aggregation group's), so the card carries no per-grid
// toolbar. `leading` is the drag handle, `actions` the ⋯ menu — both supplied by
// the board. Because groups can span screens/dates, every card is labelled with its
// own 场次 + 日期.
export function SkuCard({
  sku,
  screen,
  win,
  leading,
  actions,
  highlight,
  trajectory,
}: {
  sku: RestSku;
  screen?: RestScreen;
  win: Win;
  leading?: ReactNode;
  actions?: ReactNode;
  highlight?: boolean;
  trajectory?: TrajectoryPoint[] | null;
}) {
  const kind = statusKind(sku.flag, sku.clickable, sku.flag_text);
  const stock = stockState(sku.num, sku.limit_num, sku.flag);
  const scName = screen?.name?.trim() || "场次";
  const date = screen ? formatDate(screen.start_time) : "";
  const showDate = !!date && shouldShowDateBesideName(scName, date);

  return (
    <Card
      className={`space-y-2 px-3 py-2.5 transition-shadow ${
        highlight ? "ring-2 ring-accent" : ""
      }`}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        {leading}
        <StatusBadge kind={kind} />
        <span className="font-medium text-foreground">{sku.desc || `档 ${sku.sku_id}`}</span>
        {sku.flag_text && sku.flag_text !== statusText[kind] && (
          <span className="text-xs text-muted">「{sku.flag_text}」</span>
        )}
        <span className="text-xs text-muted">
          · {scName}
          {showDate ? ` ${date}` : ""}
        </span>
        <AgeStamp ts={sku.last_event_ts ?? 0} label="最后变化" />
        <span className="ml-auto flex items-center gap-3">
          <StockBadge stock={stock} />
          <span className="text-xs tabular-nums text-foreground/80">
            {sku.price_fen > 0 ? `¥${sku.price_fen / 100}` : "—"}
          </span>
          {actions}
        </span>
      </div>
      <StatusGrid
        projectId={sku.project_id}
        screenId={sku.screen_id}
        skuId={sku.sku_id}
        variant="full"
        win={win}
        trajectory={trajectory}
      />
    </Card>
  );
}
