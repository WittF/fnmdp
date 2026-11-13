import { describe, it, expect } from "vitest";
import { resolveLayout, type Layout } from "./skuLayout";
import type { RestSku } from "../api/rest";

function mkSku(screen: number, sku: number): RestSku {
  return {
    project_id: 1,
    screen_id: screen,
    sku_id: sku,
    desc: `档${sku}`,
    flag: 2,
    flag_text: "",
    clickable: true,
    price_fen: 0,
    num: 0,
    limit_num: 0,
    server_ts: 0,
    last_event_ts: 0,
    updated_at: "",
  };
}
const k = (screen: number, sku: number) => `1:${screen}:${sku}`;

describe("resolveLayout", () => {
  it("no stored layout → all SKUs as sku blocks in screen/sku order", () => {
    const skus = [mkSku(2, 5), mkSku(1, 9), mkSku(1, 3)];
    const blocks = resolveLayout(undefined, skus);
    expect(blocks.map((b) => (b.kind === "sku" ? b.key : `grp:${b.id}`))).toEqual([
      k(1, 3),
      k(1, 9),
      k(2, 5),
    ]);
  });

  it("respects stored order and resolves a group", () => {
    const skus = [mkSku(1, 1), mkSku(1, 2), mkSku(1, 3)];
    const layout: Layout = {
      order: [
        { kind: "group", id: "g1" },
        { kind: "sku", key: k(1, 1) },
      ],
      groups: [{ id: "g1", name: "组A", members: [k(1, 3), k(1, 2)] }],
    };
    const blocks = resolveLayout(layout, skus);
    expect(blocks).toHaveLength(2);
    expect(blocks[0]).toMatchObject({ kind: "group", id: "g1", name: "组A" });
    if (blocks[0].kind === "group") {
      expect(blocks[0].members.map((m) => m.key)).toEqual([k(1, 3), k(1, 2)]);
    }
    expect(blocks[1]).toMatchObject({ kind: "sku", key: k(1, 1) });
  });

  it("prunes vanished SKUs, drops emptied groups, appends new SKUs", () => {
    const skus = [mkSku(1, 1), mkSku(1, 9)]; // 1:1:2 gone; 1:1:9 is new
    const layout: Layout = {
      order: [
        { kind: "group", id: "gEmpty" },
        { kind: "sku", key: k(1, 1) },
      ],
      groups: [{ id: "gEmpty", name: "空组", members: [k(1, 2)] }],
    };
    const blocks = resolveLayout(layout, skus);
    // emptied group dropped, ordered sku kept, new sku appended
    expect(blocks.map((b) => (b.kind === "sku" ? b.key : `grp:${b.id}`))).toEqual([k(1, 1), k(1, 9)]);
  });

  it("never places a SKU in two blocks (group membership wins)", () => {
    const skus = [mkSku(1, 1), mkSku(1, 2)];
    const layout: Layout = {
      order: [
        { kind: "sku", key: k(1, 1) },
        { kind: "group", id: "g1" },
      ],
      groups: [{ id: "g1", name: "g", members: [k(1, 1), k(1, 2)] }],
    };
    const blocks = resolveLayout(layout, skus);
    // 1:1 already used as top-level sku → group only keeps 1:2
    const grp = blocks.find((b) => b.kind === "group");
    expect(grp && grp.kind === "group" && grp.members.map((m) => m.key)).toEqual([k(1, 2)]);
  });
});
