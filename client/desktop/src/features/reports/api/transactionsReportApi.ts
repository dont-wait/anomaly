import { API_BASE_URL } from "@/features/auth/api/adminSession";
import { authHeaders } from "@/features/monitor/api/monitorApi";
import { isReportDate, parseTransactionReport } from "./reportValidation";

export interface TransactionStatusCounts {
  successCount: number;
  failedCount: number;
  cancelledCount: number;
  expiredCount: number;
  pendingCount: number;
  unknownCount: number;
}

export interface TransactionReportSummary {
  created: TransactionStatusCounts & { transactionCount: number };
  completed: {
    count: number;
    totalAmount: number;
    totalFee: number;
    averageAmount: number | null;
  };
  risk: {
    assessedCount: number;
    unassessedCount: number;
    averageScore: number | null;
    coverage: number | null;
    allowCount: number;
    modelBlockCount: number;
    unknownDecisionCount: number;
    actualBlockedCount: number;
  };
}

export interface TransactionRiskAccount {
  accountNo: string;
  name: string;
  count: number;
  amount: number;
}

export interface TransactionReportHourly {
  created: (TransactionStatusCounts & { hour: string; count: number })[];
  completed: {
    hour: string;
    count: number;
    totalAmount: number;
    totalFee: number;
  }[];
}

export interface ReportAssessment {
  assessmentId: string | null;
  decision: "ALLOW" | "BLOCK" | null;
  riskScore: number;
  threshold: number | null;
  modelVersion: string;
  featureVersion: string | null;
  evaluatedAt: string;
  receivedAt: string | null;
}

export interface TransactionReportItem {
  transactionId: string;
  sourceAccountNo: string;
  sourceAccountName: string;
  destinationAccountNo: string;
  destinationName: string;
  amount: number | null;
  fee: number | null;
  currency: string;
  canonicalStatus: string;
  effectiveStatus:
    "success" | "failed" | "cancelled" | "expired" | "pending" | "unknown";
  statusAt: string | null;
  failureReason: string | null;
  createdAt: string;
  completedAt: string | null;
  completionTimeSource: string | null;
  otpExpiresAt: string | null;
  appliedRisk: ReportAssessment | null;
  latestAssessment: ReportAssessment | null;
  selectedAssessment: ReportAssessment | null;
  assessmentSource: "applied" | "observed" | "none";
}

export interface TransactionReport {
  reportVersion: 2;
  date: string;
  previousDate: string;
  timezone: string;
  currency: string;
  from: string;
  to: string;
  asOf: string;
  generatedAt: string;
  projection: {
    status: "caught_up" | "lagging" | "unknown";
    checkpointRevision: string | null;
  };
  summary: TransactionReportSummary;
  previous: TransactionReportSummary;
  hourly: TransactionReportHourly;
  transactions: TransactionReportItem[];
  topRiskSources: TransactionRiskAccount[];
  topRiskReceivers: TransactionRiskAccount[];
  dataQuality: {
    code: string;
    scope: string;
    date: string | null;
    count: number;
  }[];
}

export async function getTransactionReport(
  date: string,
  signal?: AbortSignal,
): Promise<TransactionReport> {
  if (!isReportDate(date))
    throw new Error("Chọn ngày báo cáo hợp lệ (YYYY-MM-DD).");
  // Keep cancellation and the deadline active until JSON has been consumed,
  // not only until headers arrive. Leave the shared monitor transport unchanged.
  const controller = new AbortController();
  const abort = () => controller.abort();
  if (signal?.aborted) abort();
  else signal?.addEventListener("abort", abort, { once: true });
  let timedOut = false;
  const timeout = window.setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, 12000);
  try {
    const response = await fetch(
      `${API_BASE_URL}/api/admin/reports/transactions?date=${encodeURIComponent(date)}`,
      { headers: authHeaders(), signal: controller.signal },
    );
    if (!response.ok)
      throw new Error("Không thể tải báo cáo doanh số giao dịch.");
    let body: unknown;
    try {
      body = await response.json();
    } catch (cause) {
      if (controller.signal.aborted) throw cause;
      // Preserve the cause while keeping compatibility with the ES2020 target.
      throw Object.assign(
        new Error("Dữ liệu báo cáo không đúng cấu trúc hoặc ngày được chọn."),
        { cause },
      );
    }
    if (controller.signal.aborted)
      throw new DOMException("Request aborted", "AbortError");
    return parseTransactionReport(body, date);
  } catch (cause) {
    if (timedOut)
      throw Object.assign(
        new Error("Tải báo cáo quá thời gian chờ. Vui lòng thử lại."),
        { cause },
      );
    throw cause;
  } finally {
    window.clearTimeout(timeout);
    signal?.removeEventListener("abort", abort);
  }
}
