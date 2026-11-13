import { useEffect, useMemo, useState } from "react";
import { fetchTrajectories, type TrajectoryPoint, type TrajectoriesQuery } from "../api/rest";
import { skuKey } from "../store";
import { useTick } from "./hooks";
import { trajectoryWindow } from "./statusgrid";
import type { Win } from "./statusWindow";
import { serverNow } from "./time";

export type TrajectoryMap = Record<string, TrajectoryPoint[]>;

const MAX_CACHE_ENTRIES = 32;
const cache = new Map<string, { data?: TrajectoryMap; promise?: Promise<TrajectoryMap> }>();

function cacheKey(q: TrajectoriesQuery): string {
  return [q.project_id, q.since_ms, q.until_ms, q.interval_ms].join(":");
}

function remember(key: string, data: TrajectoryMap): void {
  cache.delete(key);
  cache.set(key, { data });
  while (cache.size > MAX_CACHE_ENTRIES) {
    const oldest = cache.keys().next().value;
    if (oldest == null) break;
    cache.delete(oldest);
  }
}

function requestBatch(q: TrajectoriesQuery, key: string): Promise<TrajectoryMap> {
  const existing = cache.get(key);
  if (existing?.data) return Promise.resolve(existing.data);
  if (existing?.promise) return existing.promise;

  const promise = fetchTrajectories(q)
    .then((rows) => {
      const data: TrajectoryMap = {};
      for (const row of rows) {
        data[skuKey(row.project_id, row.screen_id, row.sku_id)] = row.points ?? [];
      }
      remember(key, data);
      return data;
    })
    .catch((err) => {
      cache.delete(key);
      throw err;
    });
  cache.set(key, { promise });
  return promise;
}

export function useProjectTrajectories(
  projectId: number,
  win: Win,
  refreshMs: number,
  enabled = true,
): TrajectoryMap | null {
  const live = win.anchorMs === null;
  const refreshTick = useTick(enabled && live ? refreshMs : 0);
  const endMs = useMemo(() => win.anchorMs ?? serverNow() * 1000, [enabled, projectId, refreshTick, win.anchorMs]);

  const query = useMemo(() => {
    const { sinceMs, untilMs } = trajectoryWindow(win.intervalMs, endMs);
    return {
      project_id: projectId,
      since_ms: sinceMs,
      until_ms: untilMs,
      interval_ms: win.intervalMs,
    };
  }, [endMs, projectId, win.intervalMs]);

  const key = enabled && projectId > 0 ? cacheKey(query) : "";
  const [result, setResult] = useState<{ projectId: number; data: TrajectoryMap | null }>({
    projectId,
    data: key ? (cache.get(key)?.data ?? null) : null,
  });

  useEffect(() => {
    if (!enabled || projectId <= 0) {
      setResult({ projectId, data: null });
      return;
    }

    let alive = true;
    const cached = cache.get(key)?.data;
    if (cached) {
      setResult({ projectId, data: cached });
    } else {
      setResult((cur) => (cur.projectId === projectId ? cur : { projectId, data: null }));
    }

    requestBatch(query, key)
      .then((data) => alive && setResult({ projectId, data }))
      .catch(() => {
        if (alive) setResult((cur) => (cur.projectId === projectId ? cur : { projectId, data: null }));
      });

    return () => {
      alive = false;
    };
  }, [enabled, key, projectId, query]);

  return result.projectId === projectId ? result.data : null;
}
