import { useState } from "react";
import { Link } from "react-router-dom";
import { Button, Card } from "@heroui/react";
import type { RestProject, RestScreen, RestSku, TrajectoryPoint } from "../api/rest";
import { screenSkus, skuKey } from "../store";
import { statusKind, stockState } from "../lib/flag";
import { formatDate, shouldShowDateBesideName } from "../lib/time";
import { DEFAULT_INTERVAL_MS } from "../lib/statusgrid";
import { useProjectTrajectories, type TrajectoryMap } from "../lib/trajectories";
import { COMPACT_REFRESH_MS, StatusGrid } from "./StatusGrid";
import { AgeStamp, Countdown, HotBadge, PriceBand, StatusBadge, StockBadge, WishCount } from "./primitives";

export function projectTitle(p: RestProject): string {
  return p.name?.trim() ? p.name : `项目 #${p.project_id}`;
}

const COMPACT_WIN = { intervalMs: DEFAULT_INTERVAL_MS, anchorMs: null };

function SkuRow({ sku, trajectory }: { sku: RestSku; trajectory?: TrajectoryPoint[] | null }) {
  const kind = statusKind(sku.flag, sku.clickable, sku.flag_text);
  const stock = stockState(sku.num, sku.limit_num, sku.flag);
  const highlight = kind === "tight" || stock.kind === "low" ? "bg-warning/5" : "";
  return (
    <div className={`flex items-center gap-3 px-3 py-1.5 text-sm ${highlight}`}>
      <div className="flex w-44 shrink-0 items-center gap-2">
        <StatusBadge kind={kind} />
        <span className="min-w-0 truncate text-foreground/90">{sku.desc || `档 ${sku.sku_id}`}</span>
      </div>
      {/* compact 10-minute status timeline */}
      <div className="hidden min-w-[140px] flex-1 md:block">
        <StatusGrid
          projectId={sku.project_id}
          screenId={sku.screen_id}
          skuId={sku.sku_id}
          variant="compact"
          trajectory={trajectory}
        />
      </div>
      <StockBadge stock={stock} />
      <span className="w-20 shrink-0 text-right text-xs tabular-nums text-muted">
        {sku.price_fen > 0 ? `¥${sku.price_fen / 100}` : ""}
      </span>
    </div>
  );
}

function ScreenGroup({
  screen,
  skus,
  trajectories,
}: {
  screen: RestScreen;
  skus: RestSku[];
  trajectories?: TrajectoryMap | null;
}) {
  const date = formatDate(screen.start_time);
  const name = screen.name || "场次";
  return (
    <div className="border-t border-separator/60">
      <div className="flex items-center gap-2 bg-surface-secondary/60 px-3 py-1 text-xs text-muted">
        <span className="text-foreground/80">{name}</span>
        {shouldShowDateBesideName(name, date) && <span>{date}</span>}
      </div>
      {skus.length === 0 ? (
        <div className="px-3 py-1.5 text-xs text-muted/70">暂无票档数据</div>
      ) : (
        skus.map((s) => (
          <SkuRow key={s.sku_id} sku={s} trajectory={trajectories?.[skuKey(s.project_id, s.screen_id, s.sku_id)] ?? null} />
        ))
      )}
    </div>
  );
}

// ScreenMatrix renders a project's screen→SKU grid. Shared by the overview block
// and the project detail page.
export function ScreenMatrix({
  projectID,
  screens,
  skus,
  trajectories,
}: {
  projectID: number;
  screens: RestScreen[];
  skus: Record<string, RestSku>;
  trajectories?: TrajectoryMap | null;
}) {
  if (screens.length === 0) {
    return (
      <div className="border-t border-separator/60 px-3 py-2 text-xs text-muted/70">
        等待节点回填场次/票档…
      </div>
    );
  }
  return (
    <>
      {screens.map((sc) => (
        <ScreenGroup
          key={sc.screen_id}
          screen={sc}
          skus={screenSkus(projectID, sc.screen_id, skus)}
          trajectories={trajectories}
        />
      ))}
    </>
  );
}

export function ProjectBlock({
  project,
  screens,
  skus,
}: {
  project: RestProject;
  screens: RestScreen[];
  skus: Record<string, RestSku>;
}) {
  const [open, setOpen] = useState(true);
  const trajectories = useProjectTrajectories(project.project_id, COMPACT_WIN, COMPACT_REFRESH_MS, open);
  return (
    <Card className="overflow-hidden p-0">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5">
        <Button
          isIconOnly
          variant="ghost"
          size="sm"
          onPress={() => setOpen((o) => !o)}
          aria-label={open ? "收起" : "展开"}
        >
          <span className="text-muted">{open ? "▾" : "▸"}</span>
        </Button>
        <Link
          to={`/project/${project.project_id}`}
          className="font-semibold text-foreground hover:text-link hover:underline"
        >
          {projectTitle(project)}
        </Link>
        <HotBadge hot={project.hot} />
        <AgeStamp ts={project.last_event_ts ?? 0} label="最后变化" />
        <span className="ml-auto flex flex-wrap items-center gap-x-3 gap-y-1">
          <PriceBand
            low={project.salable_low || project.price_low}
            high={project.salable_high || project.price_high}
          />
          <WishCount value={project.wish_count} />
          <Countdown saleStart={project.sale_start} />
        </span>
      </header>
      {open && <ScreenMatrix projectID={project.project_id} screens={screens} skus={skus} trajectories={trajectories} />}
    </Card>
  );
}
