import { describe, expect, it } from "vitest";
import {
  SAVINGS_PREVIEW_HASH,
  shouldShowSavingsPreview,
} from "./devSavingsPreview";

describe("Savings-only mock preview", () => {
  it("is available only at the exact preview hash during development", () => {
    expect(shouldShowSavingsPreview(SAVINGS_PREVIEW_HASH, true)).toBe(true);
    expect(shouldShowSavingsPreview("#/savings", true)).toBe(false);
    expect(shouldShowSavingsPreview("#/dashboard", true)).toBe(false);
  });

  it("cannot be used in production", () => {
    expect(shouldShowSavingsPreview(SAVINGS_PREVIEW_HASH, false)).toBe(false);
  });
});
