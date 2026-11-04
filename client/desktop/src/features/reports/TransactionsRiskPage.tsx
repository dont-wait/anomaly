import { useEffect, useMemo, useRef, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowTrendUp,
  faArrowsRotate,
  faCalendarDays,
  faCircleCheck,
  faCircleExclamation,
  faCircleXmark,
  faDownload,
  faFilePdf,
  faMagnifyingGlass,
  faMoneyBillTransfer,
  faShieldHalved,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { getDailyMonitorReport } from "@/features/monitor/api/monitorApi";
import type { MonitorDashboardResponse, RiskAssessment } from "@/features/monitor/model";
import { DailyReportChart } from "./DailyReportChart";
import { getTransactionReport, type TransactionReport, type TransactionReportItem } from "./api/transactionsReportApi";

const surface = "bg-(--admin-surface)";
const subtle = "bg-(--admin-surface-subtle)";
const border = "border-(--admin-border)";
const mono = "font-(family-name:--admin-font-mono)";
const PAGE_SIZE = 10;

type StatusFilter = "all" | "awaiting_otp" | "success" | "failed";
type DecisionFilter = "all" | "ALLOW" | "BLOCK";

const statusLabels: Record<string, string> = {
  awaiting_otp: "Chờ xử lý",
  success: "Hoàn tất",
  failed: "Bị từ chối",
  cancelled: "Đã huỷ",
};

const decisionLabels: Record<string, string> = { ALLOW: "Cho phép", BLOCK: "Chặn" };

function todayInVietnam() {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Ho_Chi_Minh" }).format(new Date());
}

function money(value?: number, currency = "VND") {
  return `${(value || 0).toLocaleString("vi-VN")} ${currency}`;
}

function formatDate(value?: string) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("vi-VN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function statusLabel(value?: string) {
  return value ? statusLabels[value] || value : "Chưa xác định";
}

function statusTone(value?: string) {
  if (value === "success") return "text-[#75d9a4]";
  if (value === "failed" || value === "cancelled") return "text-(--admin-danger)";
  return "text-(--admin-warning)";
}

function riskTone(value: string) {
  return value === "BLOCK" ? "text-(--admin-danger)" : "text-(--admin-primary)";
}

function percentChange(current: number, previous: number) {
  if (previous === 0) return current === 0 ? "Không đổi" : "Mới phát sinh";
  const change = ((current - previous) / previous) * 100;
  return `${change >= 0 ? "+" : ""}${change.toFixed(1)}% so với hôm qua`;
}

function share(value: number, total: number) {
  return total > 0 ? `${((value / total) * 100).toFixed(1)}% tổng giao dịch` : "0% tổng giao dịch";
}

function reportItemToAssessment(item: TransactionReportItem, existing?: RiskAssessment): RiskAssessment {
  if (existing) return existing;
  const blocked = item.decision === "BLOCK";
  return {
    assessmentId: item.transactionId,
    transactionId: item.transactionId,
    riskScore: item.riskScore || 0,
    predictedLabel: blocked ? 1 : 0,
    riskLevel: blocked ? "BLOCK" : "NORMAL",
    decision: blocked ? "BLOCK" : "ALLOW",
    threshold: 0,
    modelVersion: "—",
    featureVersion: "—",
    receivedAt: item.createdAt,
    handled: false,
    transaction: {
      transactionId: item.transactionId,
      sourceAccountNo: item.sourceAccountNo,
      sourceAccountName: "",
      destinationAccountNo: item.destinationAccountNo,
      destinationName: "",
      amount: item.amount,
      fee: item.fee,
      currency: item.currency,
      channel: "",
      status: item.status,
      createdAt: item.createdAt,
    },
  };
}

function downloadReportCsv(date: string, report: TransactionReport | null, items: RiskAssessment[]) {
  if (!report) return;
  const rows = [
    ["Báo cáo ngày", date],
    ["Tổng giao dịch", report.summary.transactionCount],
    ["Tổng doanh số thành công", report.summary.totalAmount],
    ["Tổng phí", report.summary.totalFee],
    ["Giá trị trung bình", report.summary.averageAmount],
    ["Thành công", report.summary.successCount],
    ["Thất bại", report.summary.failedCount],
    ["Bị chặn", report.summary.blockedCount],
    [],
    ["Transaction ID", "Source", "Destination", "Amount", "Status", "Risk", "Received At"],
    ...items.map((item) => [
      item.transactionId,
      item.transaction?.sourceAccountNo || "",
      item.transaction?.destinationAccountNo || "",
      item.transaction?.amount || 0,
      item.transaction?.status || "",
      item.decision,
      item.receivedAt,
    ]),
  ];
  const csv = "\ufeff" + rows.map((row) => row.map((value) => `"${String(value).replace(/"/g, '""')}"`).join(",")).join("\n");
  const url = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `bao-cao-giao-dich-${date}.csv`;
  anchor.click();
  URL.revokeObjectURL(url);
}

function downloadReportExcel(date: string, report: TransactionReport | null, items: RiskAssessment[]) {
  if (!report) return;
  const escapeHtml = (value: unknown) => String(value).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  const rows = items.map((item) => `<tr><td>${escapeHtml(item.transactionId)}</td><td>${escapeHtml(item.transaction?.sourceAccountNo || "")}</td><td>${escapeHtml(item.transaction?.destinationAccountNo || "")}</td><td>${escapeHtml(item.transaction?.amount || 0)}</td><td>${escapeHtml(item.transaction?.status || "")}</td><td>${escapeHtml(item.decision)}</td><td>${escapeHtml(item.receivedAt)}</td></tr>`).join("");
  const html = `<html><head><meta charset="utf-8"><style>body{font-family:Arial}table{border-collapse:collapse}th,td{border:1px solid #cbd5e1;padding:8px;min-width:140px}th{background:#1f4e78;color:white}h1{font-size:18px}</style></head><body><h1>Báo cáo giao dịch ngày ${escapeHtml(date)}</h1><p>Tổng doanh số: ${escapeHtml(report.summary.totalAmount)} | Tổng phí: ${escapeHtml(report.summary.totalFee)} | Thành công: ${escapeHtml(report.summary.successCount)} | Thất bại: ${escapeHtml(report.summary.failedCount)} | Bị chặn: ${escapeHtml(report.summary.blockedCount)}</p><table><thead><tr><th>Transaction ID</th><th>Source</th><th>Destination</th><th>Amount</th><th>Status</th><th>Risk</th><th>Received At</th></tr></thead><tbody>${rows}</tbody></table></body></html>`;
  const url = URL.createObjectURL(new Blob([html], { type: "application/vnd.ms-excel" }));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = `bao-cao-giao-dich-${date}.xls`;
  anchor.click();
  URL.revokeObjectURL(url);
}

function ReportDateField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const openPicker = () => {
    const input = inputRef.current;
    if (!input) return;
    if (typeof input.showPicker === "function") input.showPicker();
    else input.focus();
  };
  return (
    <div className="flex h-9 w-36 items-center rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 focus-within:border-(--admin-primary)">
      <input ref={inputRef} className="min-w-0 flex-1 appearance-none bg-transparent px-0 text-[12px] text-(--admin-text) outline-none" type="date" aria-label="Ngày báo cáo" value={value} onChange={(event) => onChange(event.target.value)} />
      <button className="grid h-6 w-6 shrink-0 place-items-center rounded-sm text-(--admin-text-muted) hover:bg-(--admin-primary-soft) hover:text-(--admin-primary)" type="button" aria-label="Chọn ngày báo cáo" onClick={openPicker}>
        <FontAwesomeIcon icon={faCalendarDays} className="text-[12px]" aria-hidden="true" />
      </button>
    </div>
  );
}

function MetricCard({ label, value, note, icon, tone = "default" }: { label: string; value: string | number; note: string; icon: typeof faShieldHalved; tone?: "default" | "danger" | "primary" }) {
  const color = tone === "danger" ? "text-(--admin-danger)" : tone === "primary" ? "text-(--admin-primary)" : "text-(--admin-text)";
  return (
    <article className={`${surface} rounded-(--admin-radius-md) border ${border} p-3.5`}>
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-semibold text-(--admin-text-muted)">{label}</span>
        <span className="grid h-7 w-7 place-items-center rounded-sm bg-(--admin-surface-subtle) text-[12px] text-(--admin-text-muted)"><FontAwesomeIcon icon={icon} /></span>
      </div>
      <strong className={`${mono} mt-3 block text-[23px] leading-6 font-bold ${color}`}>{value}</strong>
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
  const [response, setResponse] = useState<MonitorDashboardResponse | null>(null);
  const [report, setReport] = useState<TransactionReport | null>(null);
  const [selected, setSelected] = useState<RiskAssessment | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = async (showSpinner = true) => {
    if (showSpinner) setRefreshing(true);
    setError(null);
    try {
      const [monitor, dailyReport] = await Promise.all([getDailyMonitorReport(date), getTransactionReport(date)]);
      setResponse(monitor);
      setReport(dailyReport);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Không thể tải báo cáo giao dịch.");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    void load(false);
    const timer = window.setInterval(() => void load(false), 15000);
    return () => window.clearInterval(timer);
  }, [date]);

  useEffect(() => {
    setPage(1);
  }, [date, decision, query, status]);

  const items = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    const existing = new Map((response?.snapshot.recentAssessments || []).map((item) => [item.transactionId, item]));
    return (report?.transactions || []).map((item) => reportItemToAssessment(item, existing.get(item.transactionId)))
      .filter((item) => status === "all" || item.transaction?.status === status)
      .filter((item) => decision === "all" || item.decision === decision)
      .filter((item) => !normalized || [item.transactionId, item.transaction?.sourceAccountNo, item.transaction?.destinationAccountNo, item.transaction?.sourceAccountName, item.transaction?.destinationName].some((value) => value?.toLowerCase().includes(normalized)))
      .sort((left, right) => Date.parse(right.receivedAt) - Date.parse(left.receivedAt));
  }, [decision, query, report, response, status]);

  const snapshot = response?.snapshot;
  const summary = report?.summary;
  const previous = report?.previous;
  const totalPages = Math.max(1, Math.ceil(items.length / PAGE_SIZE));
  const currentPage = Math.min(page, totalPages);
  const visibleItems = items.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE);

  if (loading) return <section className={`${surface} grid min-h-80 place-items-center rounded-(--admin-radius-md) text-sm text-(--admin-text-muted)`}>Đang tải báo cáo...</section>;

  return (
    <div className="grid gap-4" aria-label="Giao dịch và rủi ro">
      <header className={`${surface} rounded-(--admin-radius-md) border ${border} p-4`}>
        <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
          <div className="flex items-start gap-3">
            <span className="grid h-10 w-10 shrink-0 place-items-center rounded-(--admin-radius-md) bg-(--admin-primary-soft) text-(--admin-primary)"><FontAwesomeIcon icon={faShieldHalved} /></span>
            <div><p className={`${mono} text-[10px] font-semibold uppercase tracking-[0.08em] text-(--admin-primary)`}>BÁO CÁO VẬN HÀNH</p><h1 className="mt-1 text-[21px] font-bold tracking-[-0.03em] text-(--admin-text)">Giao dịch &amp; Rủi ro</h1><p className="mt-1 text-[12px] text-(--admin-text-muted)">Tổng hợp hoạt động trong ngày và kết quả phát hiện bất thường.</p></div>
          </div>
          <div className="flex items-end gap-2"><div><p className={`${mono} mb-1 text-[10px] uppercase text-(--admin-text-muted)`}>Ngày báo cáo</p><div className="flex items-center gap-1.5"><ReportDateField value={date} onChange={setDate} /><button type="button" className={`${surface} h-9 rounded-sm border ${border} px-2.5 text-[11px] font-semibold text-(--admin-text) hover:bg-(--admin-surface-hover)`} onClick={() => setDate(todayInVietnam())} disabled={date === todayInVietnam()}>Hôm nay</button></div></div><button type="button" className={`${surface} grid h-9 w-9 place-items-center rounded-sm border ${border} text-(--admin-text-muted) hover:bg-(--admin-surface-hover) hover:text-(--admin-text)`} onClick={() => void load()} disabled={refreshing} aria-label="Làm mới báo cáo"><FontAwesomeIcon icon={faArrowsRotate} className={refreshing ? "animate-spin" : ""} /></button></div>
        </div>
      </header>

      {error && <p role="alert" className="rounded-sm border border-(--admin-danger) bg-(--admin-danger-soft) p-3 text-[12px] text-(--admin-danger)">{error}</p>}

      <section className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <MetricCard label="Tổng giao dịch" value={summary?.transactionCount || 0} note={`So với hôm qua: ${percentChange(summary?.transactionCount || 0, previous?.transactionCount || 0)}`} icon={faShieldHalved} />
        <MetricCard label="Tổng doanh số" value={money(summary?.totalAmount)} note={`So với hôm qua: ${percentChange(summary?.totalAmount || 0, previous?.totalAmount || 0)}`} icon={faMoneyBillTransfer} tone="primary" />
        <MetricCard label="Tổng phí" value={money(summary?.totalFee)} note="Phí của giao dịch thành công" icon={faCircleCheck} />
        <MetricCard label="Giá trị trung bình" value={money(summary?.averageAmount)} note="Trên giao dịch thành công" icon={faArrowTrendUp} />
        <MetricCard label="Hoàn tất" value={summary?.successCount || 0} note={share(summary?.successCount || 0, summary?.transactionCount || 0)} icon={faCircleCheck} tone="primary" />
        <MetricCard label="Thất bại" value={summary?.failedCount || 0} note={share(summary?.failedCount || 0, summary?.transactionCount || 0)} icon={faCircleXmark} tone="danger" />
        <MetricCard label="Bị chặn" value={summary?.blockedCount || 0} note={share(summary?.blockedCount || 0, summary?.transactionCount || 0)} icon={faCircleExclamation} tone="danger" />
        <MetricCard label="Risk trung bình" value={`${((snapshot?.averageRiskScore || 0) * 100).toFixed(1)}%`} note="Điểm rủi ro trong ngày" icon={faArrowTrendUp} />
      </section>

      <DailyReportChart hourly={report?.hourly || []} date={date} summary={summary} />

      <section className="grid gap-3 lg:grid-cols-2">
        {[
          ["Nguồn có nhiều rủi ro", report?.topRiskSources || []],
          ["Điểm nhận có nhiều rủi ro", report?.topRiskReceivers || []],
        ].map(([title, values]) => (
          <article key={title as string} className={`${surface} rounded-(--admin-radius-md) border ${border} p-4`}>
            <h2 className="text-[14px] font-bold text-(--admin-text)">{title as string}</h2>
            <div className="mt-3 grid gap-2">
              {(values as TransactionReport["topRiskSources"]).map((item) => (
                <div key={item.accountNo} className="flex items-center justify-between gap-3 rounded-sm bg-(--admin-surface-subtle) px-3 py-2 text-[11px]">
                  <div><p className={`${mono} font-semibold text-(--admin-text)`}>{item.accountNo}</p><p className="text-(--admin-text-muted)">{item.name || "Chưa có tên"}</p></div>
                  <div className="text-right"><p className="font-semibold text-(--admin-danger)">{item.count} cảnh báo</p><p className={`${mono} text-[10px] text-(--admin-text-muted)`}>{money(item.amount)}</p></div>
                </div>
              ))}
              {!(values as TransactionReport["topRiskSources"]).length && <p className="text-[11px] text-(--admin-text-muted)">Chưa có dữ liệu rủi ro.</p>}
            </div>
          </article>
        ))}
      </section>

      <section className={`${surface} overflow-hidden rounded-(--admin-radius-md) border ${border}`}>
        <div className="border-b border-(--admin-border) p-4"><div className="flex flex-col justify-between gap-3 lg:flex-row lg:items-center"><div><h2 className="text-[15px] font-bold text-(--admin-text)">Danh sách giao dịch</h2><p className="mt-1 text-[11px] text-(--admin-text-muted)">Các assessment gần nhất của ngày được chọn.</p></div><div className="flex flex-wrap gap-2"><div className={`flex h-8 min-w-55 items-center gap-2 rounded-sm border ${border} bg-(--admin-background) px-2.5`}><FontAwesomeIcon icon={faMagnifyingGlass} className="text-[11px] text-(--admin-text-muted)" /><input className="min-w-0 flex-1 bg-transparent text-[12px] text-(--admin-text) outline-none" placeholder="Tìm mã hoặc tài khoản" value={query} onChange={(event) => setQuery(event.target.value)} /></div><select className={`${subtle} h-8 rounded-sm border ${border} px-2 text-[11px] text-(--admin-text)`} value={status} onChange={(event) => setStatus(event.target.value as StatusFilter)} aria-label="Trạng thái"><option value="all">Tất cả trạng thái</option><option value="awaiting_otp">Chờ xử lý</option><option value="success">Hoàn tất</option><option value="failed">Bị từ chối</option></select><select className={`${subtle} h-8 rounded-sm border ${border} px-2 text-[11px] text-(--admin-text)`} value={decision} onChange={(event) => setDecision(event.target.value as DecisionFilter)} aria-label="Kết quả rủi ro"><option value="all">Tất cả risk</option><option value="ALLOW">Cho phép</option><option value="BLOCK">Chặn</option></select><button type="button" className={`${subtle} flex h-8 items-center gap-1.5 rounded-sm border ${border} px-2 text-[11px] font-semibold text-(--admin-text)`} onClick={() => downloadReportCsv(date, report, items)} disabled={!report}><FontAwesomeIcon icon={faDownload} /> CSV</button><button type="button" className={`${subtle} flex h-8 items-center gap-1.5 rounded-sm border ${border} px-2 text-[11px] font-semibold text-(--admin-text)`} onClick={() => downloadReportExcel(date, report, items)} disabled={!report}><FontAwesomeIcon icon={faDownload} /> Excel</button><button type="button" className={`${subtle} flex h-8 items-center gap-1.5 rounded-sm border ${border} px-2 text-[11px] font-semibold text-(--admin-text)`} onClick={() => window.print()}><FontAwesomeIcon icon={faFilePdf} /> PDF</button></div></div></div>
        <div className="overflow-x-auto"><table className="w-full min-w-245 text-left text-[12px]"><thead className="border-b border-(--admin-border) text-[10px] uppercase tracking-[0.04em] text-(--admin-text-muted)"><tr><th className="px-4 py-3 font-semibold">Giao dịch</th><th className="px-4 py-3 font-semibold">Từ / đến</th><th className="px-4 py-3 font-semibold">Số tiền</th><th className="px-4 py-3 font-semibold">Trạng thái</th><th className="px-4 py-3 font-semibold">Risk</th><th className="px-4 py-3 font-semibold">Thời gian</th></tr></thead><tbody className="divide-y divide-(--admin-border)">{visibleItems.map((item) => { const tx = item.transaction; const tone = statusTone(tx?.status); const risk = riskTone(item.decision); return <tr key={item.assessmentId} className="cursor-pointer transition-colors hover:bg-(--admin-surface-hover)" onClick={() => setSelected(item)}><td className="px-4 py-3"><span className={`${mono} text-[11px] font-semibold text-(--admin-text)`}>{item.transactionId.slice(0, 12)}…</span></td><td className="px-4 py-3"><span className="text-(--admin-text)">{tx?.sourceAccountNo || "—"}</span><div className="mt-1 text-[11px] text-(--admin-text-muted)">→ {tx?.destinationAccountNo || "—"}</div></td><td className={`${mono} px-4 py-3 font-semibold text-(--admin-text)`}>{money(tx?.amount, tx?.currency)}</td><td className={`px-4 py-3 font-semibold ${tone}`}>{statusLabel(tx?.status)}</td><td className="px-4 py-3"><span className={`font-semibold ${risk}`}>{decisionLabels[item.decision] || item.decision}</span><div className={`${mono} mt-1 text-[10px] text-(--admin-text-muted)`}>{(item.riskScore * 100).toFixed(1)}%</div></td><td className="px-4 py-3 text-[11px] text-(--admin-text-muted)">{formatDate(item.receivedAt)}</td></tr>; })}</tbody></table></div>
        {!items.length && <div className="p-10 text-center"><FontAwesomeIcon icon={faShieldHalved} className="text-2xl text-(--admin-text-muted)" /><p className="mt-2 text-[12px] font-semibold text-(--admin-text)">Không có dữ liệu phù hợp</p><p className="mt-1 text-[11px] text-(--admin-text-muted)">Thử đổi ngày hoặc bộ lọc.</p></div>}
        <div className="flex items-center justify-center gap-3 border-t border-(--admin-border) pt-3"><button type="button" className={`${surface} h-9 rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`} onClick={() => setPage((value) => Math.max(1, value - 1))} disabled={currentPage === 1}>← Trang trước</button><span className={`${mono} min-w-18 text-center text-[11px] text-(--admin-text-muted)`}>Trang {currentPage}</span><button type="button" className={`${surface} h-9 rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`} onClick={() => setPage((value) => Math.min(totalPages, value + 1))} disabled={currentPage === totalPages}>Trang sau →</button></div>
      </section>

      {selected && <><button className="fixed inset-0 z-30 cursor-default bg-black/40" onClick={() => setSelected(null)} aria-label="Đóng chi tiết" /><aside className="fixed inset-y-0 right-0 z-40 w-full max-w-105 overflow-y-auto border-l border-(--admin-border) bg-(--admin-surface) p-5 shadow-2xl" aria-label="Chi tiết giao dịch"><div className="flex items-start justify-between border-b border-(--admin-border) pb-4"><div><p className={`${mono} text-[10px] font-semibold uppercase tracking-[0.08em] text-(--admin-primary)`}>Chi tiết transaction</p><h2 className={`${mono} mt-1 text-[15px] font-bold text-(--admin-text)`}>{selected.transactionId}</h2></div><button type="button" className="grid h-8 w-8 place-items-center rounded-sm text-(--admin-text-muted) hover:bg-(--admin-surface-hover) hover:text-(--admin-text)" onClick={() => setSelected(null)} aria-label="Đóng"><FontAwesomeIcon icon={faXmark} /></button></div><div className="grid gap-3 py-4 text-[12px]"><div className={`${subtle} rounded-(--admin-radius-md) p-3`}><p className="font-semibold text-(--admin-text)">Risk assessment</p><div className="mt-3 grid grid-cols-2 gap-3"><div><p className="text-[10px] text-(--admin-text-muted)">Quyết định</p><strong className={riskTone(selected.decision)}>{decisionLabels[selected.decision] || selected.decision}</strong></div><div><p className="text-[10px] text-(--admin-text-muted)">Risk score</p><strong className={mono}>{(selected.riskScore * 100).toFixed(2)}%</strong></div><div><p className="text-[10px] text-(--admin-text-muted)">Threshold</p><strong className={mono}>{(selected.threshold * 100).toFixed(2)}%</strong></div><div><p className="text-[10px] text-(--admin-text-muted)">Model</p><strong className="break-all text-[11px]">{selected.modelVersion}</strong></div></div></div><div className={`${subtle} rounded-(--admin-radius-md) p-3`}><p className="font-semibold text-(--admin-text)">Thông tin giao dịch</p><dl className="mt-3 grid gap-2"><div className="flex justify-between gap-3"><dt className="text-(--admin-text-muted)">Từ</dt><dd className="text-right font-semibold">{selected.transaction?.sourceAccountNo || "—"}</dd></div><div className="flex justify-between gap-3"><dt className="text-(--admin-text-muted)">Đến</dt><dd className="text-right font-semibold">{selected.transaction?.destinationAccountNo || "—"}</dd></div><div className="flex justify-between gap-3"><dt className="text-(--admin-text-muted)">Số tiền</dt><dd className={`${mono} text-right font-semibold`}>{money(selected.transaction?.amount, selected.transaction?.currency)}</dd></div><div className="flex justify-between gap-3"><dt className="text-(--admin-text-muted)">Trạng thái</dt><dd className={statusTone(selected.transaction?.status)}>{statusLabel(selected.transaction?.status)}</dd></div><div className="flex justify-between gap-3"><dt className="text-(--admin-text-muted)">Nhận lúc</dt><dd className="text-right">{formatDate(selected.receivedAt)}</dd></div></dl></div></div></aside></>}
    </div>
  );
}
