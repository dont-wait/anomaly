import { describe, it, expect } from "vitest";
import { formatDateTime, formatMonth } from "./index";

describe("formatDateTime", () => {
  it("should format date correctly", () => {
    const date = new Date("2026-08-08T15:30:45");
    expect(formatDateTime(date)).toBe("8/8/2026, 15:30:45");
  });

  it("should handle single digit month/day", () => {
    const date = new Date("2026-01-05T09:05:01");
    expect(formatDateTime(date)).toBe("1/5/2026, 09:05:01");
  });
});

describe("formatMonth", () => {
  it("should return full month name", () => {
    const date = new Date(2026, 7, 8);
    expect(formatMonth(date)).toBe("August");
  });

  it("should return correct month for different dates", () => {
    expect(formatMonth(new Date(2026, 11, 25))).toBe("December");
    expect(formatMonth(new Date(2026, 0, 1))).toBe("January");
  });
});
