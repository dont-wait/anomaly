import { FormEvent, useMemo, useRef, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faArrowDown, faArrowUp, faBolt, faCircleExclamation, faClockRotateLeft, faMagnifyingGlass, faShieldHalved } from "@fortawesome/free-solid-svg-icons";
import { getAccountTimeline } from "./api/auditApi";
import type { TimelineCategory, TimelineItem, TimelineResponse } from "./model";

const surface = "bg-(--admin-surface)";
const subtle = "bg-(--admin-surface-subtle)";
const mono = "font-(family-name:--admin-font-mono)";

const categoryLabels: Record<TimelineCategory, string> = {
  all: "Tất cả event",
  account: "Tài khoản",
  transfer: "Transfer",
  otp: "OTP",
  login: "Login",
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
  return lines;
}

function TimelineCard({ item, currency }: { item: TimelineItem; currency: string }) {
  const isPositive = item.eventType.endsWith("Succeeded") || item.eventType === "TransferCompleted";
  const isNegative = item.eventType.endsWith("Failed") || item.eventType.endsWith("Expired") || item.eventType.endsWith("Exceeded") || item.eventType === "TransferCancelled";
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

export function AccountTimelinePage() {
  const [accountInput, setAccountInput] = useState("");
  const [category, setCategory] = useState<TimelineCategory>("all");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [result, setResult] = useState<TimelineResponse | null>(null);
  const [items, setItems] = useState<TimelineItem[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const accountInputRef = useRef<HTMLInputElement>(null);

  const groups = useMemo(() => groupedItems(items), [items]);

  async function search(event?: FormEvent) {
    event?.preventDefault();
    const account = accountInput.trim();
    if (!account) {
      setError("");
      accountInputRef.current?.focus();
      return;
    }
    if ((from && !to) || (!from && to) || (from && to && from > to)) {
      setError("Khoảng ngày không hợp lệ.");
      return;
    }
    setLoading(true);
    setError("");
    setCursor(undefined);
    try {
      const response = await getAccountTimeline(account, { category, from: from || undefined, to: to || undefined });
      setResult(response);
      setItems(response.items);
      setCursor(response.nextCursor);
    } catch (reason) {
      setResult(null);
      setItems([]);
      setError(reason instanceof Error ? reason.message : "Không thể tải lịch sử tài khoản.");
    } finally {
      setLoading(false);
    }
  }

  async function loadMore() {
    if (!result?.hasMore || !cursor || loading) return;
    setLoading(true);
    try {
      const response = await getAccountTimeline(accountInput.trim(), { category, from: from || undefined, to: to || undefined, cursor });
      setItems((current) => [...current, ...response.items]);
      setCursor(response.nextCursor);
      setResult(response);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Không thể tải thêm lịch sử.");
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
            <input ref={accountInputRef} className="min-w-0 flex-1 bg-transparent text-[12px] text-(--admin-text) outline-none placeholder:text-(--admin-text-muted)" placeholder="Account ID, số tài khoản, username..." value={accountInput} onChange={(event) => { setAccountInput(event.target.value); if (error) setError(""); }} />
          </label>
          <select className="h-9 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 text-[12px] text-(--admin-text) outline-none" value={category} onChange={(event) => setCategory(event.target.value as TimelineCategory)} aria-label="Loại event">
            {(Object.keys(categoryLabels) as TimelineCategory[]).map((key) => <option key={key} value={key}>{categoryLabels[key]}</option>)}
          </select>
          <input className="h-9 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 text-[12px] text-(--admin-text) outline-none" type="date" aria-label="Từ ngày" value={from} onChange={(event) => setFrom(event.target.value)} />
          <input className="h-9 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 text-[12px] text-(--admin-text) outline-none" type="date" aria-label="Đến ngày" value={to} onChange={(event) => setTo(event.target.value)} />
          <button className="h-9 rounded-sm bg-(--admin-primary) px-4 text-[12px] font-semibold text-white hover:bg-(--admin-primary-strong) disabled:cursor-not-allowed disabled:opacity-50" type="submit" disabled={loading || !accountInput.trim()}>{loading ? "Đang tải..." : "Xem lịch sử"}</button>
        </form>
        {error && <div className="mt-3 flex items-center gap-2 rounded-sm border border-(--admin-danger) bg-(--admin-danger-soft) px-3 py-2 text-[11px] text-(--admin-danger)" role="alert"><FontAwesomeIcon icon={faCircleExclamation} aria-hidden="true" />{error}</div>}
      </section>

      {!result && !error && <section className={`${surface} grid min-h-55 place-items-center rounded-(--admin-radius-md) border border-(--admin-border) p-6 text-center`}><div className="grid max-w-100 gap-2"><FontAwesomeIcon icon={faShieldHalved} className="mx-auto text-2xl text-(--admin-text-muted)" aria-hidden="true" /><strong className="text-[14px] text-(--admin-text)">Nhập tài khoản để bắt đầu</strong><span className="text-[12px] leading-5 text-(--admin-text-muted)">Có thể tìm bằng account ID, số tài khoản hoặc username.</span></div></section>}

      {result && <section className="grid gap-3">
        <div className={`${surface} flex flex-wrap items-center justify-between gap-3 rounded-(--admin-radius-md) border border-(--admin-border) p-3`}>
          <div><strong className="text-[14px] text-(--admin-text)">{result.account.customerName || result.account.username}</strong><span className={`${mono} ml-2 text-[11px] text-(--admin-primary)`}>{result.account.accountNo}</span><div className="mt-1 text-[11px] text-(--admin-text-muted)">{result.account.id} · {result.account.status} · Số dư hiện tại: {formatMoney(result.account.balance, result.account.currency)}</div></div>
          <span className={`${mono} text-[11px] text-(--admin-text-muted)`}>{items.length} event đã tải</span>
        </div>
        {groups.length === 0 ? <div className={`${surface} grid min-h-48 place-items-center rounded-(--admin-radius-md) border border-(--admin-border) text-[12px] text-(--admin-text-muted)`}>Không có event trong điều kiện đã chọn.</div> : groups.map(([day, dayItems]) => <section key={day} className="grid gap-2"><h2 className={`${mono} px-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-(--admin-text-muted)`}>{formatDay(day)}</h2>{dayItems.map((item) => <TimelineCard key={`${item.sourceStream}:${item.sourceRevision}:${item.eventId}`} item={item} currency={result.account.currency} />)}</section>)}
        {result.hasMore && <button className={`${surface} mx-auto h-9 rounded-sm border border-(--admin-border) px-4 text-[12px] font-semibold text-(--admin-text) hover:border-(--admin-primary) disabled:opacity-60`} type="button" onClick={() => void loadMore()} disabled={loading}>{loading ? "Đang tải..." : "Xem thêm"}</button>}
      </section>}
    </div>
  );
}
