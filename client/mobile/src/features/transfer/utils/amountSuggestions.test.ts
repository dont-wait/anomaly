import { expect, it } from "vitest";
import { suggestAmounts } from "./amountSuggestions";

it("multiplies the typed digits by powers of ten", () => {
  expect(suggestAmounts(8, 100_000_000)).toEqual([
    8_000, 80_000, 800_000, 8_000_000,
  ]);
  expect(suggestAmounts(25, 100_000_000)).toEqual([
    2_500, 25_000, 250_000, 2_500_000,
  ]);
  expect(suggestAmounts(5_000, 100_000_000)).toEqual([
    50_000, 500_000, 5_000_000, 50_000_000,
  ]);
});

it("never suggests more than the available balance", () => {
  expect(suggestAmounts(8, 1_000_000)).toEqual([8_000, 80_000, 800_000]);
  expect(suggestAmounts(9, 5_000)).toEqual([]);
});

it("suggests nothing before the user types an amount", () => {
  expect(suggestAmounts(0, 100_000_000)).toEqual([]);
});
