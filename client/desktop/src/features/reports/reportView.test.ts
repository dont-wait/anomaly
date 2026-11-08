import {
  filterReportItems,
  formatReportTimestamp,
  type StatusFilter,
} from "./reportView";
import { reportFixture, reportItem } from "./testFixtures";

describe("report filtering", () => {
  it.each([
    "success",
    "failed",
    "cancelled",
    "expired",
    "pending",
    "unknown",
  ] as const)(
    "filters %s by effective status, not canonical OTP status",
    (status) => {
      const items = (
        [
          "success",
          "failed",
          "cancelled",
          "expired",
          "pending",
          "unknown",
        ] as const
      ).map((effectiveStatus) =>
        reportItem({ transactionId: effectiveStatus, effectiveStatus }),
      );
      expect(
        filterReportItems(items, status as StatusFilter, "all", "").map(
          (item) => item.transactionId,
        ),
      ).toEqual([status]);
    },
  );

  it("keeps unknown decisions separate from missing assessments", () => {
    const known = reportFixture().transactions[1];
    const unknown = reportItem({
      transactionId: "unknown",
      selectedAssessment: { ...known.selectedAssessment!, decision: null },
    });
    const missing = reportItem({ transactionId: "missing" });
    const blocked = reportItem({
      transactionId: "blocked",
      selectedAssessment: {
        ...known.selectedAssessment!,
        decision: "BLOCK",
        riskScore: 0.9,
      },
    });
    const items = [known, unknown, missing, blocked];
    expect(filterReportItems(items, "all", "unassessed", "")).toEqual([
      missing,
    ]);
    expect(filterReportItems(items, "all", "unknown", "")).toEqual([unknown]);
    expect(filterReportItems(items, "all", "ALLOW", "")).toEqual([known]);
    expect(filterReportItems(items, "all", "BLOCK", "")).toEqual([blocked]);
  });

  it.each([
    "  nguyen dang  ",
    "NGUYỄN ĐĂNG",
    "000001",
    "transfer-123",
    "TRAN THI B",
  ])(
    "searches account, ID and Vietnamese names consistently for %s",
    (query) => {
      const item = reportItem({
        transactionId: "transfer-123",
        sourceAccountName: "Nguyễn Đăng",
        destinationName: "Trần Thị B",
      });
      expect(filterReportItems([item], "all", "all", query)).toEqual([item]);
    },
  );

  it("does not reorder, truncate or mutate a report with more than 50 transactions", () => {
    const items = Array.from({ length: 61 }, (_, index) =>
      reportItem({ transactionId: `tx-${index}` }),
    );
    const before = structuredClone(items);
    const result = filterReportItems(items, "all", "all", "");
    expect(result).toHaveLength(61);
    expect(result[60].transactionId).toBe("tx-60");
    expect(items).toEqual(before);
  });
});

describe("report timestamps", () => {
  it("uses Vietnam time across midnight regardless of the device default", () => {
    const formatted = formatReportTimestamp("2026-11-08T17:00:00Z");
    expect(formatted).toContain("00:00");
    expect(formatted).toContain("9 thg 11");
  });
  it.each([null, undefined, "", "invalid"])(
    "does not invent a time for %s",
    (value) => {
      expect(formatReportTimestamp(value)).toBe("—");
    },
  );
});
