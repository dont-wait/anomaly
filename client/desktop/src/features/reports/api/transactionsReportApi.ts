import { API_BASE_URL } from "@/features/auth/api/adminSession";
import {
  authHeaders,
  fetchWithTimeout,
} from "@/features/monitor/api/monitorApi";

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
  const response = await fetchWithTimeout(
    `${API_BASE_URL}/api/admin/reports/transactions?date=${encodeURIComponent(date)}`,
    { headers: authHeaders(), signal },
  );
  if (!response.ok)
    throw new Error("Không thể tải báo cáo doanh số giao dịch.");
  const body = (await response.json()) as { data?: TransactionReport };
  if (
    !body.data ||
    body.data.reportVersion !== 2 ||
    body.data.date !== date ||
    !body.data.summary?.created ||
    !body.data.summary?.completed ||
    !body.data.summary?.risk ||
    !Array.isArray(body.data.transactions) ||
    !Array.isArray(body.data.hourly?.created) ||
    !Array.isArray(body.data.hourly?.completed)
  ) {
    throw new Error("Dữ liệu báo cáo không đúng cấu trúc hoặc ngày được chọn.");
  }
  return body.data;
}
