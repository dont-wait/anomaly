import { FormEvent, useMemo, useRef, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faArrowDown, faArrowUp, faBolt, faCalendarDays, faCircleExclamation, faClockRotateLeft, faMagnifyingGlass, faShieldHalved } from "@fortawesome/free-solid-svg-icons";
import { AuditApiError, getAccountTimeline } from "./api/auditApi";
import type { TimelineCategory, TimelineItem, TimelineResponse } from "./model";

const surface = "bg-(--admin-surface)";
const subtle = "bg-(--admin-surface-subtle)";
const mono = "font-(family-name:--admin-font-mono)";
const PAGE_LIMIT = 20;

type AppliedTimelineQuery = {
  input: string;
  accountId: string;
  category: TimelineCategory;
  from: string;
  to: string;
};

const categoryLabels: Record<TimelineCategory, string> = {
  all: "Tất cả event",
  account: "Tài khoản",
  transfer: "Transfer",
  otp: "OTP",
  login: "Login",
  loan: "Khoản vay",
};

const eventLabels: Record<string, string> = {
  TransferCreated: "Yêu cầu chuyển tiền được tạo",
  TransferCompleted: "Chuyển tiền thành công",
  TransferFailed: "Chuyển tiền thất bại",
  TransferCancelled: "Chuyển tiền bị hủy",
  TransferOTPUpdated: "Metadata OTP được cập nhật",
  TransferOTPVerificationSucceeded: "OTP chuyển tiền xác thực thành công",
  TransferOTPVerificationFailed: "OTP chuyển tiền không hợp lệ",
  TransferOTPExpired: "OTP chuyển tiền hết hạn",
  TransferOTPAttemptsExceeded: "OTP chuyển tiền vượt số lần thử",
  EmailOTPVerificationSucceeded: "OTP email xác thực thành công",
  EmailOTPVerificationFailed: "OTP email không hợp lệ",
  EmailOTPExpired: "OTP email hết hạn",
  EmailOTPAttemptsExceeded: "OTP email vượt số lần thử",
  LoginSucceeded: "Đăng nhập thành công",
  LoginFailed: "Đăng nhập thất bại",
  BankAccountCreated: "Tài khoản được tạo",
  BankAccountUpdated: "Tài khoản được cập nhật",
  BankBalanceSeeded: "Số dư được thiết lập",
  LoanApplicationCreated: "Hồ sơ vay được tạo",
  LoanDocumentAdded: "Hồ sơ vay được bổ sung chứng từ",
  LoanApplicationSubmitted: "Hồ sơ vay được gửi",
  LoanReviewStarted: "Bắt đầu thẩm định khoản vay",
  LoanApplicationAssessed: "Khoản vay được thẩm định",
  LoanApplicationApproved: "Hồ sơ vay được phê duyệt",
  LoanApplicationRejected: "Hồ sơ vay bị từ chối",
  LoanOfferIssued: "Đề nghị khoản vay được phát hành",
  LoanOfferAccepted: "Đề nghị khoản vay được chấp nhận",
  LoanOfferRejected: "Đề nghị khoản vay bị từ chối",
  LoanContractConfirmed: "Hợp đồng vay được xác nhận",
  LoanDisbursementPrepared: "Khoản vay được chuẩn bị giải ngân",
  LoanDisbursed: "Khoản vay được giải ngân",
  LoanDisbursementCredited: "Tiền vay được cộng vào tài khoản",
  LoanInstallmentPaid: "Đã trả một kỳ khoản vay",
};

function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat("vi-VN", { dateStyle: "short", timeStyle: "medium", timeZone: "Asia/Ho_Chi_Minh" }).format(new Date(value));
}

function formatDay(value: string): string {
  return new Intl.DateTimeFormat("vi-VN", { dateStyle: "full", timeZone: "Asia/Ho_Chi_Minh" }).format(new Date(value));
}

const timelineDayKey = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Ho_Chi_Minh",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

function formatMoney(value: unknown, currency: string): string | null {
  if (typeof value !== "number") return null;
  return `${value.toLocaleString("vi-VN")} ${currency || "VND"}`;
}

function eventLabel(item: TimelineItem): string {
  return eventLabels[item.eventType] || item.summary || item.eventType;
}

function detailLines(item: TimelineItem, currency: string): string[] {
  const details = item.details || {};
  const lines: string[] = [];
  const amount = formatMoney(details.amount, typeof details.currency === "string" ? details.currency : currency);
  if (amount) lines.push(`Số tiền: ${amount}`);
  const before = formatMoney(details.balanceBefore, currency);
  const after = formatMoney(details.balanceAfter, currency);
  if (before || after) lines.push(`Số dư: ${before || "—"} → ${after || "—"}`);
  if (typeof details.direction === "string") lines.push(details.direction === "out" ? "Chiều: Gửi" : "Chiều: Nhận");
  if (typeof details.status === "string") lines.push(`Trạng thái: ${details.status}`);
  if (typeof details.reasonCode === "string") lines.push(`Lý do: ${details.reasonCode}`);
  if (typeof details.cancellationReason === "string") lines.push(`Lý do hủy: ${details.cancellationReason}`);
  if (typeof details.attemptsLeft === "number") lines.push(`Còn lại: ${details.attemptsLeft} lượt`);
  if (item.transactionId) lines.push(`Giao dịch: ${item.transactionId}`);
  if (item.loanId) lines.push(`Khoản vay: ${item.loanId}`);
  if (typeof details.applicationNo === "string") lines.push(`Mã hồ sơ: ${details.applicationNo}`);
  const requestedAmount = formatMoney(details.requestedAmount, currency);
  if (requestedAmount) lines.push(`Số tiền đề nghị: ${requestedAmount}`);
  if (typeof details.decision === "string") lines.push(`Kết quả: ${details.decision}`);
  if (typeof details.totalScore === "number") lines.push(`Điểm thẩm định: ${details.totalScore}`);
  if (typeof details.annualRateBps === "number") lines.push(`Lãi suất: ${(details.annualRateBps / 100).toFixed(2)}%/năm`);
  if (typeof details.installmentNo === "number") lines.push(`Kỳ trả: ${details.installmentNo}`);
  const principal = formatMoney(details.principalAmount, currency);
  const interest = formatMoney(details.interestAmount, currency);
  if (principal) lines.push(`Gốc kỳ này: ${principal}`);
  if (interest) lines.push(`Lãi kỳ này: ${interest}`);
  const outstanding = formatMoney(details.outstandingPrincipalAfter, currency);
  if (outstanding) lines.push(`Dư nợ gốc còn lại: ${outstanding}`);
  return lines;
}

function TimelineCard({ item, currency }: { item: TimelineItem; currency: string }) {
  const isPositive = item.eventType.endsWith("Succeeded") || item.eventType === "TransferCompleted" || item.eventType === "LoanDisbursementCredited" || item.eventType === "LoanDisbursed";
  const isNegative = item.eventType.endsWith("Failed") || item.eventType.endsWith("Expired") || item.eventType.endsWith("Exceeded") || item.eventType === "TransferCancelled" || item.eventType === "LoanApplicationRejected" || item.eventType === "LoanOfferRejected";
  const details = detailLines(item, currency);
  return (
    <article className={`${surface} grid grid-cols-[28px_minmax(0,1fr)_auto] gap-3 rounded-(--admin-radius-md) border border-(--admin-border) p-3`}>
      <div className={`grid h-7 w-7 place-items-center rounded-full ${isPositive ? "bg-(--admin-primary-soft) text-(--admin-primary)" : isNegative ? "bg-(--admin-danger-soft) text-(--admin-danger)" : "bg-(--admin-surface-hover) text-(--admin-text-muted)"}`}>
        <FontAwesomeIcon icon={isPositive ? faArrowUp : isNegative ? faArrowDown : faBolt} className="text-[11px]" aria-hidden="true" />
      </div>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <strong className="text-[13px] text-(--admin-text)">{eventLabel(item)}</strong>
          <span className={`${mono} rounded-sm bg-(--admin-surface-hover) px-1.5 py-0.5 text-[10px] uppercase text-(--admin-text-muted)`}>{categoryLabels[item.category]}</span>
        </div>
        {details.length > 0 && <div className="mt-1 grid gap-0.5 text-[11px] leading-4 text-(--admin-text-muted)">{details.map((line) => <span key={line}>{line}</span>)}</div>}
        <span className={`${mono} mt-2 block truncate text-[10px] text-(--admin-text-muted)`} title={`${item.sourceStream} @ ${item.sourceRevision}`}>{item.sourceStream} · revision {item.sourceRevision}</span>
      </div>
      <time className={`${mono} whitespace-nowrap text-[10px] text-(--admin-text-muted)`}>{formatDateTime(item.occurredAt)}</time>
    </article>
  );
}

function groupedItems(items: TimelineItem[]) {
  const groups = new Map<string, TimelineItem[]>();
  for (const item of items) {
    const key = timelineDayKey.format(new Date(item.occurredAt));
    const group = groups.get(key) || [];
    group.push(item);
    groups.set(key, group);
  }
  return [...groups.entries()];
}

function DateField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const openPicker = () => {
    const input = inputRef.current;
    if (!input) return;
    if (typeof input.showPicker === "function") {
      input.showPicker();
    } else {
      input.focus();
    }
  };

  return (
    <div className="flex h-9 items-center rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 focus-within:border-(--admin-primary)">
      <input ref={inputRef} className="min-w-0 flex-1 appearance-none bg-transparent px-0 text-[12px] text-(--admin-text) outline-none" type="date" aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} />
      <button className="grid h-6 w-6 shrink-0 place-items-center rounded-sm text-(--admin-text-muted) hover:bg-(--admin-primary-soft) hover:text-(--admin-primary)" type="button" aria-label={`Chọn ${label.toLowerCase()}`} onClick={openPicker}>
        <FontAwesomeIcon icon={faCalendarDays} className="text-[12px]" aria-hidden="true" />
      </button>
    </div>
  );
}

export function AccountTimelinePage() {
  const [accountInput, setAccountInput] = useState("");
  const [category, setCategory] = useState<TimelineCategory>("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [result, setResult] = useState<TimelineResponse | null>(null);
  const [items, setItems] = useState<TimelineItem[]>([]);
  const [appliedQuery, setAppliedQuery] = useState<AppliedTimelineQuery | null>(null);
  const [page, setPage] = useState(1);
  const [pageCursors, setPageCursors] = useState<Array<string | undefined>>([undefined]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [notFound, setNotFound] = useState(false);
  const accountInputRef = useRef<HTMLInputElement>(null);

  const groups = useMemo(() => groupedItems(items), [items]);
  const draftChanged = appliedQuery !== null && (
    appliedQuery.input !== accountInput.trim()
    || appliedQuery.category !== category
    || appliedQuery.from !== from
    || appliedQuery.to !== to
  );

  async function search(event?: FormEvent) {
    event?.preventDefault();
    const account = accountInput.trim();
    if (!account) {
      setError("");
      setNotFound(false);
      accountInputRef.current?.focus();
      return;
    }
    if ((from && !to) || (!from && to) || (from && to && from > to)) {
      setNotFound(false);
      setError("Khoảng ngày không hợp lệ.");
      return;
    }
    setLoading(true);
    setError("");
    setNotFound(false);
    setPage(1);
    setPageCursors([undefined]);
    try {
      const response = await getAccountTimeline(account, { category, from: from || undefined, to: to || undefined, limit: PAGE_LIMIT });
      setResult(response);
      setItems(response.items);
      setAppliedQuery({ input: account, accountId: response.account.id, category, from, to });
      setPageCursors(response.nextCursor ? [undefined, response.nextCursor] : [undefined]);
    } catch (reason) {
      setResult(null);
      setItems([]);
      const isNotFound = reason instanceof AuditApiError && reason.status === 404;
      setNotFound(isNotFound);
      setError(reason instanceof Error ? reason.message : "Không thể tải lịch sử tài khoản.");
    } finally {
      setLoading(false);
    }
  }

  async function loadPage(pageCursor: string | undefined, targetPage: number) {
    if (loading || !appliedQuery || draftChanged) return;
    setLoading(true);
    setError("");
    try {
      const response = await getAccountTimeline(appliedQuery.accountId, { category: appliedQuery.category, from: appliedQuery.from || undefined, to: appliedQuery.to || undefined, limit: PAGE_LIMIT, cursor: pageCursor });
      setResult(response);
      setItems(response.items);
      setPage(targetPage);
      setPageCursors((current) => {
        const next = current.slice(0, targetPage);
        next[targetPage - 1] = pageCursor;
        if (response.nextCursor) next[targetPage] = response.nextCursor;
        return next;
      });
    } catch (reason) {
      setNotFound(false);
      setError(reason instanceof Error ? reason.message : "Không thể tải trang lịch sử.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="grid gap-3">
      <section className={`${subtle} rounded-(--admin-radius-md) p-4`}>
        <div className="flex items-start gap-3">
          <div className="grid h-9 w-9 shrink-0 place-items-center rounded-(--admin-radius-md) bg-(--admin-primary-soft) text-(--admin-primary)"><FontAwesomeIcon icon={faClockRotateLeft} aria-hidden="true" /></div>
          <div>
            <h1 className="text-[18px] font-bold uppercase tracking-[-0.02em] text-(--admin-text)">Lịch sử tài khoản</h1>
            <p className="mt-1 text-[12px] leading-5 text-(--admin-text-muted)">Xem timeline event đã ghi nhận. Màn hình chỉ đọc, không thực hiện lại nghiệp vụ.</p>
          </div>
        </div>
        <form className="mt-4 grid gap-2 md:grid-cols-[minmax(220px,1fr)_150px_140px_140px_auto]" onSubmit={(event) => void search(event)}>
          <label className="flex h-9 items-center gap-2 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 focus-within:border-(--admin-primary)">
            <FontAwesomeIcon icon={faMagnifyingGlass} className="text-[12px] text-(--admin-text-muted)" aria-hidden="true" />
            <span className="sr-only">Tài khoản</span>
            <input ref={accountInputRef} className="min-w-0 flex-1 bg-transparent text-[12px] text-(--admin-text) outline-none placeholder:text-(--admin-text-muted)" placeholder="Account ID, số tài khoản, username..." value={accountInput} onChange={(event) => { setAccountInput(event.target.value); if (error) setError(""); if (notFound) setNotFound(false); }} />
          </label>
          <select className="h-9 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 text-[12px] text-(--admin-text) outline-none" value={category} onChange={(event) => setCategory(event.target.value as TimelineCategory)} aria-label="Loại event">
            {(Object.keys(categoryLabels) as TimelineCategory[]).map((key) => <option key={key} value={key}>{categoryLabels[key]}</option>)}
          </select>
          <DateField label="Từ ngày" value={from} onChange={setFrom} />
          <DateField label="Đến ngày" value={to} onChange={setTo} />
          <button className="h-9 rounded-sm bg-(--admin-primary) px-4 text-[12px] font-semibold text-white hover:bg-(--admin-primary-strong) disabled:cursor-not-allowed disabled:opacity-50" type="submit" disabled={loading || !accountInput.trim()}>{loading ? "Đang tải..." : "Xem lịch sử"}</button>
        </form>
        {error && <div className={`mt-3 flex items-center gap-2 rounded-sm border px-3 py-2 text-[11px] ${notFound ? "border-(--admin-border) bg-(--admin-surface) text-(--admin-text-muted)" : "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)"}`} role={notFound ? "status" : "alert"}>{!notFound && <FontAwesomeIcon icon={faCircleExclamation} aria-hidden="true" />}{error}</div>}
      </section>

      {!result && !error && <section className={`${surface} grid min-h-55 place-items-center rounded-(--admin-radius-md) border border-(--admin-border) p-6 text-center`}><div className="grid max-w-100 gap-2"><FontAwesomeIcon icon={faShieldHalved} className="mx-auto text-2xl text-(--admin-text-muted)" aria-hidden="true" /><strong className="text-[14px] text-(--admin-text)">Nhập tài khoản để bắt đầu</strong><span className="text-[12px] leading-5 text-(--admin-text-muted)">Có thể tìm bằng account ID, số tài khoản hoặc username.</span></div></section>}

      {result && <section className="grid gap-3">
        <div className={`${surface} flex flex-wrap items-center justify-between gap-3 rounded-(--admin-radius-md) border border-(--admin-border) p-3`}>
          <div><strong className="text-[14px] text-(--admin-text)">{result.account.customerName || result.account.username}</strong><span className={`${mono} ml-2 text-[11px] text-(--admin-primary)`}>{result.account.accountNo}</span><div className="mt-1 text-[11px] text-(--admin-text-muted)">{result.account.id} · {result.account.status} · Số dư hiện tại: {formatMoney(result.account.balance, result.account.currency)}</div></div>
          <span className={`${mono} text-[11px] text-(--admin-text-muted)`}>{items.length} event · Trang {page}</span>
        </div>
        {groups.length === 0 ? <div className={`${surface} grid min-h-48 place-items-center rounded-(--admin-radius-md) border border-(--admin-border) text-[12px] text-(--admin-text-muted)`}>Không có event trong điều kiện đã chọn.</div> : groups.map(([day, dayItems]) => <section key={day} className="grid gap-2"><h2 className={`${mono} px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-(--admin-text-muted)`}>{formatDay(day)}</h2>{dayItems.map((item) => <TimelineCard key={`${item.sourceStream}:${item.sourceRevision}:${item.eventId}`} item={item} currency={result.account.currency} />)}</section>)}
        <div className="flex items-center justify-center gap-3 pt-1">
          <button className={`${surface} h-9 rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`} type="button" onClick={() => void loadPage(pageCursors[page - 2], page - 1)} disabled={loading || page === 1 || !appliedQuery || draftChanged}>{"← Trang trước"}</button>
          <span className={`${mono} min-w-18 text-center text-[11px] text-(--admin-text-muted)`}>Trang {page}</span>
          <button className={`${surface} h-9 rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`} type="button" onClick={() => void loadPage(result.nextCursor, page + 1)} disabled={loading || !result.hasMore || !appliedQuery || draftChanged}>{"Trang sau →"}</button>
        </div>
      </section>}
    </div>
  );
}
