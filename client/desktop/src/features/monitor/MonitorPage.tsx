import { useEffect, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowDown,
  faArrowUp,
  faArrowsRotate,
  faCircleExclamation,
  faStream,
} from "@fortawesome/free-solid-svg-icons";
import { navigate, routes } from "@/app/routes";
import { getMonitorDashboard } from "./api/monitorApi";
import { RiskTrendChart } from "./components/RiskTrendChart";
import {
  ranges,
  type AccountRisk,
  type AlertEvent,
  type DashboardSnapshot,
  type Metric,
  type MonitorDashboardResponse,
  type TimeRange,
} from "./mock/monitorData";
const formatVnd = (amount: number) => `${amount.toLocaleString("vi-VN")} ₫`;

const surface = "bg-(--admin-surface)";
const surfaceSubtle = "bg-(--admin-surface-subtle)";
const surfaceHover = "hover:bg-(--admin-surface-hover)";
const mono = "font-(family-name:--admin-font-mono)";

function MetricCard({ metric }: { metric: Metric }) {
  const valueClass =
    metric.tone === "danger"
      ? "text-(--admin-danger)"
      : metric.tone === "primary"
        ? "text-(--admin-primary)"
        : "text-(--admin-text)";
  const changeClass = metric.trend === "up" ? "text-[#ffb4ab]" : "text-(--admin-text-secondary)";

  return (
    <article className={`${surfaceSubtle} flex min-h-23 flex-col justify-between rounded-(--admin-radius-md) p-3`}>
      <div className="flex items-center justify-between gap-2">
        <span className={`${mono} text-[11px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)`}>
          {metric.label}
        </span>
        <FontAwesomeIcon icon={metric.icon} className="shrink-0 text-[13px] text-(--admin-text-muted)" aria-hidden="true" />
      </div>
      <div className="mt-2 flex items-baseline justify-between gap-2">
        <strong className={`${mono} text-[22px] leading-6 font-bold ${valueClass}`}>{metric.value}</strong>
        {metric.change && (
          <span className={`${mono} flex items-center gap-0.5 whitespace-nowrap text-[11px] ${changeClass}`}>
            {metric.trend === "up" ? <FontAwesomeIcon icon={faArrowUp} aria-hidden="true" /> : <FontAwesomeIcon icon={faArrowDown} aria-hidden="true" />}
            {metric.change}
          </span>
        )}
      </div>
      <span className="mt-1 text-[11px] leading-4 text-(--admin-text-muted)">{metric.note}</span>
    </article>
  );
}

function MonitorHeader({ range, onRangeChange, isRefreshing, onRefresh }: { range: TimeRange; onRangeChange: (range: TimeRange) => void; isRefreshing: boolean; onRefresh: () => void }) {
  return (
    <section className={`${surfaceSubtle} flex flex-col justify-between gap-3 rounded-(--admin-radius-md) p-3 md:flex-row md:items-center`}>
      <div className="flex min-w-0 items-start gap-2.5">
        <span className="mt-1 h-7 w-1.5 shrink-0 rounded-sm bg-(--admin-danger)" aria-hidden="true" />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-[18px] leading-6 font-bold tracking-[-0.02em] text-(--admin-text) uppercase">
              Giám sát rủi ro &amp; bất thường hệ thống
            </h1>
            <span className={`${mono} rounded-sm bg-(--admin-surface-hover) px-1.5 py-0.5 text-[10px] tracking-[0.03em] text-(--admin-primary)`}>SYS_METRIC_LIVE</span>
          </div>
          <p className="mt-0.5 text-[13px] leading-5 text-(--admin-text-muted)">Luồng phân tích giao dịch theo thời gian thực và tự động phát hiện gian lận</p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 self-start md:self-auto">
        <div className={`${surface} flex rounded-sm p-0.5`} role="group" aria-label="Khoảng thời gian">
          {ranges.map((item) => (
            <button
              key={item.value}
              className={`rounded-sm px-2.5 py-1 text-[12px] font-semibold transition-colors ${range === item.value ? "bg-(--admin-primary) text-white" : "text-(--admin-text-muted) hover:text-(--admin-text)"}`}
              type="button"
              aria-pressed={range === item.value}
              onClick={() => onRangeChange(item.value)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <div className={`${surface} flex items-center gap-1.5 rounded-sm px-2.5 py-1.5`}>
          <span className="h-2 w-2 animate-pulse rounded-full bg-(--admin-primary)" aria-hidden="true" />
          <span className={`${mono} text-[10px] uppercase tracking-[0.06em] text-(--admin-text-secondary)`}>Realtime 2s</span>
        </div>
        <button className={`${surface} ${surfaceHover} flex h-7 items-center gap-1.5 rounded-sm px-2.5 text-[12px] font-semibold text-(--admin-text) transition-colors`} type="button" onClick={onRefresh} disabled={isRefreshing}>
          <FontAwesomeIcon icon={faArrowsRotate} className={isRefreshing ? "animate-spin" : ""} aria-hidden="true" />
          {isRefreshing ? "Đang cập nhật..." : "Làm mới"}
        </button>
      </div>
    </section>
  );
}

function RiskBreakdown({ snapshot }: { snapshot: DashboardSnapshot }) {
  const totalAlerts = snapshot.severities.reduce((total, item) => total + item.value, 0);
  const severityTone: Record<string, string> = {
    danger: "bg-(--admin-danger)",
    primary: "bg-(--admin-primary)",
    secondary: "bg-[#3c4a5e]",
    muted: "bg-(--admin-surface-hover)",
  };
  const vectorTone: Record<string, string> = {
    primary: "bg-(--admin-primary)",
    secondary: "bg-[#b9c7df]",
    outline: "bg-(--admin-text-muted)",
    muted: "bg-(--admin-surface-hover)",
  };

  return (
    <section className={`${surfaceSubtle} flex flex-col justify-between gap-6 rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex items-center justify-between gap-2 pb-2">
          <h2 className="text-[16px] leading-5 font-semibold text-(--admin-text)">Phân bố mức độ nghiêm trọng</h2>
          <span className={`${mono} text-[11px] text-(--admin-text-muted)`}>{totalAlerts} alerts</span>
        </div>
        <div className="my-2 flex h-3 overflow-hidden rounded-sm bg-(--admin-surface-hover)">
          {snapshot.severities.map((item) => <div key={item.label} className={severityTone[item.tone]} style={{ width: `${(item.value / totalAlerts) * 100}%` }} title={`${item.label}: ${item.value}`} />)}
        </div>
        <div className="grid grid-cols-2 gap-1">
          {snapshot.severities.map((item) => (
            <div key={item.label} className={`${surface} flex items-center justify-between rounded-sm px-2 py-1`}>
              <span className="flex items-center gap-1 text-[11px] font-semibold uppercase text-(--admin-text-muted)"><span className={`h-2 w-2 rounded-sm ${severityTone[item.tone]}`} />{item.label}</span>
              <strong className={`${mono} text-[12px] ${item.tone === "danger" ? "text-(--admin-danger)" : item.tone === "primary" ? "text-(--admin-primary)" : "text-(--admin-text)"}`}>{String(item.value).padStart(2, "0")}</strong>
            </div>
          ))}
        </div>
      </div>
      <div>
        <span className={`${mono} mb-2 block text-[11px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)`}>Cơ chế phát hiện chính</span>
        <div className="grid gap-2">
          {snapshot.detectionVectors.map((item) => (
            <div key={item.label}>
              <div className="mb-0.5 flex items-center justify-between gap-2 text-[12px] text-(--admin-text-secondary)"><span>{item.label}</span><span className={mono}>{item.value}%</span></div>
              <div className={`${surface} h-1.5 overflow-hidden rounded-sm`}><div className={`h-full ${vectorTone[item.tone]}`} style={{ width: `${item.value}%` }} /></div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function RiskAccountsTable({ accounts }: { accounts: AccountRisk[] }) {
  return (
    <section className={`${surfaceSubtle} flex min-h-80 flex-col justify-between rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex flex-wrap items-center justify-between gap-2 pb-2">
          <div className="flex items-center gap-2"><h2 className="text-[16px] leading-5 font-semibold text-(--admin-text) uppercase">Top 5 tài khoản rủi ro cao</h2><span className={`${mono} rounded-sm bg-[#5b2427] px-1.5 py-0.5 text-[10px] text-[#ffb4ab]`}>ACTIVE SURVEILLANCE</span></div>
          <button className="text-[12px] font-semibold text-(--admin-primary) hover:underline" type="button" onClick={() => navigate(routes.riskAccounts)}>Xem tất cả (12)</button>
        </div>
        {accounts.length === 0 ? <div className="grid min-h-48 place-items-center text-[12px] text-(--admin-text-muted)">Chưa có dữ liệu trong khoảng thời gian này.</div> : <div className="w-full overflow-x-auto"><table className="w-full min-w-150 text-left"><thead><tr className={`${surface} h-7 text-[10px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)`}><th className="px-2">Tài khoản</th><th className="px-2">Chủ tài khoản</th><th className="px-2 text-center">Score</th><th className="px-2 text-center">Alerts</th><th className="px-2 text-right">Giá trị rủi ro</th><th className="px-2 text-right">Thao tác</th></tr></thead><tbody>{accounts.map((account) => <tr key={account.accountNo} className={`h-9 transition-colors ${surfaceHover}`}><td className={`${mono} px-2 text-[11px] font-bold text-(--admin-primary)`}>{account.accountNo}</td><td className="max-w-30 truncate px-2 text-[13px] font-medium text-(--admin-text)">{account.customerName}</td><td className="px-2 text-center"><span className={`${mono} rounded-sm px-1.5 py-1 text-[11px] font-bold ${account.score >= 85 ? "bg-[#5b2427] text-[#ffb4ab]" : account.score >= 70 ? "bg-(--admin-primary-soft) text-(--admin-primary)" : "bg-(--admin-surface-hover) text-(--admin-text-secondary)"}`}>{account.score}</span></td><td className={`${mono} px-2 text-center text-[12px] text-(--admin-text)`}>{account.alerts}</td><td className={`${mono} px-2 text-right text-[12px] font-semibold text-(--admin-text)`}>{formatVnd(account.amountAtRisk)}</td><td className="px-2 text-right"><button className={`h-6 whitespace-nowrap rounded-sm px-2 text-[11px] font-semibold ${account.score >= 90 ? "bg-(--admin-danger) text-white hover:bg-[#b91c1c]" : `${surface} ${surfaceHover} text-(--admin-text)`}`} type="button" onClick={() => navigate(routes.alerts)}>Mở điều tra</button></td></tr>)}</tbody></table></div>}
      </div>
      <div className={`${mono} flex flex-wrap items-center justify-between gap-2 pt-2 text-[10px] text-(--admin-text-muted)`}><span>Tổng giá trị kiểm soát: 128.500.000 ₫</span><span>Phân nhóm: ML Ensemble Model v2.8</span></div>
    </section>
  );
}

function RealtimeAlertStream({ events }: { events: AlertEvent[] }) {
  const [isPaused, setIsPaused] = useState(false);
  const severityClass: Record<AlertEvent["severity"], string> = { CRITICAL: "bg-(--admin-danger) text-white", HIGH: "bg-(--admin-primary) text-white", MEDIUM: "bg-[#3c4a5e] text-(--admin-text-secondary)" };

  return (
    <section className={`${surfaceSubtle} flex min-h-80 flex-col justify-between rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex items-center justify-between gap-2 pb-2"><div className="flex items-center gap-1.5"><FontAwesomeIcon icon={faStream} className="text-[14px] text-(--admin-primary)" aria-hidden="true" /><h2 className="text-[16px] leading-5 font-semibold text-(--admin-text) uppercase">Realtime Alert Stream</h2></div><span className={`${mono} rounded-sm ${surface} px-1.5 py-1 text-[10px] text-(--admin-text-muted)`}>INGEST: 14 msg/s</span></div>
        {events.length === 0 ? <div className="grid min-h-48 place-items-center text-[12px] text-(--admin-text-muted)">Không có alert realtime trong khoảng thời gian này.</div> : <div className={`grid gap-1 ${isPaused ? "opacity-60" : ""}`} aria-live="polite">{events.map((event) => <article key={`${event.accountNo}-${event.time}`} className={`${surface} ${surfaceHover} flex items-start justify-between gap-2 rounded-sm p-2 transition-colors`}><div className="min-w-0"><div className="flex items-center gap-1.5"><span className={`${severityClass[event.severity]} rounded-sm px-1 py-0.5 text-[10px] font-bold`}>{event.severity}</span><span className="truncate text-[12px] font-medium text-(--admin-text)">{event.title}</span></div><div className={`${mono} mt-0.5 flex min-w-0 items-center gap-1 text-[10px] text-(--admin-text-muted)`}><span className="font-semibold text-(--admin-primary)">{event.accountNo}</span><span>·</span><span className="truncate">{event.detail}</span></div></div><time className={`${mono} shrink-0 text-[10px] text-(--admin-text-muted)`}>{event.time}</time></article>)}</div>}
      </div>
      <div className={`${mono} flex flex-wrap items-center justify-between gap-2 pt-2 text-[10px] text-(--admin-text-muted)`}><span className="flex items-center gap-1"><span className="h-1.5 w-1.5 rounded-full bg-(--admin-primary)" />Kênh: kafka.prod.risk-engine.events</span><button className="text-(--admin-primary) hover:underline" type="button" onClick={() => setIsPaused((paused) => !paused)}>{isPaused ? "Tiếp tục luồng" : "Tạm dừng luồng"}</button></div>
    </section>
  );
}

function DashboardSkeleton() {
  return (
    <div className="grid gap-1" aria-label="Đang tải dashboard">
      <div className={`${surfaceSubtle} h-21 animate-pulse rounded-(--admin-radius-md)`} />
      <div className="grid grid-cols-2 gap-1 lg:grid-cols-5">
        {Array.from({ length: 5 }, (_, index) => <div key={index} className={`${surfaceSubtle} h-23 animate-pulse rounded-(--admin-radius-md)`} />)}
      </div>
      <div className="grid grid-cols-12 gap-1"><div className={`${surfaceSubtle} col-span-12 h-75 animate-pulse rounded-(--admin-radius-md) xl:col-span-8`} /><div className={`${surfaceSubtle} col-span-12 h-75 animate-pulse rounded-(--admin-radius-md) xl:col-span-4`} /></div>
      <div className="grid grid-cols-12 gap-1"><div className={`${surfaceSubtle} col-span-12 h-80 animate-pulse rounded-(--admin-radius-md) lg:col-span-7`} /><div className={`${surfaceSubtle} col-span-12 h-80 animate-pulse rounded-(--admin-radius-md) lg:col-span-5`} /></div>
    </div>
  );
}

function DashboardError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <section className={`${surfaceSubtle} grid min-h-55 place-items-center rounded-(--admin-radius-md) p-6 text-center`} role="alert">
      <div className="grid max-w-105 gap-2">
        <FontAwesomeIcon icon={faCircleExclamation} className="mx-auto text-2xl text-(--admin-danger)" aria-hidden="true" />
        <strong className="text-[15px] text-(--admin-text)">Không thể tải dữ liệu giám sát</strong>
        <span className="text-[12px] leading-5 text-(--admin-text-muted)">{message}</span>
        <button className="mx-auto mt-1 h-8 rounded-sm bg-(--admin-primary) px-3 text-[12px] font-semibold text-white hover:bg-(--admin-primary-strong)" type="button" onClick={onRetry}>Thử lại</button>
      </div>
    </section>
  );
}

export function MonitorPage() {
  const [range, setRange] = useState<TimeRange>("24h");
  const [response, setResponse] = useState<MonitorDashboardResponse | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [requestVersion, setRequestVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    getMonitorDashboard(range)
      .then((nextResponse) => {
        if (!active) return;
        setResponse(nextResponse);
      })
      .catch((requestError: unknown) => {
        if (!active) return;
        setResponse(null);
        setError(requestError instanceof Error ? requestError.message : "Vui lòng thử lại sau.");
      })
      .finally(() => {
        if (!active) return;
        setIsLoading(false);
        setIsRefreshing(false);
      });

    return () => {
      active = false;
    };
  }, [range, requestVersion]);

  const refresh = () => {
    if (isLoading || isRefreshing) return;
    setIsRefreshing(true);
    setIsLoading(true);
    setError(null);
    setRequestVersion((version) => version + 1);
  };

  const changeRange = (nextRange: TimeRange) => {
    if (nextRange === range) return;
    setResponse(null);
    setError(null);
    setIsLoading(true);
    setRange(nextRange);
  };

  const snapshot = response?.snapshot;
  const busy = isLoading || isRefreshing;

  return (
    <div className="grid gap-1" aria-label="Dashboard giám sát rủi ro">
      <MonitorHeader range={range} onRangeChange={changeRange} isRefreshing={busy} onRefresh={refresh} />
      {error && !snapshot ? <DashboardError message={error} onRetry={refresh} /> : isLoading && !snapshot ? <DashboardSkeleton /> : snapshot ? <>
        <div className="grid grid-cols-2 gap-1 lg:grid-cols-5">{snapshot.metrics.map((metric) => <MetricCard key={metric.label} metric={metric} />)}</div>
        <div className="grid grid-cols-12 gap-1"><div className="col-span-12 xl:col-span-8"><RiskTrendChart series={snapshot.timeseries} peakTime={snapshot.peakTime} peakVelocity={snapshot.peakVelocity} /></div><div className="col-span-12 xl:col-span-4"><RiskBreakdown snapshot={snapshot} /></div></div>
        <div className="grid grid-cols-12 gap-1"><div className="col-span-12 lg:col-span-7"><RiskAccountsTable accounts={snapshot.accounts} /></div><div className="col-span-12 lg:col-span-5"><RealtimeAlertStream events={snapshot.events} /></div></div>
        <div className={`${mono} flex items-center justify-end gap-1 pt-1 text-[10px] text-(--admin-text-muted)`.trim()}><FontAwesomeIcon icon={faCircleExclamation} aria-hidden="true" />Cập nhật {response.updatedAt}{response.dataStatus === "stale" ? " · STALE" : ""}</div>
      </> : null}
    </div>
  );
}
