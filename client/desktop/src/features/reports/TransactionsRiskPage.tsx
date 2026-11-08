import { useMemo, useRef, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowTrendUp,
  faArrowsRotate,
  faCalendarDays,
  faCircleCheck,
  faCircleExclamation,
  faCircleXmark,
  faDownload,
  faMagnifyingGlass,
  faMoneyBillTransfer,
  faShieldHalved,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { DailyReportChart } from "./DailyReportChart";
import type {
  TransactionReport,
  TransactionReportItem,
} from "./api/transactionsReportApi";
import { useTransactionReport } from "./useTransactionReport";
import {
  filterReportItems,
  formatReportTimestamp as formatDate,
  type StatusFilter,
  type DecisionFilter,
} from "./reportView";

const surface = "bg-(--admin-surface)";
const subtle = "bg-(--admin-surface-subtle)";
const border = "border-(--admin-border)";
const mono = "font-(family-name:--admin-font-mono)";
const PAGE_SIZE = 10;

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

function todayInVietnam() {
  return new Intl.DateTimeFormat("en-CA", {
    timeZone: "Asia/Ho_Chi_Minh",
  }).format(new Date());
}

function money(value?: number | null, currency = "VND") {
  return value == null ? "—" : `${value.toLocaleString("vi-VN")} ${currency}`;
}

function statusLabel(value?: string) {
  return value ? statusLabels[value] || value : "Chưa xác định";
}

function statusTone(value?: string) {
  if (value === "success") return "text-[#75d9a4]";
  if (value === "failed" || value === "cancelled")
    return "text-(--admin-danger)";
  return "text-(--admin-warning)";
}

function riskTone(value?: string | null) {
  if (!value) return "text-(--admin-text-muted)";
  return value === "BLOCK" ? "text-(--admin-danger)" : "text-(--admin-primary)";
}

function percentChange(current: number, previous: number) {
  if (previous === 0) return current === 0 ? "Không đổi" : "Mới phát sinh";
  const change = ((current - previous) / previous) * 100;
  return `${change >= 0 ? "+" : ""}${change.toFixed(1)}%`;
}

function share(value: number, total: number) {
  return total > 0
    ? `${((value / total) * 100).toFixed(1)}% tổng giao dịch`
    : "0% tổng giao dịch";
}

function assessmentLabel(item: TransactionReportItem) {
  const assessment = item.selectedAssessment;
  if (!assessment) return "Chưa có đánh giá";
  return assessment.decision
    ? decisionLabels[assessment.decision]
    : "Chưa rõ quyết định";
}

function scoreLabel(value?: number | null, digits = 1) {
  return value == null ? "—" : `${(value * 100).toFixed(digits)}%`;
}

function downloadReportCsv(
  date: string,
  report: TransactionReport | null,
  items: TransactionReportItem[],
) {
  if (!report) return;
  const rows = [
    ["Báo cáo ngày", date],
    ["Thời điểm tham chiếu", report.asOf],
    ["Múi giờ", report.timezone],
    ["Phạm vi tổng hợp", "Toàn ngày; danh sách theo bộ lọc hiện tại"],
    ["Giao dịch khởi tạo", report.summary.created.transactionCount],
    ["Giao dịch hoàn tất trong ngày", report.summary.completed.count],
    ["Doanh số thực hiện", report.summary.completed.totalAmount],
    ["Phí thực thu", report.summary.completed.totalFee],
    [
      "Giá trị trung bình hoàn tất",
      report.summary.completed.averageAmount ?? "",
    ],
    ["Thành công trong nhóm khởi tạo", report.summary.created.successCount],
    ["Thất bại trong nhóm khởi tạo", report.summary.created.failedCount],
    ["Thực tế bị chặn", report.summary.risk.actualBlockedCount],
    [
      "Cảnh báo dữ liệu",
      report.dataQuality
        .map((issue) => `${issue.code}: ${issue.count}`)
        .join("; "),
    ],
    [],
    [
      "Transaction ID",
      "Source",
      "Destination",
      "Amount",
      "Status",
      "Risk",
      "Created At",
    ],
    ...items.map((item) => [
      item.transactionId,
      item.sourceAccountNo,
      item.destinationAccountNo,
      item.amount ?? "",
      item.effectiveStatus,
      item.selectedAssessment?.decision ?? "",
      item.createdAt,
    ]),
  ];
  const csv =
    "\ufeff" +
    rows
      .map((row) =>
        row.map((value) => `"${String(value).replace(/"/g, '""')}"`).join(","),
      )
      .join("\n");
  const url = URL.createObjectURL(
    new Blob([csv], { type: "text/csv;charset=utf-8" }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `bao-cao-giao-dich-${date}.csv`;
  anchor.click();
  URL.revokeObjectURL(url);
}

function downloadReportExcel(
  date: string,
  report: TransactionReport | null,
  items: TransactionReportItem[],
) {
  if (!report) return;
  const escapeHtml = (value: unknown) =>
    String(value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  const rows = items
    .map(
      (item) =>
        `<tr><td>${escapeHtml(item.transactionId)}</td><td>${escapeHtml(item.sourceAccountNo)}</td><td>${escapeHtml(item.destinationAccountNo)}</td><td>${escapeHtml(item.amount ?? "")}</td><td>${escapeHtml(item.effectiveStatus)}</td><td>${escapeHtml(item.selectedAssessment?.decision ?? "")}</td><td>${escapeHtml(item.createdAt)}</td></tr>`,
    )
    .join("");
  const html = `<html><head><meta charset="utf-8"><style>body{font-family:Arial}table{border-collapse:collapse}th,td{border:1px solid #cbd5e1;padding:8px;min-width:140px}th{background:#1f4e78;color:white}h1{font-size:18px}</style></head><body><h1>Báo cáo giao dịch ngày ${escapeHtml(date)}</h1><p>Tham chiếu: ${escapeHtml(report.asOf)} | Múi giờ: ${escapeHtml(report.timezone)} | Tổng hợp toàn ngày; danh sách theo bộ lọc hiện tại</p><p>Doanh số thực hiện: ${escapeHtml(report.summary.completed.totalAmount)} | Phí thực thu: ${escapeHtml(report.summary.completed.totalFee)} | Hoàn tất trong ngày: ${escapeHtml(report.summary.completed.count)} | Thất bại trong nhóm khởi tạo: ${escapeHtml(report.summary.created.failedCount)} | Thực tế bị chặn: ${escapeHtml(report.summary.risk.actualBlockedCount)}</p><p>Cảnh báo dữ liệu: ${escapeHtml(report.dataQuality.map((issue) => `${issue.code}: ${issue.count}`).join("; "))}</p><table><thead><tr><th>Transaction ID</th><th>Source</th><th>Destination</th><th>Amount</th><th>Status</th><th>Risk</th><th>Created At</th></tr></thead><tbody>${rows}</tbody></table></body></html>`;
  const url = URL.createObjectURL(
    new Blob([html], { type: "application/vnd.ms-excel" }),
  );
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `bao-cao-giao-dich-${date}.xls`;
  anchor.click();
  URL.revokeObjectURL(url);
}

function ReportDateField({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const openPicker = () => {
    const input = inputRef.current;
    if (!input) return;
    if (typeof input.showPicker === "function") input.showPicker();
    else input.focus();
  };
  return (
    <div className="flex h-9 w-36 items-center rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 focus-within:border-(--admin-primary)">
      <input
        ref={inputRef}
        className="min-w-0 flex-1 appearance-none bg-transparent px-0 text-[12px] text-(--admin-text) outline-none"
        type="date"
        aria-label="Ngày báo cáo"
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
      <button
        className="grid h-6 w-6 shrink-0 place-items-center rounded-sm text-(--admin-text-muted) hover:bg-(--admin-primary-soft) hover:text-(--admin-primary)"
        type="button"
        aria-label="Chọn ngày báo cáo"
        onClick={openPicker}
      >
        <FontAwesomeIcon
          icon={faCalendarDays}
          className="text-[12px]"
          aria-hidden="true"
        />
      </button>
    </div>
  );
}

function MetricCard({
  label,
  value,
  note,
  icon,
  tone = "default",
}: {
  label: string;
  value: string | number;
  note: string;
  icon: typeof faShieldHalved;
  tone?: "default" | "danger" | "primary";
}) {
  const color =
    tone === "danger"
      ? "text-(--admin-danger)"
      : tone === "primary"
        ? "text-(--admin-primary)"
        : "text-(--admin-text)";
  return (
    <article
      className={`${surface} rounded-(--admin-radius-md) border ${border} p-3.5`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-semibold text-(--admin-text-muted)">
          {label}
        </span>
        <span className="grid h-7 w-7 place-items-center rounded-sm bg-(--admin-surface-subtle) text-[12px] text-(--admin-text-muted)">
          <FontAwesomeIcon icon={icon} />
        </span>
      </div>
      <strong
        className={`${mono} mt-3 block text-[23px] leading-6 font-bold ${color}`}
      >
        {value}
      </strong>
      <p className="mt-1 text-[10px] text-(--admin-text-muted)">{note}</p>
    </article>
  );
}

export function TransactionsRiskPage() {
  const [date, setDate] = useState(todayInVietnam);
  const [status, setStatus] = useState<StatusFilter>("all");
  const [decision, setDecision] = useState<DecisionFilter>("all");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const {
    report: currentReport,
    pending: refreshing,
    stale,
    error,
    refresh: load,
    canExport,
  } = useTransactionReport(date);
  const selected = currentReport?.transactions.find(
    (item) => item.transactionId === selectedId,
  );

  const items = useMemo(
    () =>
      filterReportItems(
        currentReport?.transactions ?? [],
        status,
        decision,
        query,
      ),
    [currentReport, status, decision, query],
  );

  const summary = currentReport?.summary;
  const previous = currentReport?.previous;
  const totalPages = Math.max(1, Math.ceil(items.length / PAGE_SIZE));
  const currentPage = Math.min(page, totalPages);
  const visibleItems = items.slice(
    (currentPage - 1) * PAGE_SIZE,
    currentPage * PAGE_SIZE,
  );

  return (
    <div className="grid gap-4" aria-label="Giao dịch và rủi ro">
      <header
        className={`${surface} rounded-(--admin-radius-md) border ${border} p-4`}
      >
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
          <div className="flex items-start gap-3">
            <span className="grid h-10 w-10 shrink-0 place-items-center rounded-(--admin-radius-md) bg-(--admin-primary-soft) text-(--admin-primary)">
              <FontAwesomeIcon icon={faShieldHalved} />
            </span>
            <div>
              <p
                className={`${mono} text-[10px] font-semibold uppercase tracking-[0.08em] text-(--admin-primary)`}
              >
                BÁO CÁO VẬN HÀNH
              </p>
              <h1 className="mt-1 text-[21px] font-bold tracking-[-0.03em] text-(--admin-text)">
                Giao dịch &amp; Rủi ro
              </h1>
              <p className="mt-1 text-[12px] text-(--admin-text-muted)">
                Tổng hợp hoạt động trong ngày và kết quả phát hiện bất thường.
              </p>
            </div>
          </div>
          <div className="flex items-end gap-2">
            <div>
              <p
                className={`${mono} mb-1 text-[10px] uppercase text-(--admin-text-muted)`}
              >
                Ngày báo cáo
              </p>
              <div className="flex items-center gap-1.5">
                <ReportDateField
                  value={date}
                  onChange={(value) => {
                    setPage(1);
                    setDate(value);
                    setSelectedId(null);
                  }}
                />
                <button
                  type="button"
                  className={`${surface} h-9 rounded-sm border ${border} px-2.5 text-[11px] font-semibold text-(--admin-text) hover:bg-(--admin-surface-hover)`}
                  onClick={() => {
                    setPage(1);
                    setDate(todayInVietnam());
                    setSelectedId(null);
                  }}
                  disabled={date === todayInVietnam()}
                >
                  Hôm nay
                </button>
              </div>
            </div>
            <button
              type="button"
              className={`${surface} grid h-9 w-9 place-items-center rounded-sm border ${border} text-(--admin-text-muted) hover:bg-(--admin-surface-hover) hover:text-(--admin-text)`}
              onClick={() => void load()}
              disabled={refreshing}
              aria-label="Làm mới báo cáo"
            >
              <FontAwesomeIcon
                icon={faArrowsRotate}
                className={refreshing ? "animate-spin" : ""}
              />
            </button>
          </div>
        </div>
      </header>

      {error && (
        <p
          role="alert"
          className="rounded-sm border border-(--admin-danger) bg-(--admin-danger-soft) p-3 text-[12px] text-(--admin-danger)"
        >
          {error}
        </p>
      )}

      {refreshing && (
        <p role="status" className="text-[12px] text-(--admin-text-muted)">
          {currentReport ? "Đang cập nhật báo cáo..." : "Đang tải báo cáo..."}
        </p>
      )}
      {!currentReport && !refreshing && (
        <p role="status" className="text-[12px] text-(--admin-text-muted)">
          Chưa có báo cáo cho ngày được chọn. Chọn ngày hợp lệ hoặc làm mới để
          thử lại.
        </p>
      )}

      {currentReport && (
        <>
          <p className="text-[11px] text-(--admin-text-muted)">
            Tổng hợp toàn ngày; bộ lọc chỉ áp dụng cho danh sách. Tham chiếu:{" "}
            {formatDate(currentReport?.asOf)} (giờ Việt Nam).
            {currentReport?.projection.status === "unknown" &&
              " Chưa xác định độ trễ projection."}
            {currentReport?.projection.status === "lagging" &&
              " Projection đang chậm so với dữ liệu gốc."}
            {date === todayInVietnam() &&
              " Ngày chưa kết thúc; so sánh với toàn ngày liền trước."}
            {stale && " Dữ liệu đang hiển thị là bản tải trước đó."}
          </p>
          {!!currentReport?.dataQuality.length && (
            <p role="alert" className="text-[12px] text-(--admin-warning)">
              Báo cáo có cảnh báo chất lượng dữ liệu:{" "}
              {currentReport.dataQuality
                .map(
                  (issue) =>
                    `${issue.code} (${issue.count}; ${issue.date ?? "toàn lịch sử"}; ${issue.scope})`,
                )
                .join(", ")}
              .
            </p>
          )}

          <section className="grid grid-cols-2 gap-3 md:grid-cols-4">
            <MetricCard
              label="Khởi tạo trong ngày"
              value={summary?.created.transactionCount ?? "—"}
              note={`So với ngày liền trước (${currentReport.previousDate}): ${percentChange(summary?.created.transactionCount || 0, previous?.created.transactionCount || 0)}`}
              icon={faShieldHalved}
            />
            <MetricCard
              label="Doanh số thực hiện"
              value={money(summary?.completed.totalAmount)}
              note={`Theo ngày hoàn tất · ${percentChange(summary?.completed.totalAmount || 0, previous?.completed.totalAmount || 0)} so với ${currentReport.previousDate}`}
              icon={faMoneyBillTransfer}
              tone="primary"
            />
            <MetricCard
              label="Phí thực thu"
              value={money(summary?.completed.totalFee)}
              note="Phí của giao dịch thành công"
              icon={faCircleCheck}
            />
            <MetricCard
              label="Giá trị trung bình"
              value={money(summary?.completed.averageAmount)}
              note="Trên giao dịch thành công"
              icon={faArrowTrendUp}
            />
            <MetricCard
              label="Hoàn tất trong ngày"
              value={summary?.completed.count ?? "—"}
              note="Theo thời điểm hoàn tất, không phải khởi tạo"
              icon={faCircleCheck}
              tone="primary"
            />
            <MetricCard
              label="Thất bại (nhóm khởi tạo)"
              value={summary?.created.failedCount ?? "—"}
              note={share(
                summary?.created.failedCount || 0,
                summary?.created.transactionCount || 0,
              )}
              icon={faCircleXmark}
              tone="danger"
            />
            <MetricCard
              label="Thực tế bị chặn"
              value={summary?.risk.actualBlockedCount ?? "—"}
              note={share(
                summary?.risk.actualBlockedCount || 0,
                summary?.created.transactionCount || 0,
              )}
              icon={faCircleExclamation}
              tone="danger"
            />
            <MetricCard
              label="Risk trung bình"
              value={scoreLabel(summary?.risk.averageScore)}
              note={`Đã đánh giá ${summary?.risk.assessedCount ?? 0}/${summary?.created.transactionCount ?? 0} giao dịch khởi tạo`}
              icon={faArrowTrendUp}
            />
          </section>

          <DailyReportChart
            hourly={currentReport?.hourly || { created: [], completed: [] }}
            date={date}
            summary={summary}
          />

          <section className="grid gap-3 lg:grid-cols-2">
            {[
              [
                "Nguồn có nhiều giao dịch BLOCK",
                currentReport?.topRiskSources || [],
              ],
              [
                "Điểm nhận có nhiều giao dịch BLOCK",
                currentReport?.topRiskReceivers || [],
              ],
            ].map(([title, values]) => (
              <article
                key={title as string}
                className={`${surface} rounded-(--admin-radius-md) border ${border} p-4`}
              >
                <h2 className="text-[14px] font-bold text-(--admin-text)">
                  {title as string}
                </h2>
                <div className="mt-3 grid gap-2">
                  {(values as TransactionReport["topRiskSources"]).map(
                    (item) => (
                      <div
                        key={item.accountNo}
                        className="flex items-center justify-between gap-3 rounded-sm bg-(--admin-surface-subtle) px-3 py-2 text-[11px]"
                      >
                        <div>
                          <p
                            className={`${mono} font-semibold text-(--admin-text)`}
                          >
                            {item.accountNo}
                          </p>
                          <p className="text-(--admin-text-muted)">
                            {item.name || "Chưa có tên"}
                          </p>
                        </div>
                        <div className="text-right">
                          <p className="font-semibold text-(--admin-danger)">
                            {item.count} giao dịch BLOCK
                          </p>
                          <p
                            className={`${mono} text-[10px] text-(--admin-text-muted)`}
                          >
                            {money(item.amount)}
                          </p>
                        </div>
                      </div>
                    ),
                  )}
                  {!(values as TransactionReport["topRiskSources"]).length && (
                    <p className="text-[11px] text-(--admin-text-muted)">
                      Chưa có dữ liệu rủi ro.
                    </p>
                  )}
                </div>
              </article>
            ))}
          </section>
        </>
      )}
      <section
        className={`${surface} overflow-hidden rounded-(--admin-radius-md) border ${border}`}
      >
        <div className="border-b border-(--admin-border) p-4">
          <div className="flex flex-col justify-between gap-3 lg:flex-row lg:items-center">
            <div>
              <h2 className="text-[15px] font-bold text-(--admin-text)">
                Danh sách giao dịch
              </h2>
              <p className="mt-1 text-[11px] text-(--admin-text-muted)">
                Giao dịch khởi tạo trong ngày được chọn; trạng thái tại thời
                điểm tham chiếu.
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <div
                className={`flex h-8 min-w-55 items-center gap-2 rounded-sm border ${border} bg-(--admin-background) px-2.5`}
              >
                <FontAwesomeIcon
                  icon={faMagnifyingGlass}
                  className="text-[11px] text-(--admin-text-muted)"
                />
                <input
                  className="min-w-0 flex-1 bg-transparent text-[12px] text-(--admin-text) outline-none"
                  placeholder="Tìm mã hoặc tài khoản"
                  value={query}
                  onChange={(event) => {
                    setPage(1);
                    setQuery(event.target.value);
                  }}
                />
              </div>
              <select
                className={`${subtle} h-8 rounded-sm border ${border} px-2 text-[11px] text-(--admin-text)`}
                value={status}
                onChange={(event) => {
                  setPage(1);
                  setStatus(event.target.value as StatusFilter);
                }}
                aria-label="Trạng thái"
              >
                <option value="all">Tất cả trạng thái</option>
                <option value="pending">Chờ xử lý</option>
                <option value="success">Hoàn tất</option>
                <option value="failed">Bị từ chối</option>
                <option value="cancelled">Đã huỷ</option>
                <option value="expired">Hết hạn OTP</option>
                <option value="unknown">Chưa xác định</option>
              </select>
              <select
                className={`${subtle} h-8 rounded-sm border ${border} px-2 text-[11px] text-(--admin-text)`}
                value={decision}
                onChange={(event) => {
                  setPage(1);
                  setDecision(event.target.value as DecisionFilter);
                }}
                aria-label="Kết quả rủi ro"
              >
                <option value="all">Tất cả risk</option>
                <option value="ALLOW">Cho phép</option>
                <option value="BLOCK">Chặn</option>
                <option value="unassessed">Chưa có đánh giá</option>
                <option value="unknown">Chưa rõ quyết định</option>
              </select>
              <button
                type="button"
                className={`${subtle} flex h-8 items-center gap-1.5 rounded-sm border ${border} px-2 text-[11px] font-semibold text-(--admin-text)`}
                onClick={() => {
                  if (canExport && currentReport)
                    downloadReportCsv(currentReport.date, currentReport, items);
                }}
                disabled={!canExport}
              >
                <FontAwesomeIcon icon={faDownload} /> CSV
              </button>
              <button
                type="button"
                className={`${subtle} flex h-8 items-center gap-1.5 rounded-sm border ${border} px-2 text-[11px] font-semibold text-(--admin-text)`}
                onClick={() => {
                  if (canExport && currentReport)
                    downloadReportExcel(
                      currentReport.date,
                      currentReport,
                      items,
                    );
                }}
                disabled={!canExport}
              >
                <FontAwesomeIcon icon={faDownload} /> Excel
              </button>
            </div>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full min-w-245 text-left text-[12px]">
            <thead className="border-b border-(--admin-border) text-[10px] uppercase tracking-[0.04em] text-(--admin-text-muted)">
              <tr>
                <th className="px-4 py-3 font-semibold">Giao dịch</th>
                <th className="px-4 py-3 font-semibold">Từ / đến</th>
                <th className="px-4 py-3 font-semibold">Số tiền</th>
                <th className="px-4 py-3 font-semibold">Trạng thái</th>
                <th className="px-4 py-3 font-semibold">Risk</th>
                <th className="px-4 py-3 font-semibold">Khởi tạo</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--admin-border)">
              {visibleItems.map((item) => {
                const tone = statusTone(item.effectiveStatus);
                const risk = riskTone(item.selectedAssessment?.decision);
                return (
                  <tr
                    key={item.transactionId}
                    className="cursor-pointer transition-colors hover:bg-(--admin-surface-hover)"
                    onClick={() => setSelectedId(item.transactionId)}
                  >
                    <td className="px-4 py-3">
                      <span
                        className={`${mono} text-[11px] font-semibold text-(--admin-text)`}
                      >
                        {item.transactionId.slice(0, 12)}…
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className="text-(--admin-text)">
                        {item.sourceAccountNo || "—"}
                      </span>
                      <div className="mt-1 text-[11px] text-(--admin-text-muted)">
                        → {item.destinationAccountNo || "—"}
                      </div>
                    </td>
                    <td
                      className={`${mono} px-4 py-3 font-semibold text-(--admin-text)`}
                    >
                      {money(item.amount, item.currency)}
                    </td>
                    <td className={`px-4 py-3 font-semibold ${tone}`}>
                      {statusLabel(item.effectiveStatus)}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`font-semibold ${risk}`}>
                        {assessmentLabel(item)}
                      </span>
                      <div
                        className={`${mono} mt-1 text-[10px] text-(--admin-text-muted)`}
                      >
                        {scoreLabel(item.selectedAssessment?.riskScore)}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-[11px] text-(--admin-text-muted)">
                      {formatDate(item.createdAt)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        {currentReport && !items.length && (
          <div className="p-10 text-center">
            <FontAwesomeIcon
              icon={faShieldHalved}
              className="text-2xl text-(--admin-text-muted)"
            />
            <p className="mt-2 text-[12px] font-semibold text-(--admin-text)">
              Không có dữ liệu phù hợp
            </p>
            <p className="mt-1 text-[11px] text-(--admin-text-muted)">
              Thử đổi ngày hoặc bộ lọc.
            </p>
          </div>
        )}
        <div className="flex items-center justify-center gap-3 border-t border-(--admin-border) pt-3">
          <button
            type="button"
            className={`${surface} h-9 rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`}
            onClick={() => setPage((value) => Math.max(1, value - 1))}
            disabled={currentPage === 1}
          >
            ← Trang trước
          </button>
          <span
            className={`${mono} min-w-18 text-center text-[11px] text-(--admin-text-muted)`}
          >
            Trang {currentPage}
          </span>
          <button
            type="button"
            className={`${surface} h-9 rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`}
            onClick={() => setPage((value) => Math.min(totalPages, value + 1))}
            disabled={currentPage === totalPages}
          >
            Trang sau →
          </button>
        </div>
      </section>

      {selected && (
        <>
          <button
            className="fixed inset-0 z-30 cursor-default bg-black/40"
            onClick={() => setSelectedId(null)}
            aria-label="Đóng chi tiết"
          />
          <aside
            className="fixed inset-y-0 right-0 z-40 w-full max-w-105 overflow-y-auto border-l border-(--admin-border) bg-(--admin-surface) p-5 shadow-2xl"
            aria-label="Chi tiết giao dịch"
          >
            <div className="flex items-start justify-between border-b border-(--admin-border) pb-4">
              <div>
                <p
                  className={`${mono} text-[10px] font-semibold uppercase tracking-[0.08em] text-(--admin-primary)`}
                >
                  Chi tiết transaction
                </p>
                <h2
                  className={`${mono} mt-1 text-[15px] font-bold text-(--admin-text)`}
                >
                  {selected.transactionId}
                </h2>
              </div>
              <button
                type="button"
                className="grid h-8 w-8 place-items-center rounded-sm text-(--admin-text-muted) hover:bg-(--admin-surface-hover) hover:text-(--admin-text)"
                onClick={() => setSelectedId(null)}
                aria-label="Đóng"
              >
                <FontAwesomeIcon icon={faXmark} />
              </button>
            </div>
            <div className="grid gap-3 py-4 text-[12px]">
              <div className={`${subtle} rounded-(--admin-radius-md) p-3`}>
                <p className="font-semibold text-(--admin-text)">
                  Risk assessment
                </p>
                <div className="mt-3 grid grid-cols-2 gap-3">
                  <div>
                    <p className="text-[10px] text-(--admin-text-muted)">
                      Quyết định
                    </p>
                    <strong
                      className={riskTone(
                        selected.selectedAssessment?.decision,
                      )}
                    >
                      {assessmentLabel(selected)}
                    </strong>
                  </div>
                  <div>
                    <p className="text-[10px] text-(--admin-text-muted)">
                      Risk score
                    </p>
                    <strong className={mono}>
                      {scoreLabel(selected.selectedAssessment?.riskScore, 2)}
                    </strong>
                  </div>
                  <div>
                    <p className="text-[10px] text-(--admin-text-muted)">
                      Threshold
                    </p>
                    <strong className={mono}>
                      {scoreLabel(selected.selectedAssessment?.threshold, 2)}
                    </strong>
                  </div>
                  <div>
                    <p className="text-[10px] text-(--admin-text-muted)">
                      Model
                    </p>
                    <strong className="break-all text-[11px]">
                      {selected.selectedAssessment?.modelVersion ?? "—"}
                    </strong>
                  </div>
                </div>
                <p className="mt-3 text-[11px] text-(--admin-text-muted)">
                  {selected.assessmentSource === "applied"
                    ? "Quyết định đã áp dụng cho giao dịch"
                    : selected.assessmentSource === "observed"
                      ? "Assessment quan sát được; chưa phải bằng chứng đã chặn giao dịch"
                      : "Chưa có đánh giá"}
                </p>
              </div>
              <div className={`${subtle} rounded-(--admin-radius-md) p-3`}>
                <p className="font-semibold text-(--admin-text)">
                  Thông tin giao dịch
                </p>
                <dl className="mt-3 grid gap-2">
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">Từ</dt>
                    <dd className="text-right font-semibold">
                      {selected.sourceAccountNo || "—"}
                    </dd>
                  </div>
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">Đến</dt>
                    <dd className="text-right font-semibold">
                      {selected.destinationAccountNo || "—"}
                    </dd>
                  </div>
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">Số tiền</dt>
                    <dd className={`${mono} text-right font-semibold`}>
                      {money(selected.amount, selected.currency)}
                    </dd>
                  </div>
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">Trạng thái</dt>
                    <dd className={statusTone(selected.effectiveStatus)}>
                      {statusLabel(selected.effectiveStatus)}
                    </dd>
                  </div>
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">Khởi tạo</dt>
                    <dd className="text-right">
                      {formatDate(selected.createdAt)}
                    </dd>
                  </div>
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">Hoàn tất</dt>
                    <dd>
                      {formatDate(selected.completedAt)}
                      {selected.completionTimeSource === "posted_at" &&
                        " (mốc ghi sổ lịch sử)"}
                    </dd>
                  </div>
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">Đánh giá lúc</dt>
                    <dd>
                      {formatDate(selected.selectedAssessment?.evaluatedAt)}
                    </dd>
                  </div>
                  <div className="flex justify-between gap-3">
                    <dt className="text-(--admin-text-muted)">
                      Nhận assessment
                    </dt>
                    <dd>
                      {formatDate(selected.selectedAssessment?.receivedAt)}
                    </dd>
                  </div>
                </dl>
              </div>
            </div>
          </aside>
        </>
      )}
    </div>
  );
}
