import { describe, expect, it } from "vitest";
import { buyable, statusKind, stockLabel, stockState } from "./flag";

describe("flag semantics", () => {
  it("buyable requires an on-sale flag and clickable", () => {
    expect(buyable(2, true)).toBe(true);
    expect(buyable(2, false)).toBe(false);
    expect(buyable(6, true)).toBe(true); // 余票紧张 is still buyable
    expect(buyable(4, true)).toBe(false);
  });

  it("classifies status kind", () => {
    expect(statusKind(2, true, "在售")).toBe("buyable");
    expect(statusKind(6, true, "余票紧张")).toBe("tight");
    expect(statusKind(1, false, "未开售")).toBe("presale");
    expect(statusKind(4, false, "已售罄")).toBe("soldout");
    expect(statusKind(5, false, "不可售")).toBe("unavailable");
    expect(statusKind(102, false, "已结束")).toBe("ended");
  });
});

describe("stock state (num vs limit)", () => {
  it("ample when num >= limit (顶格)", () => {
    expect(stockState(8, 8, 2)).toEqual({ kind: "ample", remaining: 8 });
  });
  it("low when 0 < num < limit, exposing exact remaining", () => {
    const s = stockState(3, 8, 2);
    expect(s).toEqual({ kind: "low", remaining: 3 });
    expect(stockLabel(s)).toBe("告急 3 张");
  });
  it("out when num=0", () => {
    expect(stockState(0, 8, 4)).toEqual({ kind: "out", remaining: 0 });
  });
  it("falls back to flag when no limit info", () => {
    expect(stockState(0, 0, 4).kind).toBe("out");
    expect(stockState(0, 0, 2).kind).toBe("unknown");
  });
});
