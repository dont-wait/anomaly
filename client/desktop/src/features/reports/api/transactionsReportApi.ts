import { API_BASE_URL, readAdminSession } from "@/features/auth/api/adminSession";

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
  failed: number;
  pending: number;
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

export async function getTransactionReport(date: string): Promise<TransactionReport> {
  const session = readAdminSession();
  const response = await fetch(
    `${API_BASE_URL}/api/admin/reports/transactions?date=${encodeURIComponent(date)}`,
    { headers: session ? { Authorization: `Bearer ${session.accessToken}` } : {} },
  );
  if (!response.ok) throw new Error("Không thể tải báo cáo doanh số giao dịch.");
  const body = (await response.json()) as { data?: TransactionReport } | TransactionReport;
  if ("data" in body && body.data) return body.data;
  return body as TransactionReport;
}
