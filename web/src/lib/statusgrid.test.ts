import { describe, expect, it } from "vitest";
import type { RestEvent } from "../api/rest";
import {
  bestGridStatus,
  buildGrid,
  cellCountFor,
  gridStatusOf,
  mergePoint,
  panAnchor,
  panStepMs,
  pointsFromEvents,
  STATUS_GRID_CELL_COUNT,
  trajectoryWindow,
  WINDOW_PRESETS,
  type StatusPoint,
} from "./statusgrid";

function ev(server_ts: number, new_flag: number, new_clickable: boolean, flag_text = ""): RestEvent {
  return {
    project_id: 1,
    screen_id: 1,
    sku_id: 1,
    event_type: "sku_onsale",
    severity: "info",
    old_flag: 0,
    new_flag,
    old_clickable: false,
    new_clickable,
    old_price_fen: 0,
    price_fen: 0,
    old_num: 0,
    new_num: 0,
    flag_text,
    server_ts,
    node_id: "",
    created_at: "",
  };
}

describe("gridStatusOf", () => {
  it("maps flag/clickable/flag_text to grid status", () => {
    expect(gridStatusOf(2, true, "在售")).toBe("available");
    expect(gridStatusOf(6, true, "余票紧张")).toBe("available");
    expect(gridStatusOf(1, false, "未开售")).toBe("presale");
    expect(gridStatusOf(4, false, "已售罄")).toBe("soldout");
    expect(gridStatusOf(5, false, "不可售")).toBe("soldout");
    expect(gridStatusOf(102, false, "已结束")).toBe("ended");
    expect(gridStatusOf(0, false, "")).toBe("nodata");
  });
});

describe("bestGridStatus", () => {
  it("surfaces the most actionable status from a bucket", () => {
    expect(bestGridStatus(["soldout", "available", "presale"])).toBe("available");
    expect(bestGridStatus(["nodata", "presale", "soldout"])).toBe("soldout");
  });
});

describe("pointsFromEvents", () => {
  it("derives sorted status points from the post-change fingerprint", () => {
    const pts = pointsFromEvents([ev(200, 2, true), ev(100, 1, false), ev(300, 4, false)]);
    expect(pts.map((p) => p.status)).toEqual(["presale", "available", "soldout"]);
    expect(pts.map((p) => p.time)).toEqual([100_000, 200_000, 300_000]);
  });
});

describe("mergePoint", () => {
  it("inserts sorted and dedups exact (time,status)", () => {
    let pts: StatusPoint[] = [{ time: 100, status: "available" }];
    pts = mergePoint(pts, { time: 50, status: "presale" });
    expect(pts.map((p) => p.time)).toEqual([50, 100]);
    const same = mergePoint(pts, { time: 100, status: "available" });
    expect(same).toBe(pts); // unchanged reference on dedup
  });
});

describe("buildGrid", () => {
  const intervalMs = 1000;
  const cellCount = 5;

  it("without fillUntil, leaves gaps as no-report (diff-only default)", () => {
    // now = 5000ms aligned → cells start at 1000,2000,3000,4000,5000
    const points: StatusPoint[] = [{ time: 2500, status: "available" }];
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points });
    expect(cells).toHaveLength(5);
    // bucket [2000,3000) holds the change → real
    const real = cells.find((c) => c.time === 2000)!;
    expect(real.status).toBe("available");
    expect(real.hasData).toBe(true);
    // bucket [3000,4000) has no observation and no liveness → no fill.
    const gap = cells.find((c) => c.time === 3000)!;
    expect(gap.status).toBe("nodata");
    expect(gap.hasData).toBe(false);
    expect(gap.filled).toBe(false);
    // bucket [1000,2000) before any data → nodata as well
    const before = cells.find((c) => c.time === 1000)!;
    expect(before.status).toBe("nodata");
    expect(before.hasData).toBe(false);
  });

  it("forward-fills empty buckets with the last status up to fillUntil", () => {
    const points: StatusPoint[] = [{ time: 2500, status: "soldout" }];
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points, fillUntil: 5000 });
    expect(cells.find((c) => c.time === 2000)!.hasData).toBe(true);
    for (const t of [3000, 4000, 5000]) {
      const c = cells.find((x) => x.time === t)!;
      expect(c.status).toBe("soldout"); // carried forward
      expect(c.hasData).toBe(false);
      expect(c.filled).toBe(true);
    }
  });

  it("stops forward-fill past fillUntil so a collection gap stays no-data", () => {
    const points: StatusPoint[] = [{ time: 2500, status: "soldout" }];
    // collection stalled: liveness only through bucket start 3000.
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points, fillUntil: 3000 });
    const filled = cells.find((c) => c.time === 3000)!;
    expect(filled.filled).toBe(true);
    expect(filled.status).toBe("soldout");
    const gap = cells.find((c) => c.time === 4000)!; // start 4000 > fillUntil
    expect(gap.status).toBe("nodata");
    expect(gap.hasData).toBe(false);
    expect(gap.filled).toBe(false);
  });

  it("forward-fills from a state observed before the window opens", () => {
    // Last change at 500ms — before the window start (1000). With liveness through
    // `now`, the whole window should carry that state instead of showing no-data.
    const points: StatusPoint[] = [{ time: 500, status: "soldout" }];
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points, fillUntil: 5000 });
    for (const t of [1000, 2000, 3000, 4000, 5000]) {
      const c = cells.find((x) => x.time === t)!;
      expect(c.status).toBe("soldout");
      expect(c.filled).toBe(true);
      expect(c.hasData).toBe(false);
    }
  });

  it("carries the chronologically last status, not the priority winner", () => {
    // Bucket [2000,3000): available@2100 then soldout@2900. The cell shows available
    // (most actionable), but later empty buckets must carry soldout (what holds).
    const points: StatusPoint[] = [
      { time: 2100, status: "available" },
      { time: 2900, status: "soldout" },
    ];
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points, fillUntil: 5000 });
    const bucket = cells.find((c) => c.time === 2000)!;
    expect(bucket.status).toBe("available");
    expect(bucket.hasData).toBe(true);
    const after = cells.find((c) => c.time === 3000)!;
    expect(after.status).toBe("soldout"); // carried = last by time
    expect(after.filled).toBe(true);
  });

  it("does not forward-fill before the first observation", () => {
    const points: StatusPoint[] = [{ time: 3500, status: "available" }];
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points, fillUntil: 5000 });
    for (const t of [1000, 2000]) {
      const c = cells.find((x) => x.time === t)!;
      expect(c.status).toBe("nodata"); // nothing to carry yet
      expect(c.filled).toBe(false);
    }
    expect(cells.find((c) => c.time === 3000)!.hasData).toBe(true);
  });

  it("applies priority when multiple statuses land in one bucket", () => {
    // two changes inside [2000,3000): soldout then available → available wins (higher priority)
    const points: StatusPoint[] = [
      { time: 2100, status: "soldout" },
      { time: 2900, status: "available" },
    ];
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points });
    const bucket = cells.find((c) => c.time === 2000)!;
    expect(bucket.status).toBe("available");
    expect(bucket.hasData).toBe(true);
  });

  it("includes a change at `now` in the last cell", () => {
    const points: StatusPoint[] = [{ time: 5000, status: "soldout" }];
    const cells = buildGrid({ now: 5000, intervalMs, cellCount, points });
    const last = cells[cells.length - 1];
    expect(last.time).toBe(5000);
    expect(last.status).toBe("soldout");
    expect(last.hasData).toBe(true);
  });
});

describe("cellCountFor", () => {
  it("clamps to [15,120]", () => {
    expect(cellCountFor(0)).toBe(60); // fallback
    expect(cellCountFor(50)).toBe(15); // tiny → min
    expect(cellCountFor(100_000)).toBe(120); // huge → max
    expect(cellCountFor(12 * 60)).toBe(60); // 720px / 12px ≈ 60
  });
});

describe("WINDOW_PRESETS", () => {
  it("covers the 3-day and 7-day windows across the fixed grid", () => {
    const threeDays = WINDOW_PRESETS.find((p) => p.label === "3天")!;
    const sevenDays = WINDOW_PRESETS.find((p) => p.label === "7天")!;
    expect(threeDays.intervalMs * STATUS_GRID_CELL_COUNT).toBe(3 * 24 * 60 * 60 * 1000);
    expect(sevenDays.intervalMs * STATUS_GRID_CELL_COUNT).toBe(7 * 24 * 60 * 60 * 1000);
  });

  it("offers a 1-minute window as the finest zoom (sub-second per cell)", () => {
    const oneMin = WINDOW_PRESETS.find((p) => p.label === "1分钟")!;
    expect(oneMin.intervalMs * STATUS_GRID_CELL_COUNT).toBe(60 * 1000);
    expect(oneMin.intervalMs).toBeLessThan(WINDOW_PRESETS.find((p) => p.label === "3分钟")!.intervalMs);
  });
});

describe("trajectoryWindow / pan", () => {
  it("spans exactly cellCount buckets ending at endMs", () => {
    const { sinceMs, untilMs } = trajectoryWindow(1000, 100_000, 100);
    expect(untilMs).toBe(100_000);
    expect(sinceMs).toBe(100_000 - 1000 * 99);
    // server iterates t from since..until inclusive → cellCount points
    expect((untilMs - sinceMs) / 1000 + 1).toBe(100);
  });

  it("pans back by half a window and snaps forward to live at now", () => {
    const interval = 1000;
    const now = 1_000_000;
    const step = panStepMs(interval, 100);
    expect(step).toBe(50_000);
    // from live (end=now), ← 更早 → a history anchor half a window back
    expect(panAnchor(now, now, interval, -1, 100)).toBe(now - step);
    // 更新 → from there reaches now → snap back to live (null)
    expect(panAnchor(now - step, now, interval, 1, 100)).toBeNull();
    // a forward pan that still stays in the past stays in history
    expect(panAnchor(now - 3 * step, now, interval, 1, 100)).toBe(now - 2 * step);
  });
});
