import { isReportDate, parseTransactionReport } from "./reportValidation";
import { reportFixture } from "../testFixtures";

describe("report wire validation", () => {
  it("keeps legitimate zero, null metadata and precise revision strings without inventing data", () => {
    const data = reportFixture();
    data.projection.checkpointRevision = "90071992547409931234";
    data.generatedAt = "2026-11-08T05:00:00.123456789Z";
    const parsed = parseTransactionReport({ data }, data.date);
    expect(parsed).toBe(data);
    expect(parsed.transactions[0].selectedAssessment).toBeNull();
    expect(parsed.transactions[1].selectedAssessment?.riskScore).toBe(0);
    expect(parsed.transactions[1].selectedAssessment?.threshold).toBeNull();
    expect(parsed.projection.checkpointRevision).toBe("90071992547409931234");
  });

  it.each([
    ["previous", null],
    ["projection", null],
    ["dataQuality", null],
    ["topRiskSources", undefined],
    ["transactions.0.createdAt", "not-a-date"],
    ["transactions.0.createdAt", "2026-02-30T00:00:00Z"],
    ["transactions.0.createdAt", "2026-11-08T24:00:00Z"],
    ["transactions.0.createdAt", "2026-11-08T00:00:00"],
    ["transactions.0.effectiveStatus", "awaiting_otp"],
    ["transactions.0.amount", 0],
    ["transactions.0.amount", Number.MAX_SAFE_INTEGER + 1],
    ["transactions.0.fee", -1],
    ["transactions.1.selectedAssessment.riskScore", NaN],
    ["transactions.1.selectedAssessment.riskScore", Infinity],
    ["transactions.1.selectedAssessment.riskScore", 1.1],
    ["transactions.1.selectedAssessment.riskScore", -0.1],
    ["transactions.1.selectedAssessment.threshold", undefined],
    ["transactions.1.selectedAssessment.decision", "PENDING"],
    ["transactions.1.selectedAssessment.receivedAt", "bad-time"],
    ["summary.created.transactionCount", "2"],
    ["summary.created.expiredCount", undefined],
    ["summary.completed.totalAmount", Number.MAX_SAFE_INTEGER + 1],
    ["summary.completed.averageAmount", -1],
    ["summary.risk.averageScore", 2],
    ["summary.risk.coverage", NaN],
    ["summary.risk.assessedCount", 0.5],
    ["hourly.created.0.hour", "01:00"],
    ["hourly.created.0.count", -1],
    ["hourly.completed", []],
    ["projection.checkpointRevision", 9007199254740992],
    ["projection.status", "fresh"],
    ["timezone", "UTC"],
    ["currency", "USD"],
  ])("rejects malformed %s instead of coercing it to zero", (path, value) => {
    const data = reportFixture();
    const keys = path.split(".");
    let target = data as unknown as Record<string, unknown>;
    for (const key of keys.slice(0, -1))
      target = target[key] as Record<string, unknown>;
    target[keys[keys.length - 1]] = value;
    expect(() => parseTransactionReport({ data }, data.date)).toThrow(
      /cấu trúc hoặc ngày/,
    );
  });

  it("rejects missing selected source and metadata mixed from a different assessment", () => {
    const data = reportFixture();
    data.transactions[1].assessmentSource = "none";
    expect(() => parseTransactionReport({ data }, data.date)).toThrow();
    data.transactions[1].assessmentSource = "applied";
    expect(() => parseTransactionReport({ data }, data.date)).toThrow();
    data.transactions[1].assessmentSource = "observed";
    data.transactions[1].selectedAssessment = {
      ...data.transactions[1].selectedAssessment!,
      modelVersion: "unrelated-model",
    };
    expect(() => parseTransactionReport({ data }, data.date)).toThrow();
  });

  it("allows legacy applied risk with unknown decision without using latest assessment metadata", () => {
    const data = reportFixture();
    const applied = {
      ...data.transactions[1].selectedAssessment!,
      assessmentId: null,
      decision: null,
      receivedAt: null,
    };
    data.transactions[1].appliedRisk = applied;
    data.transactions[1].selectedAssessment = applied;
    data.transactions[1].assessmentSource = "applied";
    const parsed = parseTransactionReport({ data }, data.date);
    expect(parsed.transactions[1].selectedAssessment?.decision).toBeNull();
    expect(parsed.transactions[1].selectedAssessment?.receivedAt).toBeNull();
  });

  it("allows nullable invalid-money rows and quality warnings from the server", () => {
    const data = reportFixture();
    data.transactions[0].amount = null;
    data.transactions[0].fee = null;
    data.transactions[0].currency = "USD";
    data.dataQuality = [
      { code: "INVALID_MONEY", scope: "created", date: data.date, count: 1 },
    ];
    expect(
      parseTransactionReport({ data }, data.date).transactions[0].amount,
    ).toBeNull();
  });

  it.each([
    "",
    "2026-02-30",
    "2026-1-08",
    " 2026-11-08",
    "2026-11-08T00:00:00Z",
    null,
  ])("rejects invalid calendar date %s", (date) => {
    expect(isReportDate(date)).toBe(false);
  });
  it.each(["2026-11-08", "2024-02-29"])(
    "accepts valid calendar date %s",
    (date) => {
      expect(isReportDate(date)).toBe(true);
    },
  );
});
