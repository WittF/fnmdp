import { describe, expect, it } from "vitest";
import { $subs, DEFAULT_TYPES, setSubs, toggleType } from "./notify";

describe("notification subscriptions", () => {
  it("defaults to the high-value event types, opt-in disabled, sound on", () => {
    const s = $subs.get();
    expect(s.enabled).toBe(false); // opt-in
    expect(s.sound).toBe(true);
    expect(s.projectIds).toEqual([]); // all projects by default
    for (const t of DEFAULT_TYPES) expect(s.types[t]).toBe(true);
    expect(s.types.sku_soldout ?? false).toBe(false); // noisy types off by default
  });

  it("toggleType flips a single type, setSubs merges", () => {
    expect($subs.get().types.sku_restock).toBe(true);
    toggleType("sku_restock");
    expect($subs.get().types.sku_restock).toBe(false);
    toggleType("sku_restock");
    expect($subs.get().types.sku_restock).toBe(true);

    setSubs({ sound: false });
    expect($subs.get().sound).toBe(false);
    expect($subs.get().types.sku_restock).toBe(true); // unrelated keys preserved
    setSubs({ sound: true });
  });
});
