import type {
  TransactionReport,
  TransactionReportItem,
} from "./api/transactionsReportApi";
import type { DecisionFilter, StatusFilter } from "./reportView";

export interface ExportFilters {
  status: StatusFilter;
  decision: DecisionFilter;
  query: string;
}

interface CsvCell {
  value: unknown;
  text?: boolean;
}

const statusLabels: Record<string, string> = {
  pending: "Chờ xử lý",
  success: "Hoàn tất",
  failed: "Bị từ chối",
  cancelled: "Đã huỷ",
  expired: "Hết hạn OTP",
  unknown: "Chưa xác định",
};

const decisionLabels: Record<string, string> = {
  ALLOW: "Cho phép",
  BLOCK: "Chặn",
};

const filterLabels: Record<string, string> = {
  all: "Tất cả",
  unassessed: "Chưa có đánh giá",
  unknown: "Chưa rõ quyết định",
};

function safeText(value: unknown, text = false) {
  let result = String(value ?? "");
  if (text || /^[=+\-@]/.test(result)) result = `'${result}`;
  return result;
}

function csvCell({ value, text }: CsvCell) {
  return `"${safeText(value, text).replace(/"/g, '""')}"`;
}

function row(...cells: CsvCell[]) {
  return cells.map(csvCell).join(",");
}

const amount = (value: number | null | undefined): CsvCell => ({
  value: value ?? "",
});
const text = (value: unknown): CsvCell => ({ value });
const identifier = (value: unknown): CsvCell => ({ value, text: true });

function filterValue(value: string) {
  return (
    filterLabels[value] ?? statusLabels[value] ?? decisionLabels[value] ?? value
  );
}

function transactionRows(items: TransactionReportItem[]) {
  return [
    row(
      text("Transaction ID"),
      text("Source"),
      text("Destination"),
      text("Amount"),
      text("Fee"),
      text("Currency"),
      text("Status"),
      text("Risk decision"),
      text("Risk score"),
      text("Created At"),
      text("Completed At"),
    ),
    ...items.map((item) =>
      row(
        identifier(item.transactionId),
        identifier(item.sourceAccountNo),
        identifier(item.destinationAccountNo),
        amount(item.amount),
        amount(item.fee),
        text(item.currency),
        text(statusLabels[item.effectiveStatus] ?? item.effectiveStatus),
        text(
          item.selectedAssessment?.decision
            ? (decisionLabels[item.selectedAssessment.decision] ??
                item.selectedAssessment.decision)
            : "Chưa có đánh giá",
        ),
        amount(item.selectedAssessment?.riskScore ?? null),
        text(item.createdAt),
        text(item.completedAt),
      ),
    ),
  ];
}

export function buildTransactionReportCsv(
  report: TransactionReport,
  items: TransactionReportItem[],
  filters: ExportFilters,
) {
  const { created, completed, risk } = report.summary;
  const rows = [
    row(text("Báo cáo giao dịch & rủi ro"), text(report.date)),
    row(text("Report version"), amount(report.reportVersion)),
    row(text("Timezone"), text(report.timezone)),
    row(text("Currency"), text(report.currency)),
    row(text("As of"), text(report.asOf)),
    row(text("Generated at"), text(report.generatedAt)),
    row(text("Projection status"), text(report.projection.status)),
    row(
      text("Checkpoint revision"),
      text(report.projection.checkpointRevision),
    ),
    row(
      text("Summary scope"),
      text("Toàn ngày; không bị ảnh hưởng bởi bộ lọc bảng"),
    ),
    row(text("Created transaction count"), amount(created.transactionCount)),
    row(text("Created success count"), amount(created.successCount)),
    row(text("Created failed count"), amount(created.failedCount)),
    row(text("Created cancelled count"), amount(created.cancelledCount)),
    row(text("Created expired count"), amount(created.expiredCount)),
    row(text("Created pending count"), amount(created.pendingCount)),
    row(text("Created unknown count"), amount(created.unknownCount)),
    row(text("Completed count"), amount(completed.count)),
    row(text("Completed total amount"), amount(completed.totalAmount)),
    row(text("Completed total fee"), amount(completed.totalFee)),
    row(text("Completed average amount"), amount(completed.averageAmount)),
    row(text("Risk assessed count"), amount(risk.assessedCount)),
    row(text("Risk unassessed count"), amount(risk.unassessedCount)),
    row(text("Risk average score"), amount(risk.averageScore)),
    row(text("Risk coverage"), amount(risk.coverage)),
    row(text("Risk ALLOW count"), amount(risk.allowCount)),
    row(text("Risk BLOCK count"), amount(risk.modelBlockCount)),
    row(text("Risk unknown decision count"), amount(risk.unknownDecisionCount)),
    row(text("Actual blocked count"), amount(risk.actualBlockedCount)),
    row(
      text("Data quality warnings"),
      text(
        report.dataQuality
          .map((issue) => `${issue.code}: ${issue.count}`)
          .join("; "),
      ),
    ),
    [],
    row(
      text("Filtered list scope"),
      text("Toàn bộ dòng khớp bộ lọc; không chỉ trang đang xem"),
    ),
    row(text("Status filter"), text(filterValue(filters.status))),
    row(text("Risk filter"), text(filterValue(filters.decision))),
    row(text("Search"), text(filters.query)),
    ...transactionRows(items),
  ];
  return `\ufeff${rows.join("\n")}`;
}

export function downloadTransactionReportCsv(
  date: string,
  report: TransactionReport,
  items: TransactionReportItem[],
  filters: ExportFilters,
) {
  const url = URL.createObjectURL(
    new Blob([buildTransactionReportCsv(report, items, filters)], {
      type: "text/csv;charset=utf-8",
    }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `bao-cao-giao-dich-${date}.csv`;
  anchor.click();
  URL.revokeObjectURL(url);
}
