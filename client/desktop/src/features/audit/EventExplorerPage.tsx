import { FormEvent, useRef, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowRight,
  faCircleExclamation,
  faCode,
  faMagnifyingGlass,
  faRotate,
} from "@fortawesome/free-solid-svg-icons";
import { navigate, routes } from "@/app/routes";
import {
  EventExplorerApiError,
  getEventExplorer,
  getEventExplorerDetail,
  type EventExplorerQuery,
} from "./api/eventExplorerApi";
import type {
  EventExplorerCategory,
  EventExplorerItem,
  EventExplorerListItem,
  EventExplorerResponse,
} from "./model";
import { useAdminTheme } from "@/shared/theme/theme";

const surface = "bg-(--admin-surface)";
const subtle = "bg-(--admin-surface-subtle)";
const mono = "font-(family-name:--admin-font-mono)";

const categoryLabels: Record<EventExplorerCategory, string> = {
  all: "Tất cả event",
  account: "Tài khoản",
  transfer: "Transfer",
  otp: "OTP",
  login: "Login",
  loan: "Khoản vay",
  other: "Khác",
};

const searchTypeLabels: Record<EventExplorerQuery["searchType"], string> = {
  transactionId: "Mã giao dịch",
  account: "Số tài khoản",
};

const eventTypeLabels: Record<string, string> = {
  TransferOTPVerificationFailed: "Xác thực OTP chuyển tiền thất bại",
  TransferOTPVerificationSucceeded: "Xác thực OTP chuyển tiền thành công",
  TransferOTPExpired: "OTP chuyển tiền hết hạn",
  TransferOTPAttemptsExceeded: "OTP chuyển tiền vượt số lần thử",
  TransferCreated: "Yêu cầu chuyển tiền được tạo",
  TransferCompleted: "Chuyển tiền thành công",
  TransferFailed: "Chuyển tiền thất bại",
  TransferCancelled: "Chuyển tiền bị hủy",
  LoginSucceeded: "Đăng nhập thành công",
  LoginFailed: "Đăng nhập thất bại",
  BankAccountCreated: "Tài khoản được tạo",
  BankAccountVerified: "Tài khoản được xác thực",
  BankAccountWithdraw: "Tài khoản bị trừ tiền",
};

const initialQuery: EventExplorerQuery = {
  searchType: "transactionId",
  search: "",
  category: "all",
  eventType: "",
  status: "",
  from: "",
  to: "",
};

function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat("vi-VN", {
    dateStyle: "short",
    timeStyle: "medium",
    timeZone: "Asia/Ho_Chi_Minh",
  }).format(new Date(value));
}

type EventExplorerDisplayItem = EventExplorerListItem | EventExplorerItem;

function eventLabel(item: EventExplorerDisplayItem): string {
  const eventType = "eventType" in item ? item.eventType : "";
  return eventTypeLabels[eventType] || item.summary || eventType;
}

function formatMoney(value: unknown, currency: unknown): string | null {
  if (typeof value !== "number") return null;
  return `${value.toLocaleString("vi-VN")} ${typeof currency === "string" && currency ? currency : "VND"}`;
}

function businessDetails(item: EventExplorerDisplayItem): string[] {
  const details = "details" in item ? item.details || {} : {};
  const lines: string[] = [];
  const amount = formatMoney(details.amount, details.currency);
  if (amount) lines.push(`Số tiền: ${amount}`);
  if (typeof details.status === "string")
    lines.push(
      `Trạng thái: ${details.status === "success" ? "Thành công" : details.status}`,
    );
  if (typeof details.reasonCode === "string")
    lines.push(
      `Lý do: ${details.reasonCode === "invalid_code" ? "Mã OTP không hợp lệ" : details.reasonCode}`,
    );
  if (typeof details.attemptsLeft === "number")
    lines.push(`Còn lại: ${details.attemptsLeft} lần thử`);
  if (typeof details.channel === "string")
    lines.push(`Kênh: ${details.channel}`);
  return lines;
}

function sameQuery(
  left: EventExplorerQuery | null,
  right: EventExplorerQuery,
): boolean {
  if (!left) return false;
  return [
    "searchType",
    "search",
    "category",
    "eventType",
    "status",
    "from",
    "to",
  ].every(
    (key) =>
      left[key as keyof EventExplorerQuery] ===
      right[key as keyof EventExplorerQuery],
  );
}

function EventDetailDrawer({
  item,
  loading,
  error,
  onClose,
}: {
  item: EventExplorerDisplayItem;
  loading: boolean;
  error: string;
  onClose: () => void;
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex justify-end bg-black/40"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside
        className={`${subtle} h-full w-full max-w-120 overflow-y-auto p-4 shadow-2xl`}
        role="dialog"
        aria-modal="true"
        aria-label="Chi tiết event"
      >
        <div className="flex items-start justify-between gap-3 border-b border-(--admin-border) pb-3">
          <div className="min-w-0">
            <p
              className={`${mono} text-[10px] uppercase tracking-[0.08em] text-(--admin-text-muted)`}
            >
              Chi tiết event
            </p>
            <h2 className="mt-1 truncate text-[16px] font-semibold text-(--admin-text)">
              {eventLabel(item)}
            </h2>
          </div>
          <button
            className={`${surface} grid h-8 w-8 shrink-0 place-items-center rounded-sm text-(--admin-text-secondary) hover:bg-(--admin-surface-hover)`}
            type="button"
            aria-label="Đóng chi tiết"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        <dl className="mt-4 grid gap-2 text-[12px]">
          <div className="flex justify-between gap-4">
            <dt className="text-(--admin-text-muted)">Thời điểm</dt>
            <dd className={`${mono} text-right text-(--admin-text)`}>
              {formatDateTime(item.occurredAt)}
            </dd>
          </div>
          {item.transactionId && (
            <div className="flex justify-between gap-4">
              <dt className="text-(--admin-text-muted)">Mã giao dịch</dt>
              <dd className={`${mono} text-right text-(--admin-primary)`}>
                {item.transactionId}
              </dd>
            </div>
          )}
          {item.loanId && (
            <div className="flex justify-between gap-4">
              <dt className="text-(--admin-text-muted)">Khoản vay</dt>
              <dd className={`${mono} text-right text-(--admin-primary)`}>
                {item.loanId}
              </dd>
            </div>
          )}
        </dl>
        {businessDetails(item).length > 0 && (
          <div className="mt-4 border-t border-(--admin-border) pt-4">
            <h3 className="text-[13px] font-semibold text-(--admin-text)">
              Thông tin giao dịch
            </h3>
            <div className="mt-2 grid gap-1 text-[12px] text-(--admin-text-secondary)">
              {businessDetails(item).map((line) => (
                <span key={line}>{line}</span>
              ))}
            </div>
          </div>
        )}
        {item.accounts && item.accounts.length > 0 && (
          <div className="mt-4 border-t border-(--admin-border) pt-4">
            <h3 className="text-[13px] font-semibold text-(--admin-text)">
              Tài khoản liên quan
            </h3>
            <div className="mt-2 grid gap-1">
              {item.accounts.map((account) => (
                <button
                  key={`${account.accountNo}:${account.role}`}
                  className="flex items-center justify-between rounded-sm px-2 py-1.5 text-left text-[12px] text-(--admin-primary) hover:bg-(--admin-surface-hover)"
                  type="button"
                  onClick={() =>
                    navigate(
                      `${routes.auditAccounts}?account=${encodeURIComponent(account.accountNo)}`,
                    )
                  }
                >
                  <span className="min-w-0 truncate">
                    <strong className={`${mono} block`}>
                      {account.role ? `${account.role}: ` : ""}
                      {account.accountNo}
                    </strong>
                    {account.customerName && (
                      <small className="block text-(--admin-text-muted)">
                        {account.customerName}
                      </small>
                    )}
                  </span>
                  <FontAwesomeIcon
                    icon={faArrowRight}
                    className="text-[10px]"
                    aria-hidden="true"
                  />
                </button>
              ))}
            </div>
          </div>
        )}
        <details className="mt-4 border-t border-(--admin-border) pt-4">
          <summary className="cursor-pointer text-[13px] font-semibold text-(--admin-text)">
            Thông tin kỹ thuật
          </summary>
          {loading ? (
            <p className="mt-2 text-[12px] text-(--admin-text-muted)">
              Đang tải chi tiết...
            </p>
          ) : error ? (
            <p className="mt-2 text-[12px] text-(--admin-danger)" role="alert">
              {error}
            </p>
          ) : (
            <div className="mt-2 grid gap-2">
              <dl className="grid gap-1 text-[11px] text-(--admin-text-muted)">
                <div className="flex justify-between gap-4">
                  <dt>Event ID</dt>
                  <dd className={`${mono} max-w-65 break-all text-right`}>
                    {item.eventId}
                  </dd>
                </div>
                {"eventType" in item && (
                  <div className="flex justify-between gap-4">
                    <dt>Event type</dt>
                    <dd className={`${mono} text-right`}>{item.eventType}</dd>
                  </div>
                )}
                {"sourceStream" in item && (
                  <div className="flex justify-between gap-4">
                    <dt>Stream</dt>
                    <dd className={`${mono} max-w-65 break-all text-right`}>
                      {item.sourceStream}
                    </dd>
                  </div>
                )}
                {"sourceRevision" in item && (
                  <div className="flex justify-between gap-4">
                    <dt>Revision</dt>
                    <dd className={`${mono} text-right`}>
                      {item.sourceRevision}
                    </dd>
                  </div>
                )}
              </dl>
              <pre
                className={`${mono} overflow-x-auto rounded-sm bg-(--admin-background) p-3 text-[10px] leading-4 text-(--admin-text-secondary)`}
              >
                {JSON.stringify(
                  "details" in item ? item.details || {} : {},
                  null,
                  2,
                )}
              </pre>
            </div>
          )}
        </details>
      </aside>
    </div>
  );
}

export function EventExplorerPage() {
  const { theme } = useAdminTheme();
  const [query, setQuery] = useState<EventExplorerQuery>(initialQuery);
  const [appliedQuery, setAppliedQuery] = useState<EventExplorerQuery | null>(
    null,
  );
  const [result, setResult] = useState<EventExplorerResponse | null>(null);
  const [page, setPage] = useState(1);
  const [pageCursors, setPageCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const [selectedItem, setSelectedItem] =
    useState<EventExplorerDisplayItem | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const searchRequestRef = useRef(0);
  const searchAbortRef = useRef<AbortController | null>(null);
  const detailRequestRef = useRef(0);
  const detailAbortRef = useRef<AbortController | null>(null);

  const draftChanged = !sameQuery(appliedQuery, query);
  const neutralError =
    error === "Dữ liệu event không hợp lệ" ||
    error === "Không tìm thấy dữ liệu phù hợp." ||
    error === "Không thể tải event.";

  async function loadPage(
    nextQuery: EventExplorerQuery,
    cursor: string | undefined,
    targetPage: number,
  ) {
    const requestId = searchRequestRef.current + 1;
    searchRequestRef.current = requestId;
    searchAbortRef.current?.abort();
    const controller = new AbortController();
    searchAbortRef.current = controller;
    setLoading(true);
    setError("");
    try {
      const response = await getEventExplorer({
        ...nextQuery,
        cursor,
        limit: 20,
        signal: controller.signal,
      });
      if (searchRequestRef.current !== requestId) return;
      setResult(response);
      setAppliedQuery(nextQuery);
      setPage(targetPage);
      setPageCursors((current) => {
        const next = current.slice(0, targetPage);
        next[targetPage - 1] = cursor;
        if (response.nextCursor) next[targetPage] = response.nextCursor;
        return next;
      });
    } catch (reason) {
      if (searchRequestRef.current !== requestId) return;
      setResult(null);
      setError(
        reason instanceof EventExplorerApiError
          ? reason.message
          : "Không thể tải event.",
      );
    } finally {
      if (searchRequestRef.current === requestId) setLoading(false);
    }
  }

  function submit(event?: FormEvent) {
    event?.preventDefault();
    setPageCursors([undefined]);
    void loadPage({ ...query }, undefined, 1);
  }

  function reset() {
    searchRequestRef.current += 1;
    searchAbortRef.current?.abort();
    searchAbortRef.current = null;
    setQuery(initialQuery);
    setAppliedQuery(null);
    setResult(null);
    setPage(1);
    setPageCursors([undefined]);
    setError("");
    setLoading(false);
  }

  async function openDetails(item: EventExplorerListItem) {
    const requestId = detailRequestRef.current + 1;
    detailRequestRef.current = requestId;
    detailAbortRef.current?.abort();
    const controller = new AbortController();
    detailAbortRef.current = controller;
    setSelectedItem(item);
    setDetailLoading(true);
    setDetailError("");
    try {
      const detail = await getEventExplorerDetail({
        ...item,
        signal: controller.signal,
      });
      if (detailRequestRef.current === requestId) setSelectedItem(detail);
    } catch (reason) {
      if (detailRequestRef.current === requestId)
        setDetailError(
          reason instanceof EventExplorerApiError
            ? reason.message
            : "Không thể tải chi tiết event.",
        );
    } finally {
      if (detailRequestRef.current === requestId) setDetailLoading(false);
    }
  }

  return (
    <div className="grid gap-3">
      <section className={`${subtle} rounded-(--admin-radius-md) p-4`}>
        <div className="flex items-center gap-3">
          <div className="grid h-9 w-9 shrink-0 place-items-center rounded-(--admin-radius-md) bg-(--admin-primary-soft) text-(--admin-primary)">
            <FontAwesomeIcon icon={faCode} aria-hidden="true" />
          </div>
          <h1 className="text-[18px] font-bold uppercase tracking-[-0.02em] text-(--admin-text)">
            Event Explorer
          </h1>
        </div>
        <form
          className="mt-4 grid w-full gap-2 lg:grid-cols-[140px_minmax(280px,2fr)_minmax(150px,1fr)_minmax(260px,1.5fr)_auto]"
          onSubmit={submit}
        >
          <label className="sr-only" htmlFor="event-search-type">
            Kiểu tìm kiếm
          </label>
          <select
            id="event-search-type"
            className="h-10 min-w-0 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] font-medium text-(--admin-text) outline-none transition-colors focus:border-(--admin-primary)"
            value={query.searchType}
            onChange={(event) =>
              setQuery({
                ...query,
                searchType: event.target
                  .value as EventExplorerQuery["searchType"],
              })
            }
          >
            {(
              Object.keys(
                searchTypeLabels,
              ) as EventExplorerQuery["searchType"][]
            ).map((key) => (
              <option key={key} value={key}>
                {searchTypeLabels[key]}
              </option>
            ))}
          </select>
          <label className="flex h-10 min-w-0 items-center gap-2 rounded-sm border border-(--admin-border) bg-(--admin-background) px-3 transition-colors focus-within:border-(--admin-primary) focus-within:shadow-(--admin-focus-ring)">
            <FontAwesomeIcon
              icon={faMagnifyingGlass}
              className="shrink-0 text-[12px] text-(--admin-text-muted)"
              aria-hidden="true"
            />
            <span className="sr-only">Từ khóa</span>
            <input
              className="min-w-0 flex-1 bg-transparent text-[12px] text-(--admin-text) outline-none placeholder:text-(--admin-text-muted)"
              placeholder="Nhập mã giao dịch hoặc số tài khoản..."
              value={query.search}
              onChange={(event) =>
                setQuery({ ...query, search: event.target.value })
              }
            />
          </label>
          <select
            className="h-10 min-w-0 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none transition-colors focus:border-(--admin-primary)"
            aria-label="Category"
            value={query.category}
            onChange={(event) =>
              setQuery({
                ...query,
                category: event.target.value as EventExplorerCategory,
              })
            }
          >
            {(Object.keys(categoryLabels) as EventExplorerCategory[]).map(
              (key) => (
                <option key={key} value={key}>
                  {categoryLabels[key]}
                </option>
              ),
            )}
          </select>
          <div className="flex min-w-0 gap-2">
            <label className="block h-10 min-w-0 flex-1">
              <span className="sr-only">Từ ngày</span>
              <input
                className="h-full w-full rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none transition-colors focus:border-(--admin-primary)"
                style={{ colorScheme: theme }}
                type="date"
                aria-label="Từ ngày"
                value={query.from}
                onChange={(event) =>
                  setQuery({ ...query, from: event.target.value })
                }
              />
            </label>
            <label className="block h-10 min-w-0 flex-1">
              <span className="sr-only">Đến ngày</span>
              <input
                className="h-full w-full rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none transition-colors focus:border-(--admin-primary)"
                style={{ colorScheme: theme }}
                type="date"
                aria-label="Đến ngày"
                value={query.to}
                onChange={(event) =>
                  setQuery({ ...query, to: event.target.value })
                }
              />
            </label>
          </div>
          <div className="flex shrink-0 gap-2">
            <button
              className="h-10 whitespace-nowrap rounded-sm border border-(--admin-primary-strong) bg-(--admin-primary) px-4 text-[12px] font-semibold text-white transition-colors hover:bg-(--admin-primary-strong) disabled:cursor-not-allowed disabled:opacity-50"
              type="submit"
              disabled={loading}
            >
              {loading ? "Đang tải..." : "Tìm kiếm"}
            </button>
            <button
              className={`${surface} h-10 whitespace-nowrap rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) transition-colors hover:border-(--admin-primary) hover:bg-(--admin-surface-hover)`}
              type="button"
              onClick={reset}
            >
              Đặt lại
            </button>
          </div>
        </form>
        {error && (
          <div
            className={`mt-3 flex items-center gap-2 rounded-sm border px-3 py-2 text-[11px] ${neutralError ? "border-(--admin-border) bg-(--admin-surface) text-(--admin-text-muted)" : "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)"}`}
            role={neutralError ? "status" : "alert"}
          >
            <FontAwesomeIcon icon={faCircleExclamation} aria-hidden="true" />
            {error}
          </div>
        )}
      </section>

      <section
        className={`${surface} rounded-(--admin-radius-md) border border-(--admin-border) p-3`}
      >
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-[16px] font-semibold uppercase text-(--admin-text)">
              Kết quả tra cứu
            </h2>
            <p className="mt-0.5 text-[11px] text-(--admin-text-muted)">
              {result
                ? `${result.items.length} event · đã quét ${result.scanned}`
                : "Chưa thực hiện tra cứu"}
            </p>
          </div>
          <button
            className={`${surface} flex h-8 items-center gap-2 rounded-sm border border-(--admin-border) px-2.5 text-[12px] text-(--admin-text) transition-colors hover:border-(--admin-primary) hover:bg-(--admin-surface-hover)`}
            type="button"
            onClick={() => submit()}
            disabled={loading}
          >
            <FontAwesomeIcon
              icon={faRotate}
              className={loading ? "animate-spin" : ""}
              aria-hidden="true"
            />
            Làm mới
          </button>
        </div>
        {!result && !loading && !error && (
          <div className="grid min-h-48 place-items-center text-center text-[12px] text-(--admin-text-muted)">
            Nhập điều kiện và chọn “Tìm kiếm” để tra cứu event.
          </div>
        )}
        {result && result.items.length === 0 && (
          <div className="grid min-h-48 place-items-center text-center text-[12px] text-(--admin-text-muted)">
            {result.scanComplete
              ? "Không tìm thấy event phù hợp."
              : "Chưa thấy kết quả trong phạm vi vừa quét. Hãy bấm Trang sau để tiếp tục."}
          </div>
        )}
        {result && result.items.length > 0 && (
          <div className="w-full overflow-x-auto">
            <table className="w-full min-w-250 text-left">
              <thead>
                <tr className="h-8 border-b border-(--admin-border) text-[10px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)">
                  <th className="px-2">Thời điểm</th>
                  <th className="px-2">Sự kiện</th>
                  <th className="px-2">Nhóm</th>
                  <th className="px-2">Mã giao dịch</th>
                  <th className="px-2">Tài khoản liên quan</th>
                </tr>
              </thead>
              <tbody>
                {result.items.map((item) => (
                  <tr
                    key={item.eventId}
                    className="h-11 cursor-pointer border-b border-(--admin-border-subtle) transition-colors hover:bg-(--admin-surface-hover)"
                    role="button"
                    tabIndex={0}
                    onClick={() => void openDetails(item)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ")
                        void openDetails(item);
                    }}
                  >
                    <td
                      className={`${mono} whitespace-nowrap px-2 text-[10px] text-(--admin-text-muted)`}
                    >
                      {formatDateTime(item.occurredAt)}
                    </td>
                    <td className="max-w-60 truncate px-2 text-[12px] font-semibold text-(--admin-text)">
                      {eventLabel(item)}
                    </td>
                    <td
                      className={`${mono} px-2 text-[10px] uppercase text-(--admin-text-secondary)`}
                    >
                      {categoryLabels[item.category]}
                    </td>
                    <td
                      className={`${mono} px-2 text-[11px] text-(--admin-primary)`}
                    >
                      {item.transactionId || "—"}
                    </td>
                    <td
                      className={`${mono} max-w-55 truncate px-2 text-[10px] text-(--admin-text-muted)`}
                    >
                      {item.accounts
                        ?.map(
                          (account) =>
                            `${account.role ? `${account.role}: ` : ""}${account.accountNo}`,
                        )
                        .join(", ") || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {result && (
          <div className="mt-3 flex items-center justify-center gap-3">
            <button
              className={`${surface} h-8 rounded-sm border border-(--admin-border) px-3 text-[12px] text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`}
              type="button"
              disabled={loading || page === 1 || draftChanged}
              onClick={() => {
                const cursor = pageCursors[page - 2];
                if (appliedQuery) void loadPage(appliedQuery, cursor, page - 1);
              }}
            >
              ← Trang trước
            </button>
            <span className={`${mono} text-[11px] text-(--admin-text-muted)`}>
              Trang {page}
            </span>
            <button
              className={`${surface} h-8 rounded-sm border border-(--admin-border) px-3 text-[12px] text-(--admin-text) hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`}
              type="button"
              disabled={
                loading || !result.hasMore || draftChanged || !result.nextCursor
              }
              onClick={() => {
                if (appliedQuery && result.nextCursor)
                  void loadPage(appliedQuery, result.nextCursor, page + 1);
              }}
            >
              Trang sau →
            </button>
          </div>
        )}
      </section>
      {selectedItem && (
        <EventDetailDrawer
          item={selectedItem}
          loading={detailLoading}
          error={detailError}
          onClose={() => {
            detailRequestRef.current += 1;
            detailAbortRef.current?.abort();
            detailAbortRef.current = null;
            setSelectedItem(null);
            setDetailLoading(false);
            setDetailError("");
          }}
        />
      )}
    </div>
  );
}
