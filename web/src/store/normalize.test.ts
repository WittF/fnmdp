import { describe, expect, it } from "vitest";
import type { Event as WsEvent } from "../protocol/types";
import type { RestEvent, RestProject } from "../api/rest";
import { fromRestEvent, fromWsEvent, projectSortRank } from "./normalize";

const wsRestock: WsEvent = {
  type: "event",
  version: 8,
  event_type: "sku_restock",
  severity: "critical",
  project_id: 1001,
  screen_id: 10,
  sku_id: 7,
  old: { flag: 4, clickable: false, price_fen: 8800, num: 0 },
  new: { flag: 2, clickable: true, price_fen: 8800, num: 3 },
  price_fen: 8800,
  flag_text: "在售",
  node_id: "node-1",
  details: { source: "live" },
  server_ts: 1750100000,
};

const restRestock: RestEvent = {
  project_id: 1001,
  screen_id: 10,
  sku_id: 7,
  event_type: "sku_restock",
  severity: "critical",
  old_flag: 4,
  new_flag: 2,
  old_clickable: false,
  new_clickable: true,
  old_price_fen: 8800,
  price_fen: 8800,
  old_num: 0,
  new_num: 3,
  flag_text: "在售",
  server_ts: 1750100000,
  node_id: "node-1",
  created_at: "2026-06-20T00:00:00Z",
};

describe("event normalization", () => {
  it("WS and REST shapes yield the same identity and old/new num", () => {
    const a = fromWsEvent(wsRestock);
    const b = fromRestEvent(restRestock);
    expect(a.id).toBe(b.id); // same dedup key across transports
    expect(a.new?.num).toBe(3);
    expect(b.new?.num).toBe(3);
    expect(a.old?.num).toBe(0);
    expect(b.old?.num).toBe(0);
    expect(a.flag_text).toBe("在售");
    expect(a.node_id).toBe("node-1");
    expect(a.details?.source).toBe("live");
    expect(b.node_id).toBe("node-1");
  });

  it("flattens a null old side", () => {
    const added: RestEvent = {
      ...restRestock,
      event_type: "sku_added",
      old_flag: 0,
      old_clickable: false,
      old_num: 0,
    };
    expect(fromRestEvent(added).old).toBeNull();
  });
});

describe("project sort (buyability first)", () => {
  const p = (flag: number): RestProject =>
    ({ project_id: 1, flag, hot: false } as RestProject);

  it("ranks buyable above presale above soldout", () => {
    expect(projectSortRank(p(2), [{ flag: 2, clickable: true, flag_text: "在售" }])).toBe(0);
    expect(projectSortRank(p(1), [{ flag: 1, clickable: false, flag_text: "未开售" }])).toBe(1);
    expect(projectSortRank(p(4), [{ flag: 4, clickable: false, flag_text: "已售罄" }])).toBe(3);
  });
});
