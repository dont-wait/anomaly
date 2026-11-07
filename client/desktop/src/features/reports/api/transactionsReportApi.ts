import { API_BASE_URL } from "@/features/auth/api/adminSession";
import {
  authHeaders,
  fetchWithTimeout,
} from "@/features/monitor/api/monitorApi";

export interface TransactionReportSummary {
  transactionCount: number;
  totalAmount: number;
  totalFee: number;
  averageAmount: number;
  successCount: number;
  failedCount: number;
  blockedCount: number;
}

export interface TransactionRiskAccount {
  accountNo: string;
  name: string;
  count: number;
  amount: number;
}

export interface TransactionReportHourly {
  hour: string;
  count: number;
  amount: number;
  success: number;
  successAmount: number;
  failed: number;
  failedAmount: number;
  pending: number;
  pendingAmount: number;
}

export interface TransactionReportItem {
  transactionId: string;
  sourceAccountNo: string;
  destinationAccountNo: string;
  amount: number;
  fee: number;
  currency: string;
  status: string;
  decision?: string;
  riskScore?: number;
  createdAt: string;
}

export interface TransactionReport {
  date: string;
  previousDate: string;
  summary: TransactionReportSummary;
  previous: TransactionReportSummary;
  hourly: TransactionReportHourly[];
  transactions: TransactionReportItem[];
  topRiskSources: TransactionRiskAccount[];
  topRiskReceivers: TransactionRiskAccount[];
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
  const body = (await response.json()) as
    { data?: TransactionReport } | TransactionReport;
  if ("data" in body && body.data) return body.data;
  return body as TransactionReport;
}
