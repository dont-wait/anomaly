import { useEffect, useMemo, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowsRotate,
  faCircleExclamation,
  faLock,
  faLockOpen,
  faMagnifyingGlass,
  faTriangleExclamation,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { getMonitorDashboard, updateAccountStatus } from "./api/alertsApi";
import { AccountStatusConfirmation } from "./AccountStatusConfirmation";
import type {
  MonitorDashboardResponse,
  RiskAssessment,
  TimeRange,
} from "@/features/monitor/model";

type RiskFilter = "all" | "WARNING" | "BLOCK";
const ALERTS_PAGE_SIZE = 10;

const surface = "bg-(--admin-surface)";
const subtle = "bg-(--admin-surface-subtle)";
const mono = "font-(family-name:--admin-font-mono)";

const levelLabel: Record<string, string> = {
  WARNING: "Cảnh báo",
  BLOCK: "Chặn",
};
const statusLabel: Record<string, string> = {
  awaiting_otp: "Chờ OTP",
  success: "Đã hoàn tất",
  failed: "Bị từ chối",
  cancelled: "Đã huỷ",
  expired: "Hết hạn OTP",
};

function money(value: number, currency: string) {
  return `${value.toLocaleString("vi-VN")} ${currency || "VND"}`;
}

export function AlertsPage() {
  const [range, setRange] = useState<TimeRange>("7d");
  const [level, setLevel] = useState<RiskFilter>("all");
  const [status, setStatus] = useState("all");
  const [query, setQuery] = useState("");
  const [response, setResponse] = useState<MonitorDashboardResponse | null>(
    null,
  );
  const [selected, setSelected] = useState<RiskAssessment | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [accountStatuses, setAccountStatuses] = useState<
    Record<string, "active" | "locked">
  >({});
  const sourceStatus =
    accountStatuses[selected?.transaction?.sourceAccountId || ""] ??
    selected?.transaction?.sourceAccountStatus;
  const destinationStatus =
    accountStatuses[selected?.transaction?.destinationAccountId || ""] ??
    selected?.transaction?.destinationAccountStatus;
  const [pendingAction, setPendingAction] = useState<{
    accountId: string;
    accountNo?: string;
    status: "active" | "locked";
  } | null>(null);

  function resetFilters() {
    setRange("7d");
    setLevel("all");
    setStatus("all");
    setQuery("");
    setPage(1);
  }

  const refresh = async () => {
    setBusy(true);
    setError(null);
    try {
      setResponse(await getMonitorDashboard(range));
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Không thể tải cảnh báo.",
      );
    } finally {
      setBusy(false);
    }
  };

  useEffect(() => {
    let active = true;
    getMonitorDashboard(range)
      .then((nextResponse) => {
        if (active) setResponse(nextResponse);
      })
      .catch((cause: unknown) => {
        if (active)
          setError(
            cause instanceof Error ? cause.message : "Không thể tải cảnh báo.",
          );
      })
      .finally(() => {
        if (active) setBusy(false);
      });
    return () => {
      active = false;
    };
  }, [range]);

  const alerts = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return (response?.snapshot?.recentAssessments ?? [])
      .filter(
        (item) => item.riskLevel === "WARNING" || item.riskLevel === "BLOCK",
      )
      .filter((item) => level === "all" || item.riskLevel === level)
      .filter((item) => status === "all" || item.transaction?.status === status)
      .filter(
        (item) =>
          !normalized ||
          [
            item.transactionId,
            item.transaction?.sourceAccountNo,
            item.transaction?.destinationAccountNo,
          ].some((value) => value?.toLowerCase().includes(normalized)),
      )
      .sort(
        (left, right) =>
          Date.parse(right.receivedAt) - Date.parse(left.receivedAt),
      );
  }, [level, query, response, status]);

  const totalPages = Math.max(1, Math.ceil(alerts.length / ALERTS_PAGE_SIZE));
  const currentPage = Math.min(page, totalPages);
  const visibleAlerts = alerts.slice(
    (currentPage - 1) * ALERTS_PAGE_SIZE,
    currentPage * ALERTS_PAGE_SIZE,
  );

  const changeAccountStatus = (
    accountId: string | undefined,
    next: "active" | "locked",
    accountNo?: string,
  ) => {
    if (!accountId) return;
    setActionMessage(null);
    setPendingAction({ accountId, accountNo, status: next });
  };

  return (
    <div className="grid gap-3" aria-label="Cảnh báo rủi ro">
      <section className={`${subtle} rounded-(--admin-radius-md) p-4`}>
        <div className="flex items-center gap-3">
          <div className="grid h-9 w-9 shrink-0 place-items-center rounded-(--admin-radius-md) bg-(--admin-danger-soft) text-(--admin-danger)">
            <FontAwesomeIcon icon={faTriangleExclamation} aria-hidden="true" />
          </div>
          <h1 className="text-[18px] font-bold uppercase tracking-[-0.02em] text-(--admin-text)">
            Cảnh báo
          </h1>
        </div>
        <form
          className="mt-4 grid w-full gap-2 lg:grid-cols-[150px_minmax(280px,2fr)_150px_160px_auto]"
          onSubmit={(event) => event.preventDefault()}
        >
          <label className="sr-only" htmlFor="alert-range">
            Khoảng thời gian
          </label>
          <select
            id="alert-range"
            value={range}
            onChange={(event) => {
              setRange(event.target.value as TimeRange);
              setPage(1);
            }}
            className="h-10 min-w-0 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] font-medium text-(--admin-text) outline-none transition-colors focus:border-(--admin-primary)"
          >
            <option value="24h">24 giờ gần nhất</option>
            <option value="7d">7 ngày gần nhất</option>
            <option value="30d">30 ngày gần nhất</option>
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
              value={query}
              onChange={(event) => {
                setQuery(event.target.value);
                setPage(1);
              }}
            />
          </label>
          <label className="sr-only" htmlFor="alert-level">
            Mức độ
          </label>
          <select
            id="alert-level"
            className="h-10 min-w-0 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none transition-colors focus:border-(--admin-primary)"
            value={level}
            onChange={(event) => {
              setLevel(event.target.value as RiskFilter);
              setPage(1);
            }}
          >
            <option value="all">Tất cả mức độ</option>
            <option value="WARNING">Cảnh báo</option>
            <option value="BLOCK">Chặn</option>
          </select>
          <label className="sr-only" htmlFor="alert-status">
            Trạng thái giao dịch
          </label>
          <select
            id="alert-status"
            className="h-10 min-w-0 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none transition-colors focus:border-(--admin-primary)"
            value={status}
            onChange={(event) => {
              setStatus(event.target.value);
              setPage(1);
            }}
          >
            <option value="all">Tất cả trạng thái</option>
            <option value="awaiting_otp">Chờ OTP</option>
            <option value="success">Đã hoàn tất</option>
            <option value="failed">Bị từ chối</option>
            <option value="cancelled">Đã huỷ</option>
          </select>
          <div className="flex shrink-0 gap-2">
            <button
              className="h-10 whitespace-nowrap rounded-sm border border-(--admin-primary-strong) bg-(--admin-primary) px-4 text-[12px] font-semibold text-white transition-colors hover:bg-(--admin-primary-strong) disabled:cursor-not-allowed disabled:opacity-50"
              type="submit"
              disabled={busy}
            >
              {busy ? "Đang tải..." : "Tìm kiếm"}
            </button>
            <button
              className={`${surface} h-10 whitespace-nowrap rounded-sm border border-(--admin-border) px-3 text-[12px] font-semibold text-(--admin-text) transition-colors hover:border-(--admin-primary) hover:bg-(--admin-surface-hover)`}
              type="button"
              onClick={resetFilters}
            >
              Đặt lại
            </button>
          </div>
        </form>
        {error && (
          <div
            className="mt-3 flex items-center gap-2 rounded-sm border border-(--admin-danger) bg-(--admin-danger-soft) px-3 py-2 text-[11px] text-(--admin-danger)"
            role="alert"
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
              Kết quả cảnh báo
            </h2>
            <p className="mt-0.5 text-[11px] text-(--admin-text-muted)">
              {response
                ? `${alerts.length} cảnh báo · Trang ${currentPage}/${totalPages} · mới nhất trước`
                : "Chưa tải dữ liệu"}
            </p>
          </div>
          <button
            className={`${surface} flex h-8 items-center gap-2 rounded-sm border border-(--admin-border) px-2.5 text-[12px] text-(--admin-text) transition-colors hover:border-(--admin-primary) hover:bg-(--admin-surface-hover)`}
            type="button"
            onClick={() => void refresh()}
            disabled={busy}
          >
            <FontAwesomeIcon
              icon={faArrowsRotate}
              className={busy ? "animate-spin" : ""}
              aria-hidden="true"
            />
            Làm mới
          </button>
        </div>
        <div className="w-full overflow-x-auto">
          <table className="w-full min-w-280 text-left">
            <thead>
              <tr className="h-8 border-b border-(--admin-border) text-[10px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)">
                <th className="px-2">Thời điểm</th>
                <th className="px-2">Mức độ</th>
                <th className="px-2">Mã giao dịch</th>
                <th className="px-2">Tài khoản nguồn</th>
                <th className="px-2">Tài khoản đích</th>
                <th className="px-2">Điểm rủi ro</th>
              </tr>
            </thead>
            <tbody>
              {visibleAlerts.map((item) => (
                <tr
                  key={item.assessmentId}
                  className="h-11 cursor-pointer border-b border-(--admin-border-subtle) transition-colors hover:bg-(--admin-surface-hover)"
                  role="button"
                  tabIndex={0}
                  onClick={() => setSelected(item)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ")
                      setSelected(item);
                  }}
                >
                  <td
                    className={`${mono} whitespace-nowrap px-2 text-[10px] text-(--admin-text-muted)`}
                  >
                    {new Date(item.receivedAt).toLocaleString("vi-VN")}
                  </td>
                  <td className="px-2">
                    <span
                      className={`inline-flex items-center gap-1.5 text-[11px] font-semibold ${item.riskLevel === "BLOCK" ? "text-(--admin-danger)" : "text-[#e1b86b]"}`}
                    >
                      <FontAwesomeIcon
                        icon={
                          item.riskLevel === "BLOCK"
                            ? faCircleExclamation
                            : faTriangleExclamation
                        }
                      />
                      {levelLabel[item.riskLevel]}
                    </span>
                  </td>
                  <td
                    className={`${mono} px-2 text-[11px] text-(--admin-primary)`}
                  >
                    {item.transactionId}
                  </td>
                  <td
                    className={`${mono} px-2 text-[10px] text-(--admin-text-muted)`}
                  >
                    {item.transaction?.sourceAccountNo || "—"}
                  </td>
                  <td
                    className={`${mono} px-2 text-[10px] text-(--admin-text-muted)`}
                  >
                    {item.transaction?.destinationAccountNo || "—"}
                  </td>
                  <td
                    className={`${mono} px-2 text-[11px] text-(--admin-text)`}
                  >
                    {item.riskScore.toFixed(4)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        {!alerts.length && (
          <div className="grid min-h-48 place-items-center text-center text-[12px] text-(--admin-text-muted)">
            Không có cảnh báo phù hợp.
          </div>
        )}
        {alerts.length > 0 && (
          <div className="mt-3 flex items-center justify-center gap-3 pt-1">
            <button
              className={`${surface} h-8 rounded-sm border border-(--admin-border) px-3 text-[12px] text-(--admin-text) transition-colors hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`}
              type="button"
              onClick={() => setPage((value) => Math.max(1, value - 1))}
              disabled={currentPage === 1}
            >
              ← Trang trước
            </button>
            <span
              className={`${mono} min-w-18 text-center text-[11px] text-(--admin-text-muted)`}
            >
              Trang {currentPage}/{totalPages}
            </span>
            <button
              className={`${surface} h-8 rounded-sm border border-(--admin-border) px-3 text-[12px] text-(--admin-text) transition-colors hover:border-(--admin-primary) disabled:cursor-not-allowed disabled:opacity-40`}
              type="button"
              onClick={() =>
                setPage((value) => Math.min(totalPages, value + 1))
              }
              disabled={currentPage === totalPages}
            >
              Trang sau →
            </button>
          </div>
        )}
      </section>

      {actionMessage && (
        <div
          className="rounded-sm border border-(--admin-border) bg-(--admin-surface) px-3 py-2 text-[11px] text-(--admin-text)"
          role="status"
        >
          {actionMessage}
        </div>
      )}
      {selected && (
        <div
          className="fixed inset-0 z-50 flex justify-end bg-black/40"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setSelected(null);
          }}
        >
          <aside
            className={`${subtle} h-full w-full max-w-120 overflow-y-auto p-4 shadow-2xl`}
            role="dialog"
            aria-modal="true"
            aria-label="Chi tiết cảnh báo"
          >
            <div className="flex items-start justify-between gap-3 border-b border-(--admin-border) pb-3">
              <div className="min-w-0">
                <p
                  className={`${mono} text-[10px] uppercase tracking-[0.08em] text-(--admin-text-muted)`}
                >
                  Chi tiết cảnh báo
                </p>
                <h2 className="mt-1 truncate text-[16px] font-semibold text-(--admin-text)">
                  {levelLabel[selected.riskLevel]} · {selected.transactionId}
                </h2>
              </div>
              <button
                className={`${surface} grid h-8 w-8 shrink-0 place-items-center rounded-sm text-(--admin-text-secondary) hover:bg-(--admin-surface-hover)`}
                type="button"
                aria-label="Đóng chi tiết"
                onClick={() => setSelected(null)}
              >
                <FontAwesomeIcon icon={faXmark} />
              </button>
            </div>
            <dl className="mt-4 grid gap-2 text-[12px]">
              <div className="flex justify-between gap-4">
                <dt className="text-(--admin-text-muted)">Thời điểm</dt>
                <dd className={`${mono} text-right text-(--admin-text)`}>
                  {new Date(selected.receivedAt).toLocaleString("vi-VN")}
                </dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="text-(--admin-text-muted)">Mã giao dịch</dt>
                <dd className={`${mono} text-right text-(--admin-primary)`}>
                  {selected.transactionId}
                </dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="text-(--admin-text-muted)">Risk score</dt>
                <dd className={`${mono} text-right text-(--admin-text)`}>
                  {selected.riskScore.toFixed(6)}
                </dd>
              </div>
              <div className="flex justify-between gap-4">
                <dt className="text-(--admin-text-muted)">Decision</dt>
                <dd className="text-right text-(--admin-text)">
                  {selected.decision}
                </dd>
              </div>
            </dl>
            <div className="mt-4 border-t border-(--admin-border) pt-4">
              <h3 className="text-[13px] font-semibold text-(--admin-text)">
                Thông tin giao dịch
              </h3>
              <dl className="mt-2 grid gap-2 text-[12px]">
                <div className="flex justify-between gap-4">
                  <dt className="text-(--admin-text-muted)">Tài khoản nguồn</dt>
                  <dd className="text-right">
                    <span className="block font-semibold">
                      {selected.transaction?.sourceAccountName || "—"}
                    </span>
                    <span className={`${mono} text-(--admin-primary)`}>
                      {selected.transaction?.sourceAccountNo || "—"}
                    </span>
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-(--admin-text-muted)">Tài khoản đích</dt>
                  <dd className="text-right">
                    <span className="block font-semibold">
                      {selected.transaction?.destinationName || "—"}
                    </span>
                    <span className={`${mono} text-(--admin-primary)`}>
                      {selected.transaction?.destinationAccountNo || "—"}
                    </span>
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-(--admin-text-muted)">Số tiền</dt>
                  <dd className={`${mono} text-right`}>
                    {selected.transaction
                      ? money(
                          selected.transaction.amount,
                          selected.transaction.currency,
                        )
                      : "—"}
                  </dd>
                </div>
                <div className="flex justify-between gap-4">
                  <dt className="text-(--admin-text-muted)">Trạng thái</dt>
                  <dd className="text-right">
                    {statusLabel[selected.transaction?.status || ""] ||
                      "Chưa xác định"}
                  </dd>
                </div>
              </dl>
            </div>
            <div className="mt-4 border-t border-(--admin-border) pt-4">
              <h3 className="text-[13px] font-semibold text-(--admin-text)">
                Quản lý tài khoản
              </h3>
              <div className="mt-3 grid gap-3">
                <div
                  className={`${surface} rounded-sm border border-(--admin-border) p-3`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p
                        className={`${mono} text-[10px] uppercase tracking-[0.06em] text-(--admin-text-muted)`}
                      >
                        Tài khoản nguồn
                      </p>
                      <p className="mt-1 truncate text-[12px] font-semibold text-(--admin-text)">
                        {selected.transaction?.sourceAccountName ||
                          "Chưa xác định"}
                      </p>
                      <p
                        className={`${mono} mt-0.5 truncate text-[11px] text-(--admin-primary)`}
                      >
                        {selected.transaction?.sourceAccountNo || "—"}
                      </p>
                    </div>
                    <FontAwesomeIcon
                      icon={faLock}
                      className="mt-1 text-(--admin-text-muted)"
                      aria-hidden="true"
                    />
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-2">
                    <button
                      type="button"
                      disabled={
                        busy ||
                        !selected.transaction?.sourceAccountId ||
                        sourceStatus === "locked"
                      }
                      onClick={() =>
                        void changeAccountStatus(
                          selected.transaction?.sourceAccountId,
                          "locked",
                          selected.transaction?.sourceAccountNo,
                        )
                      }
                      className="flex h-9 w-full items-center justify-center gap-2 rounded-sm bg-(--admin-danger) px-2 text-[12px] font-semibold text-white transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <FontAwesomeIcon icon={faLock} aria-hidden="true" /> Khóa
                    </button>
                    <button
                      type="button"
                      disabled={
                        busy ||
                        !selected.transaction?.sourceAccountId ||
                        sourceStatus === "active"
                      }
                      onClick={() =>
                        void changeAccountStatus(
                          selected.transaction?.sourceAccountId,
                          "active",
                          selected.transaction?.sourceAccountNo,
                        )
                      }
                      className={`${surface} flex h-9 w-full items-center justify-center gap-2 rounded-sm border border-(--admin-border) px-2 text-[12px] font-semibold text-(--admin-text) transition-colors hover:border-(--admin-primary) hover:bg-(--admin-surface-hover) disabled:cursor-not-allowed disabled:opacity-50`}
                    >
                      <FontAwesomeIcon icon={faLockOpen} aria-hidden="true" />{" "}
                      Mở khóa
                    </button>
                  </div>
                </div>
                <div
                  className={`${surface} rounded-sm border border-(--admin-border) p-3`}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p
                        className={`${mono} text-[10px] uppercase tracking-[0.06em] text-(--admin-text-muted)`}
                      >
                        Tài khoản đích
                      </p>
                      <p className="mt-1 truncate text-[12px] font-semibold text-(--admin-text)">
                        {selected.transaction?.destinationName ||
                          "Chưa xác định"}
                      </p>
                      <p
                        className={`${mono} mt-0.5 truncate text-[11px] text-(--admin-primary)`}
                      >
                        {selected.transaction?.destinationAccountNo || "—"}
                      </p>
                    </div>
                    <FontAwesomeIcon
                      icon={faLock}
                      className="mt-1 text-(--admin-text-muted)"
                      aria-hidden="true"
                    />
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-2">
                    <button
                      type="button"
                      disabled={
                        busy ||
                        !selected.transaction?.destinationAccountId ||
                        destinationStatus === "locked"
                      }
                      onClick={() =>
                        void changeAccountStatus(
                          selected.transaction?.destinationAccountId,
                          "locked",
                          selected.transaction?.destinationAccountNo,
                        )
                      }
                      className="flex h-9 w-full items-center justify-center gap-2 rounded-sm bg-(--admin-danger) px-2 text-[12px] font-semibold text-white transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <FontAwesomeIcon icon={faLock} aria-hidden="true" /> Khóa
                    </button>
                    <button
                      type="button"
                      disabled={
                        busy ||
                        !selected.transaction?.destinationAccountId ||
                        destinationStatus === "active"
                      }
                      onClick={() =>
                        void changeAccountStatus(
                          selected.transaction?.destinationAccountId,
                          "active",
                          selected.transaction?.destinationAccountNo,
                        )
                      }
                      className={`${surface} flex h-9 w-full items-center justify-center gap-2 rounded-sm border border-(--admin-border) px-2 text-[12px] font-semibold text-(--admin-text) transition-colors hover:border-(--admin-primary) hover:bg-(--admin-surface-hover) disabled:cursor-not-allowed disabled:opacity-50`}
                    >
                      <FontAwesomeIcon icon={faLockOpen} aria-hidden="true" />{" "}
                      Mở khóa
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </aside>
        </div>
      )}
      {pendingAction && (
        <AccountStatusConfirmation
          accountNo={pendingAction.accountNo || pendingAction.accountId}
          status={pendingAction.status}
          onCancel={() => setPendingAction(null)}
          onConfirm={async () => {
            await updateAccountStatus(
              pendingAction.accountId,
              pendingAction.status,
              pendingAction.accountNo,
            );
            setAccountStatuses((current) => ({
              ...current,
              [pendingAction.accountId]: pendingAction.status,
            }));
            setActionMessage(
              pendingAction.status === "locked"
                ? "Đã khóa tài khoản."
                : "Đã mở khóa tài khoản.",
            );
            setPendingAction(null);
          }}
        />
      )}
    </div>
  );
}
