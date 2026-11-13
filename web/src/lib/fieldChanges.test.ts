import { describe, expect, it } from "vitest";
import { fieldChanges } from "./fieldChanges";
import type { FeedEvent } from "../store/normalize";
import type { Fingerprint } from "../protocol/types";

function ev(partial: Partial<FeedEvent>): FeedEvent {
  return {
    id: "x",
    event_type: "sku_onsale",
    severity: "notable",
    project_id: 1,
    screen_id: 10,
    sku_id: 100,
    old: null,
    new: null,
    price_fen: 0,
    flag_text: "",
    node_id: "",
    server_ts: 0,
    seq: 0,
    ...partial,
  };
}

const fp = (o: Partial<Fingerprint>): Fingerprint => ({
  flag: 0,
  clickable: false,
  price_fen: 0,
  num: 0,
  ...o,
});

describe("fieldChanges", () => {
  it("lists only changed getV2 fields, with the flag enum + derived label", () => {
    const changes = fieldChanges(
      ev({
        old: fp({ flag: 4, clickable: false, num: 10, price_fen: 8800 }),
        new: fp({ flag: 2, clickable: true, num: 0, price_fen: 8800 }),
      }),
    );
    // price unchanged → omitted (true git-diff)
    expect(changes.map((c) => c.key)).toEqual(["sale_flag.number", "clickable", "num"]);
    const flag = changes.find((c) => c.key === "sale_flag.number")!;
    expect(flag.oldText).toBe("4 (已售罄)");
    expect(flag.newText).toBe("2 (在售)");
    expect(changes.find((c) => c.key === "clickable")).toMatchObject({ oldText: "false", newText: "true" });
    expect(changes.find((c) => c.key === "num")).toMatchObject({ oldText: "10", newText: "0" });
  });

  it("omits num/price for project-scoped events (sku_id == 0)", () => {
    const changes = fieldChanges(
      ev({
        sku_id: 0,
        screen_id: 0,
        old: fp({ flag: 1, num: 5, price_fen: 100 }),
        new: fp({ flag: 2, num: 99, price_fen: 999 }),
      }),
    );
    expect(changes.map((c) => c.key)).toEqual(["sale_flag.number"]);
  });

  it("renders the missing side as — for an added event (old == null)", () => {
    const changes = fieldChanges(
      ev({
        old: null,
        new: fp({ flag: 2, clickable: true, num: 5, price_fen: 8800 }),
      }),
    );
    expect(changes.map((c) => c.key)).toEqual(["sale_flag.number", "clickable", "num", "price"]);
    expect(changes.every((c) => c.oldText === "—")).toBe(true);
    expect(changes.find((c) => c.key === "price")).toMatchObject({ newText: "8800" });
  });

  it("derives a hotProject row from event_type (not carried in the fingerprint)", () => {
    // A pure hot toggle: flag unchanged, hot rides in details (dropped by WS) —
    // the row comes purely from event_type.
    const same = fp({ flag: 2 });
    const on = fieldChanges(ev({ event_type: "project_hot_on", sku_id: 0, screen_id: 0, old: same, new: { ...same } }));
    expect(on).toEqual([{ key: "hotProject", oldText: "false", newText: "true" }]);
    const off = fieldChanges(ev({ event_type: "project_hot_off", sku_id: 0, screen_id: 0, old: null, new: null }));
    expect(off).toEqual([{ key: "hotProject", oldText: "true", newText: "false" }]);
  });

  it("returns empty when nothing tracked changed", () => {
    const same = fp({ flag: 2, clickable: true, num: 5, price_fen: 8800 });
    expect(fieldChanges(ev({ old: same, new: { ...same } }))).toEqual([]);
  });

  it("returns empty when both fingerprints are null", () => {
    expect(fieldChanges(ev({ old: null, new: null }))).toEqual([]);
  });
});
