// Status-timeline grid ("格子状态图").
// Each cell is a time bucket reconstructed by the master from sparse state events
// plus project-level poll coverage. Buckets with in-bucket changes are solid;
// buckets whose state is inferred-held from coverage are slightly faded; uncovered
// buckets are no-report.
//
// The window (interval 粒度 + pan anchor 范围) is either OWNED locally (uncontrolled:
// the `full` variant renders its own StatusWindowControl; `compact` stays a live
// read-only strip) or CONTROLLED via the `win` prop — used by the project board so a
// single shared/aggregation-group control drives many grids at once (no per-grid
// toolbar then).
import { useEffect, useMemo, useState } from "react";
import { useStore } from "@nanostores/react";
import { $skus, skuKey } from "../store";
import { fetchTrajectory, type TrajectoryPoint } from "../api/rest";
import {
  bestGridStatus,
  DEFAULT_INTERVAL_MS,
  type GridCell,
  gridStatusLabel,
  gridStatusOf,
  gridStatusVar,
  STATUS_GRID_CELL_COUNT,
  trajectoryWindow,
  type GridStatus,
} from "../lib/statusgrid";
import { panWin, type Win } from "../lib/statusWindow";
import { StatusLegend, StatusWindowControl } from "./StatusWindowControl";
import { formatClock, serverNow } from "../lib/time";
import { useTick } from "../lib/hooks";

interface Props {
  projectId: number;
  screenId: number;
  skuId: number;
  variant?: "compact" | "full";
  // When provided, the grid is controlled (no built-in toolbar); the caller owns
  // the window via a shared StatusWindowControl.
  win?: Win;
  // Parent-provided batch data. `null` means the batch request is still pending;
  // in that state the grid renders placeholders and deliberately does not fall
  // back to its own per-SKU request.
  trajectory?: TrajectoryPoint[] | null;
}

export const FULL_REFRESH_MS = 15_000;
export const COMPACT_REFRESH_MS = 60_000;

function pointStatus(p: TrajectoryPoint): GridStatus {
  if (!p.observed) return "nodata";
  const statuses = [gridStatusOf(p.flag, p.clickable, p.flag_text)];
  for (const sample of p.samples ?? []) {
    statuses.push(gridStatusOf(sample.flag, sample.clickable, sample.flag_text));
  }
  return bestGridStatus(statuses);
}

function cellsFromTrajectory(points: TrajectoryPoint[]): GridCell[] {
  return points.map((p) => {
    const hasData = p.observed && (p.samples?.length ?? 0) > 0;
    return {
      time: p.t,
      status: pointStatus(p),
      hasData,
      filled: p.observed && !hasData,
    };
  });
}

function emptyCells(intervalMs: number, endMs: number): GridCell[] {
  const { sinceMs } = trajectoryWindow(intervalMs, endMs);
  return Array.from({ length: STATUS_GRID_CELL_COUNT }, (_, i) => ({
    time: sinceMs + i * intervalMs,
    status: "nodata" as GridStatus,
    hasData: false,
    filled: false,
  }));
}

export function StatusGrid({ projectId, screenId, skuId, variant = "compact", win, trajectory: batchTrajectory }: Props) {
  const skus = useStore($skus);
  const sku = skus[skuKey(projectId, screenId, skuId)];
  const controlled = win != null;
  const hasBatchTrajectory = batchTrajectory !== undefined;
  // Local window — only used when uncontrolled.
  const [localInterval, setLocalInterval] = useState(DEFAULT_INTERVAL_MS);
  const [localAnchor, setLocalAnchor] = useState<number | null>(null);
  const intervalMs = controlled ? win.intervalMs : localInterval;
  const anchorMs = controlled ? win.anchorMs : localAnchor;
  const [localTrajectory, setLocalTrajectory] = useState<TrajectoryPoint[]>([]);
  const refreshTick = useTick(variant === "full" ? FULL_REFRESH_MS : COMPACT_REFRESH_MS);

  const live = anchorMs === null;
  const endMs = anchorMs ?? serverNow() * 1000;
  // History mode freezes the window: the periodic live tick must not refetch/advance
  // it. sku changes still refresh the (frozen) window, which is harmless.
  const liveTick = live ? refreshTick : 0;

  useEffect(() => {
    if (hasBatchTrajectory) return;
    if (!skuId) return;
    let alive = true;
    const end = anchorMs ?? serverNow() * 1000;
    const { sinceMs, untilMs } = trajectoryWindow(intervalMs, end);
    fetchTrajectory({
      project_id: projectId,
      screen_id: screenId,
      sku_id: skuId,
      since_ms: sinceMs,
      until_ms: untilMs,
      interval_ms: intervalMs,
    })
      .then((rows) => alive && setLocalTrajectory(rows))
      .catch(() => alive && setLocalTrajectory([]));
    return () => {
      alive = false;
    };
  }, [
    hasBatchTrajectory,
    projectId,
    screenId,
    skuId,
    intervalMs,
    anchorMs,
    liveTick,
    sku?.server_ts,
    sku?.flag,
    sku?.clickable,
    sku?.flag_text,
  ]);

  const trajectory = hasBatchTrajectory ? (batchTrajectory ?? []) : localTrajectory;
  const cells = useMemo(() => {
    return trajectory.length > 0 ? cellsFromTrajectory(trajectory) : emptyCells(intervalMs, endMs);
  }, [intervalMs, trajectory, endMs]);

  const localWin: Win = { intervalMs: localInterval, anchorMs: localAnchor };

  return (
    <div className="w-full space-y-1.5">
      {variant === "full" && !controlled && (
        <div className="space-y-1.5">
          <StatusLegend />
          <StatusWindowControl
            win={localWin}
            onWindow={(ms) => setLocalInterval(ms)}
            onPan={(dir) => setLocalAnchor(panWin(localWin, dir).anchorMs)}
            onReset={() => setLocalAnchor(null)}
          />
        </div>
      )}

      <div
        className="grid w-full overflow-hidden rounded-md bg-surface-secondary px-1.5 py-2"
        style={{ gridTemplateColumns: `repeat(${STATUS_GRID_CELL_COUNT}, minmax(0, 1fr))`, gap: "1px" }}
      >
        {cells.map((c, i) => (
          <div
            key={i}
            role="img"
            aria-label={gridStatusLabel[c.status]}
            title={`${formatClock(Math.floor(c.time / 1000))} · ${gridStatusLabel[c.status]}${
              c.hasData ? "" : c.filled ? "（推断：状态保持）" : "（该时间桶未覆盖采集）"
            }`}
            className="h-6 min-w-0 rounded-sm transition-transform duration-150 hover:scale-y-110"
            style={{
              backgroundColor: gridStatusVar[c.status],
              // observed = solid; inferred-held = slightly faded; no-data = faint + outline.
              opacity: c.hasData ? 1 : c.filled ? 0.85 : 0.26,
              border: c.hasData || c.filled ? "none" : "1px solid var(--grid-nodata)",
            }}
          />
        ))}
      </div>
    </div>
  );
}
