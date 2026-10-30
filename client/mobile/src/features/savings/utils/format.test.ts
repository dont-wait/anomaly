import { describe, expect, it } from "vitest";
import { SAVINGS_TERMS, addMonths, estimateSavings, parseAmount } from "./format";

describe("savings quote preview (UI only)", () => {
  it("uses the seven product terms from UC-10", () => {
    expect(SAVINGS_TERMS.map((term) => term.months)).toEqual([3, 6, 9, 12, 24, 36, 60]);
    expect(SAVINGS_TERMS.map((term) => term.rate)).toEqual([4.5, 4.8, 5, 5.2, 5.5, 5.8, 6]);
  });

  it("clamps month-end dates", () => {
    expect(addMonths("2026-01-31T00:00:00.000Z", 3)).toBe("2026-04-30T00:00:00.000Z");
    expect(addMonths("2024-02-29T00:00:00.000Z", 12)).toBe("2025-02-28T00:00:00.000Z");
  });

  it("calculates UI estimate without changing the input principal", () => {
    const preview = estimateSavings(10_000_000, 12, "2026-10-29T00:00:00.000Z");
    expect(preview.amount).toBe(10_000_000);
    expect(preview.annualRate).toBe(5.2);
    expect(preview.interest).toBe(520_000);
  });

  it("rejects unsupported terms and insufficient principal", () => {
    expect(() => estimateSavings(999_999, 3, "2026-10-29T00:00:00.000Z")).toThrow();
    expect(() => estimateSavings(1_000_000, 1, "2026-10-29T00:00:00.000Z")).toThrow();
  });

  it("parses pasted VND amounts safely", () => {
    expect(parseAmount("10.000.000")).toBe(10_000_000);
    expect(parseAmount("10 000 000")).toBe(10_000_000);
    expect(Number.isNaN(parseAmount("abc"))).toBe(true);
    expect(Number.isNaN(parseAmount("999999999999999999999999"))).toBe(true);
  });
});
