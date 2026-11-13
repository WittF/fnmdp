import { describe, expect, it } from "vitest";
import { priceBand, yuan } from "./price";

describe("price", () => {
  it("converts fen to yuan, dropping trailing zeros", () => {
    expect(yuan(138000)).toBe("1380");
    expect(yuan(12800)).toBe("128");
    expect(yuan(12850)).toBe("128.5");
    expect(yuan(12834)).toBe("128.34");
  });

  it("renders price bands", () => {
    expect(priceBand(12800, 138000)).toBe("¥128–1380");
    expect(priceBand(12800, 12800)).toBe("¥128");
    expect(priceBand(0, 0)).toBe("—");
    expect(priceBand(0, 12800)).toBe("¥128");
  });
});
