import { expect, it } from "vitest";
import { buildAccountQr, parseAccountQr } from "./payload";

it("reads back the account number it encoded", () => {
  expect(parseAccountQr(buildAccountQr("1234567890"))).toBe("1234567890");
});

it.each([
  "1234567890",
  "https://evil.example/anomalybank://transfer?accountNo=1234567890",
  "anomalybank://transfer?accountNo=",
  "anomalybank://transfer?accountNo=12345",
  "anomalybank://transfer?accountNo=1234567890&amount=9000000",
  "00020101021138570010A000000727",
])("rejects %s", (text) => {
  expect(parseAccountQr(text)).toBeNull();
});
