import { describe, it, expect } from "vitest";
import { EVENT_GROUPS, DEFAULT_HIDDEN } from "./eventFilter";
import { EVENT_TYPES } from "../protocol/types";

const sorted = (xs: string[]) => [...xs].sort();

describe("eventFilter EVENT_GROUPS", () => {
  it("flatten exactly equals EVENT_TYPES (exhaustive, no dupes, no gaps)", () => {
    const flat = EVENT_GROUPS.flatMap((g) => g.types);
    expect(flat.length).toBe(EVENT_TYPES.length);
    expect(sorted(flat)).toEqual(sorted([...EVENT_TYPES]));
  });

  it("DEFAULT_HIDDEN are all real event types", () => {
    for (const t of DEFAULT_HIDDEN) expect(EVENT_TYPES).toContain(t);
  });
});
