import type {
  TransactionReport,
  TransactionReportItem,
} from "./api/transactionsReportApi";

export function reportItem(
  overrides: Partial<TransactionReportItem> = {},
): TransactionReportItem {
  return {
    transactionId: "tx-unassessed",
    sourceAccountNo: "00000000000001",
    sourceAccountName: "Nguồn",
    destinationAccountNo: "00000000000002",
    destinationName: "Đích",
    amount: 100000,
    fee: 0,
    currency: "VND",
    canonicalStatus: "awaiting_otp",
    effectiveStatus: "pending",
    statusAt: null,
    failureReason: null,
    createdAt: "2026-11-08T00:00:00Z",
    completedAt: null,
    completionTimeSource: null,
    otpExpiresAt: null,
    appliedRisk: null,
    latestAssessment: null,
    selectedAssessment: null,
    assessmentSource: "none",
    ...overrides,
  };
}

export function reportFixture(date = "2026-11-08"): TransactionReport {
  const counts = {
    successCount: 0,
    failedCount: 0,
    cancelledCount: 0,
    expiredCount: 1,
    pendingCount: 1,
    unknownCount: 0,
  };
  const from = new Date(`${date}T00:00:00+07:00`).toISOString();
  const asOf = new Date(new Date(from).getTime() + 12 * 3600000).toISOString();
  const risk = {
    assessmentId: "assessment-zero",
    decision: "ALLOW" as const,
    riskScore: 0,
    threshold: null,
    modelVersion: "model",
    featureVersion: null,
    evaluatedAt: asOf,
    receivedAt: asOf,
  };
  const summary = {
    created: { ...counts, transactionCount: 2 },
    completed: {
      count: 1,
      totalAmount: 700000,
      totalFee: 700,
      averageAmount: 700000,
    },
    risk: {
      assessedCount: 1,
      unassessedCount: 1,
      averageScore: 0,
      coverage: 0.5,
      allowCount: 1,
      modelBlockCount: 0,
      unknownDecisionCount: 0,
      actualBlockedCount: 0,
    },
  };
  return {
    reportVersion: 2,
    date,
    previousDate: new Date(new Date(`${date}T00:00:00Z`).getTime() - 86400000)
      .toISOString()
      .slice(0, 10),
    timezone: "Asia/Ho_Chi_Minh",
    currency: "VND",
    from,
    to: new Date(new Date(from).getTime() + 86400000).toISOString(),
    asOf,
    generatedAt: asOf,
    projection: { status: "unknown", checkpointRevision: null },
    summary,
    previous: structuredClone(summary),
    hourly: {
      created: Array.from({ length: 24 }, (_, hour) => ({
        hour: `${String(hour).padStart(2, "0")}:00`,
        count: hour === 0 ? 2 : 0,
        successCount: 0,
        failedCount: 0,
        cancelledCount: 0,
        expiredCount: hour === 0 ? 1 : 0,
        pendingCount: hour === 0 ? 1 : 0,
        unknownCount: 0,
      })),
      completed: Array.from({ length: 24 }, (_, hour) => ({
        hour: `${String(hour).padStart(2, "0")}:00`,
        count: hour === 12 ? 1 : 0,
        totalAmount: hour === 12 ? 700000 : 0,
        totalFee: hour === 12 ? 700 : 0,
      })),
    },
    transactions: [
      reportItem({ createdAt: from }),
      reportItem({
        transactionId: "tx-zero",
        createdAt: from,
        effectiveStatus: "expired",
        selectedAssessment: risk,
        latestAssessment: risk,
        assessmentSource: "observed",
      }),
    ],
    topRiskSources: [],
    topRiskReceivers: [],
    dataQuality: [],
  };
}
