import type {
  ReportAssessment,
  TransactionReport,
} from "./transactionsReportApi";

type RecordValue = Record<string, unknown>;
type Check = (value: unknown) => boolean;

const object = (value: unknown): value is RecordValue =>
  typeof value === "object" && value !== null && !Array.isArray(value);
const string: Check = (value) => typeof value === "string";
const nonempty: Check = (value) =>
  typeof value === "string" && value.trim() !== "";
const count: Check = (value) =>
  typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const amount: Check = (value) => count(value) && (value as number) > 0;
const decimal: Check = (value) =>
  typeof value === "number" &&
  Number.isFinite(value) &&
  value >= 0 &&
  value <= Number.MAX_SAFE_INTEGER;
const score: Check = (value) => decimal(value) && (value as number) <= 1;
const nullable =
  (check: Check): Check =>
  (value) =>
    value === null || check(value);
const oneOf =
  (...values: string[]): Check =>
  (value) =>
    typeof value === "string" && values.includes(value);
const array =
  (check: Check): Check =>
  (value) =>
    Array.isArray(value) && value.every(check);
const fields = (
  value: unknown,
  checks: Record<string, Check>,
): value is RecordValue =>
  object(value) &&
  Object.entries(checks).every(([key, check]) => check(value[key]));

export function isReportDate(value: unknown): value is string {
  if (typeof value !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value))
    return false;
  const date = new Date(`${value}T00:00:00Z`);
  return (
    Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value
  );
}

const timestamp: Check = (value) =>
  typeof value === "string" &&
  /^\d{4}-\d{2}-\d{2}T(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d+)?Z$/.test(
    value,
  ) &&
  isReportDate(value.slice(0, 10)) &&
  Number.isFinite(Date.parse(value));

const statusCounts = {
  successCount: count,
  failedCount: count,
  cancelledCount: count,
  expiredCount: count,
  pendingCount: count,
  unknownCount: count,
};

const summary: Check = (value) =>
  fields(value, {
    created: (created) =>
      fields(created, { ...statusCounts, transactionCount: count }),
    completed: (completed) =>
      fields(completed, {
        count,
        totalAmount: count,
        totalFee: count,
        averageAmount: nullable(decimal),
      }),
    risk: (risk) =>
      fields(risk, {
        assessedCount: count,
        unassessedCount: count,
        averageScore: nullable(score),
        coverage: nullable(score),
        allowCount: count,
        modelBlockCount: count,
        unknownDecisionCount: count,
        actualBlockedCount: count,
      }),
  });

const assessment: Check = (value) =>
  fields(value, {
    assessmentId: nullable(nonempty),
    decision: nullable(oneOf("ALLOW", "BLOCK")),
    riskScore: score,
    threshold: nullable(score),
    modelVersion: nonempty,
    featureVersion: nullable(nonempty),
    evaluatedAt: timestamp,
    receivedAt: nullable(timestamp),
  });

const assessmentFields: (keyof ReportAssessment)[] = [
  "assessmentId",
  "decision",
  "riskScore",
  "threshold",
  "modelVersion",
  "featureVersion",
  "evaluatedAt",
  "receivedAt",
];

const item: Check = (value) => {
  if (
    !fields(value, {
      transactionId: nonempty,
      sourceAccountNo: string,
      sourceAccountName: string,
      destinationAccountNo: string,
      destinationName: string,
      amount: nullable(amount),
      fee: nullable(count),
      currency: string,
      canonicalStatus: string,
      effectiveStatus: oneOf(
        "success",
        "failed",
        "cancelled",
        "expired",
        "pending",
        "unknown",
      ),
      statusAt: nullable(timestamp),
      failureReason: nullable(string),
      createdAt: timestamp,
      completedAt: nullable(timestamp),
      completionTimeSource: nullable(oneOf("completed_at", "posted_at")),
      otpExpiresAt: nullable(timestamp),
      appliedRisk: nullable(assessment),
      latestAssessment: nullable(assessment),
      selectedAssessment: nullable(assessment),
      assessmentSource: oneOf("applied", "observed", "none"),
    })
  )
    return false;
  if (value.assessmentSource === "none")
    return value.selectedAssessment === null;
  const selected = value.selectedAssessment;
  const candidate =
    value.assessmentSource === "applied"
      ? value.appliedRisk
      : value.latestAssessment;
  if (value.assessmentSource === "observed" && value.appliedRisk !== null)
    return false;
  return (
    object(selected) &&
    object(candidate) &&
    assessmentFields.every((key) => selected[key] === candidate[key])
  );
};

const hour: Check = (value) =>
  typeof value === "string" && /^(?:[01]\d|2[0-3]):00$/.test(value);
const hours =
  (check: Check): Check =>
  (value) =>
    Array.isArray(value) &&
    value.length === 24 &&
    value.every(
      (point, index) =>
        check(point) && point.hour === `${String(index).padStart(2, "0")}:00`,
    );
const account: Check = (value) =>
  fields(value, { accountNo: nonempty, name: string, count, amount: count });

// Validate the wire shape before React formats dates, indexes collections, or
// exports values. Statistics themselves remain server-owned; do not recompute
// them from a filtered page or replace invalid/missing fields with zero.
export function parseTransactionReport(
  body: unknown,
  date: string,
): TransactionReport {
  if (
    !isReportDate(date) ||
    !object(body) ||
    !fields(body.data, {
      reportVersion: (value) => value === 2,
      date: (value) => value === date,
      previousDate: isReportDate,
      timezone: (value) => value === "Asia/Ho_Chi_Minh",
      currency: (value) => value === "VND",
      from: timestamp,
      to: timestamp,
      asOf: timestamp,
      generatedAt: timestamp,
      projection: (value) =>
        fields(value, {
          status: oneOf("caught_up", "lagging", "unknown"),
          checkpointRevision: nullable(
            (revision) =>
              typeof revision === "string" && /^\d+$/.test(revision),
          ),
        }),
      summary,
      previous: summary,
      hourly: (value) =>
        fields(value, {
          created: hours((point) =>
            fields(point, { hour, count, ...statusCounts }),
          ),
          completed: hours((point) =>
            fields(point, { hour, count, totalAmount: count, totalFee: count }),
          ),
        }),
      transactions: array(item),
      topRiskSources: array(account),
      topRiskReceivers: array(account),
      dataQuality: array((value) =>
        fields(value, {
          code: nonempty,
          scope: oneOf("created", "completed", "global"),
          date: nullable(isReportDate),
          count,
        }),
      ),
    })
  ) {
    throw new Error("Dữ liệu báo cáo không đúng cấu trúc hoặc ngày được chọn.");
  }
  return body.data as unknown as TransactionReport;
}
