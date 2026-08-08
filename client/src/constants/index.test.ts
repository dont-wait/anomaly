import { describe, it, expect } from "vitest";
import {
  API_BASE_URL,
  APP_NAME,
  TRANSACTION_LIMITS,
} from "./index";

describe("constants", () => {
  describe("API_BASE_URL", () => {
    it("should return a string", () => {
      expect(typeof API_BASE_URL).toBe("string");
    });

    it("should have a valid URL format", () => {
      expect(API_BASE_URL).toMatch(/^https?:\/\/.+/);
    });
  });

  describe("APP_NAME", () => {
    it("should be 'Anomaly'", () => {
      expect(APP_NAME).toBe("Anomaly");
    });
  });

  describe("TRANSACTION_LIMITS", () => {
    it("should have DAILY_WITHDRAWAL as 50,000,000", () => {
      expect(TRANSACTION_LIMITS.DAILY_WITHDRAWAL).toBe(50_000_000);
    });

    it("should have SINGLE_WITHDRAWAL as 10,000,000", () => {
      expect(TRANSACTION_LIMITS.SINGLE_WITHDRAWAL).toBe(10_000_000);
    });

    it("should have DAILY_WITHDRAWAL >= SINGLE_WITHDRAWAL", () => {
      expect(TRANSACTION_LIMITS.DAILY_WITHDRAWAL).toBeGreaterThanOrEqual(
        TRANSACTION_LIMITS.SINGLE_WITHDRAWAL,
      );
    });
  });
});
