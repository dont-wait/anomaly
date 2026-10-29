import { useEffect, useRef, useState } from "react";
import type { IconDefinition } from "@fortawesome/fontawesome-svg-core";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowDown,
  faArrowUp,
  faArrowsRotate,
  faBan,
  faBell,
  faCircleCheck,
  faCircleExclamation,
  faCircleQuestion,
  faCircleXmark,
  faClock,
  faHourglassEnd,
  faStream,
} from "@fortawesome/free-solid-svg-icons";
import { navigate, routes } from "@/app/routes";
import { clearAdminSession } from "@/features/auth/api/adminSession";
import { consumeRiskStream, getMonitorDashboard, shouldAdvanceMonitorCursor } from "./api/monitorApi";
import { RiskTrendChart } from "./components/RiskTrendChart";
import { MonitorDashboardSkeleton } from "./components/MonitorDashboardSkeleton";
import { ranges, type MonitorDashboardResponse, type MonitorSnapshot, type RiskAssessment, type TimeRange } from "./model";

const surface = "bg-(--admin-surface)";
const surfaceSubtle = "bg-(--admin-surface-subtle)";
const surfaceHover = "hover:bg-(--admin-surface-hover)";
const mono = "font-(family-name:--admin-font-mono)";
const LOADING_SKELETON_DELAY_MS = 100;
const MIN_SKELETON_VISIBLE_MS = 240;
const TRANSACTION_STATUS_REFRESH_MS = 30_000;
const formatMoney = (amount: number, currency: string) => `${amount.toLocaleString("vi-VN")} ${currency || "VND"}`;

const transactionStatusLabels: Record<string, string> = {
  awaiting_otp: "Chờ OTP",
  success: "Đã hoàn tất",
  failed: "Bị từ chối",
  cancelled: "Đã huỷ",
  expired: "Hết hạn OTP",
};

const riskLevelLabels: Record<string, string> = {
  NORMAL: "Bình thường",
  WARNING: "Cảnh báo",
  BLOCK: "Chặn",
};

const decisionLabels: Record<string, string> = {
  ALLOW: "Cho phép",
  BLOCK: "Chặn",
};

function riskLevelLabel(level: string): string {
  return riskLevelLabels[level] || level;
}

function decisionLabel(decision: string): string {
  return decisionLabels[decision] || decision;
}

function transactionStatusLabel(status?: string): string {
  return status ? transactionStatusLabels[status] || status : "Chưa xác định";
}

const transactionStatusIcons: Record<string, IconDefinition> = {
  awaiting_otp: faClock,
  success: faCircleCheck,
  failed: faCircleXmark,
  cancelled: faBan,
  expired: faHourglassEnd,
};

function TransactionStatusIcon({ status }: { status?: string }) {
  const label = transactionStatusLabel(status);
  const icon = status ? transactionStatusIcons[status] || faCircleQuestion : faCircleQuestion;
  const color = status === "success" ? "text-[#75d9a4]" : status === "awaiting_otp" ? "text-[#f6d58a]" : status === "failed" || status === "cancelled" || status === "expired" ? "text-[#ff8f86]" : "text-(--admin-text-muted)";
  return <span className={`${color} inline-flex w-7 justify-center`} title={label} aria-label={label}><FontAwesomeIcon icon={icon} aria-hidden="true" /></span>;
}

interface Metric {
  label: string;
  value: string;
  note?: string;
  icon: IconDefinition;
  tone?: "default" | "danger" | "primary";
  change?: string;
  trend?: "up" | "down" | "flat";
}

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
      {metric.note && <span className="mt-1 text-[11px] leading-4 text-(--admin-text-muted)">{metric.note}</span>}
    </article>
  );
}

function MonitorHeader({ range, onRangeChange, isRefreshing, onRefresh, streamStatus }: { range: TimeRange; onRangeChange: (range: TimeRange) => void; isRefreshing: boolean; onRefresh: () => void; streamStatus: string }) {
  return (
    <section className={`${surfaceSubtle} flex flex-col justify-between gap-3 rounded-(--admin-radius-md) p-3 md:flex-row md:items-center`}>
      <div className="flex min-w-0 items-start gap-2.5">
        <span className="mt-1 h-7 w-1.5 shrink-0 rounded-sm bg-(--admin-danger)" aria-hidden="true" />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-[18px] leading-6 font-bold tracking-[-0.02em] text-(--admin-text) uppercase">
              Giám sát rủi ro giao dịch
            </h1>
            <span className={`${mono} rounded-sm bg-(--admin-surface-hover) px-1.5 py-0.5 text-[10px] tracking-[0.03em] text-(--admin-primary)`}>TRỰC TIẾP</span>
          </div>
          <p className="mt-0.5 text-[13px] leading-5 text-(--admin-text-muted)">Theo dõi yêu cầu giao dịch và cảnh báo rủi ro theo thời gian thực</p>
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
          <span className={`${mono} text-[10px] uppercase tracking-[0.06em] text-(--admin-text-secondary)`}>{streamStatus}</span>
        </div>
        <button className={`${surface} ${surfaceHover} flex h-7 items-center gap-1.5 rounded-sm px-2.5 text-[12px] font-semibold text-(--admin-text) transition-colors`} type="button" onClick={onRefresh} disabled={isRefreshing}>
          <FontAwesomeIcon icon={faArrowsRotate} className={isRefreshing ? "animate-spin" : ""} aria-hidden="true" />
          {isRefreshing ? "Đang cập nhật..." : "Làm mới"}
        </button>
      </div>
    </section>
  );
}

function RiskBreakdown({ snapshot }: { snapshot: MonitorSnapshot }) {
  const totalAssessments = snapshot.totalAssessments;
  const labelItems = [
    { label: "Bình thường", value: snapshot.normalCount, tone: "primary" },
    { label: "Cảnh báo", value: snapshot.warningCount, tone: "secondary" },
    { label: "Chặn", value: snapshot.blockCount, tone: "danger" },
  ];
  const severityTone: Record<string, string> = {
    danger: "bg-(--admin-danger)",
    primary: "bg-(--admin-primary)",
    secondary: "bg-[#3c4a5e]",
    muted: "bg-(--admin-surface-hover)",
  };
  return (
    <section className={`${surfaceSubtle} h-full flex flex-col justify-between gap-6 rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex items-center justify-between gap-2 pb-2">
          <h2 className="text-[16px] leading-5 font-semibold text-(--admin-text)">Phân bố mức rủi ro</h2>
          <span className={`${mono} text-[11px] text-(--admin-text-muted)`}>{totalAssessments} yêu cầu</span>
        </div>
        <div className="my-2 flex h-3 overflow-hidden rounded-sm bg-(--admin-surface-hover)">
          {labelItems.map((item) => <div key={item.label} className={severityTone[item.tone]} style={{ width: totalAssessments ? `${(item.value / totalAssessments) * 100}%` : "0%" }} title={`${item.label}: ${item.value}`} />)}
        </div>
        <div className="grid grid-cols-3 gap-1">
          {labelItems.map((item) => (
            <div key={item.label} className={`${surface} flex items-center justify-between rounded-sm px-2 py-1`}>
              <span className="flex items-center gap-1 text-[11px] font-semibold uppercase text-(--admin-text-muted)"><span className={`h-2 w-2 rounded-sm ${severityTone[item.tone]}`} />{item.label}</span>
              <strong className={`${mono} text-[12px] ${item.tone === "danger" ? "text-(--admin-danger)" : item.tone === "primary" ? "text-(--admin-primary)" : "text-(--admin-text)"}`}>{String(item.value).padStart(2, "0")}</strong>
            </div>
          ))}
        </div>
      </div>
      <div>
        <span className={`${mono} mb-2 block text-[11px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)`}>Phiên bản mô hình</span>
        <div className="grid gap-2">
          {snapshot.modelVersions.map((item) => (
            <div key={item.label}>
              <div className="mb-0.5 flex items-center justify-between gap-2 text-[12px] text-(--admin-text-secondary)"><span className="truncate">{item.label}</span><span className={mono}>{item.count}</span></div>
            </div>
          ))}
          {snapshot.modelVersions.length === 0 && <span className="text-[12px] text-(--admin-text-muted)">Chưa có dữ liệu.</span>}
        </div>
      </div>
    </section>
  );
}

function RiskRequestsTable({ assessments }: { assessments: RiskAssessment[] }) {
  return (
    <section className={`${surfaceSubtle} flex min-h-80 flex-col justify-between rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex flex-wrap items-center justify-between gap-2 pb-2">
          <div className="flex items-center gap-2"><h2 className="text-[16px] leading-5 font-semibold text-(--admin-text) uppercase">Yêu cầu chấm điểm gần nhất</h2><span className={`${mono} rounded-sm bg-[#5b2427] px-1.5 py-0.5 text-[10px] text-[#ffb4ab]`}>KẾT QUẢ MÔ HÌNH</span></div>
          <span className="text-[12px] text-(--admin-text-muted)">{assessments.length} kết quả</span>
        </div>
        {assessments.length === 0 ? <div className="grid min-h-48 place-items-center text-[12px] text-(--admin-text-muted)">Chưa có dữ liệu trong khoảng thời gian này.</div> : <div className="w-full overflow-x-auto"><table className="w-full min-w-250 text-left"><thead><tr className={`${surface} h-7 text-[10px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)`}><th className="px-2">Mã giao dịch</th><th className="px-2">Tài khoản gửi</th><th className="px-2 text-right">Số tiền</th><th className="px-2 text-center">Điểm rủi ro</th><th className="px-2 text-center">Mức rủi ro</th><th className="px-2 text-center">Kết quả</th><th className="px-2 text-center">Trạng thái</th><th className="px-2 text-center">Ngưỡng</th><th className="px-2">Mô hình</th><th className="px-2 text-right">Thời điểm</th></tr></thead><tbody>{[...assessments].reverse().map((assessment) => <tr key={assessment.assessmentId} className={`h-9 transition-colors ${surfaceHover}`}><td className={`${mono} px-2 text-[11px] font-bold text-(--admin-primary)`}>{assessment.transactionId}</td><td className="max-w-35 truncate px-2 text-[12px] text-(--admin-text-secondary)" title={assessment.transaction?.sourceAccountName || undefined}>{assessment.transaction?.sourceAccountNo || "—"}</td><td className={`${mono} px-2 text-right text-[11px] text-(--admin-text)`}>{assessment.transaction ? formatMoney(assessment.transaction.amount, assessment.transaction.currency) : "—"}</td><td className={`${mono} px-2 text-center text-[12px] font-semibold text-(--admin-text)`}>{assessment.riskScore.toFixed(4)}</td><td className="px-2 text-center"><span className={`${mono} rounded-sm px-1.5 py-1 text-[10px] font-bold ${assessment.riskLevel === "BLOCK" ? "bg-[#5b2427] text-[#ffb4ab]" : assessment.riskLevel === "WARNING" ? "bg-[#4b3b1d] text-[#f6d58a]" : "bg-(--admin-surface-hover) text-(--admin-text-secondary)"}`}>{riskLevelLabel(assessment.riskLevel)}</span></td><td className={`${mono} px-2 text-center text-[11px] font-bold ${assessment.decision === "BLOCK" ? "text-(--admin-danger)" : "text-(--admin-primary)"}`}>{decisionLabel(assessment.decision)}</td><td className="px-2 text-center"><TransactionStatusIcon status={assessment.transaction?.status} /></td><td className={`${mono} px-2 text-center text-[12px] text-(--admin-text)`}>{assessment.threshold.toFixed(4)}</td><td className="max-w-35 truncate px-2 text-[11px] text-(--admin-text-secondary)">{assessment.modelVersion}</td><td className={`${mono} px-2 text-right text-[10px] text-(--admin-text-muted)`}>{new Date(assessment.receivedAt).toLocaleTimeString("vi-VN", { hour12: false })}</td></tr>)}</tbody></table></div>}
      </div>
    </section>
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
  const [showSkeleton, setShowSkeleton] = useState(false);
  const [skeletonForRequest, setSkeletonForRequest] = useState(true);
  const skeletonOnLoadRef = useRef(true);
  const skeletonVisibleAtRef = useRef<number | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [requestVersion, setRequestVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [streamStatus, setStreamStatus] = useState("Đang kết nối");
  const responseRef = useRef<MonitorDashboardResponse | null>(null);
  const streamCursorRef = useRef("");
  const hasResponse = response !== null;

  useEffect(() => {
    responseRef.current = response;
  }, [response]);

  useEffect(() => {
    if (!hasResponse) return;
    const timer = window.setInterval(() => {
      setRequestVersion((version) => version + 1);
    }, TRANSACTION_STATUS_REFRESH_MS);
    return () => window.clearInterval(timer);
  }, [hasResponse]);

  useEffect(() => {
    let active = true;
    let skeletonTimer: number | undefined;
    let finishTimer: number | undefined;
    let retryTimer: number | undefined;
    let requestFailed = false;

    if (skeletonOnLoadRef.current) {
      skeletonTimer = window.setTimeout(() => {
        if (!active) return;
        skeletonVisibleAtRef.current = performance.now();
        setShowSkeleton(true);
      }, LOADING_SKELETON_DELAY_MS);
    }

    const finishLoading = () => {
      skeletonOnLoadRef.current = false;
      skeletonVisibleAtRef.current = null;
      setSkeletonForRequest(false);
      setShowSkeleton(false);
      setIsLoading(false);
      setIsRefreshing(false);
    };

    getMonitorDashboard(range)
      .then((nextResponse) => {
        if (!active) return;
        if (shouldAdvanceMonitorCursor(streamCursorRef.current, nextResponse.cursor)) {
          streamCursorRef.current = nextResponse.cursor;
        }
        setResponse(nextResponse);
      })
      .catch((requestError: unknown) => {
        if (!active) return;
        if (requestError instanceof Error && "status" in requestError && [401, 403].includes((requestError as { status?: number }).status ?? 0)) {
          clearAdminSession();
          navigate(routes.login);
          setStreamStatus("Phiên đăng nhập hết hạn");
          return;
        }
        requestFailed = true;
        streamCursorRef.current = "";
        setResponse(null);
        setError(requestError instanceof Error ? requestError.message : "Vui lòng thử lại sau.");
        retryTimer = window.setTimeout(() => {
          if (active) {
            setIsLoading(true);
            setError(null);
            setRequestVersion((version) => version + 1);
          }
        }, 3000);
      })
      .finally(() => {
        if (!active) return;
        if (skeletonTimer !== undefined) window.clearTimeout(skeletonTimer);
        if (requestFailed) {
          finishLoading();
          return;
        }
        const skeletonVisibleAt = skeletonVisibleAtRef.current;
        const remainingVisibleTime = skeletonVisibleAt === null
          ? 0
          : Math.max(0, MIN_SKELETON_VISIBLE_MS - (performance.now() - skeletonVisibleAt));

        if (remainingVisibleTime > 0) {
          finishTimer = window.setTimeout(finishLoading, remainingVisibleTime);
        } else {
          finishLoading();
        }
      });

    return () => {
      active = false;
      if (skeletonTimer !== undefined) window.clearTimeout(skeletonTimer);
      if (finishTimer !== undefined) window.clearTimeout(finishTimer);
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
    };
  }, [range, requestVersion]);

  useEffect(() => {
    const initialResponse = responseRef.current;
    if (!initialResponse) return;
    let cancelled = false;
    let retryTimer: number | undefined;
    let controller: AbortController | undefined;
    if (!streamCursorRef.current) streamCursorRef.current = initialResponse.cursor;
    let delay = 1000;

    const run = async () => {
      while (!cancelled) {
        controller = new AbortController();
        setStreamStatus("Đang kết nối");
        try {
          await consumeRiskStream(streamCursorRef.current, (event, nextCursor) => {
            if (cancelled) return;
            if (nextCursor) streamCursorRef.current = nextCursor;
            setStreamStatus("Trực tiếp");
            setResponse((current) => {
              if (!current || current.snapshot.recentAssessments.some((item) => item.assessmentId === event.assessmentId)) return current;
              const previous = current.snapshot;
              const total = previous.totalAssessments + 1;
              const anomalous = previous.anomalousCount + event.predictedLabel;
              const normalCount = previous.normalCount + (event.riskLevel === "NORMAL" ? 1 : 0);
              const warningCount = previous.warningCount + (event.riskLevel === "WARNING" ? 1 : 0);
              const blockCount = previous.blockCount + (event.riskLevel === "BLOCK" ? 1 : 0);
              const recentAssessments = [...previous.recentAssessments, event].slice(-50);
              const existingVersion = previous.modelVersions.find((item) => item.label === event.modelVersion);
              const modelVersions = existingVersion
                ? previous.modelVersions.map((item) => item.label === event.modelVersion ? { ...item, count: item.count + 1 } : item)
                : [...previous.modelVersions, { label: event.modelVersion, count: 1 }];
              const bucketAt = new Date(event.receivedAt);
              bucketAt.setMinutes(0, 0, 0);
              const timeseries = [...previous.timeseries];
              const last = timeseries[timeseries.length - 1];
              if (last && new Date(last.timestamp).getTime() === bucketAt.getTime()) {
                const count = last.assessmentCount + 1;
                timeseries[timeseries.length - 1] = { ...last, assessmentCount: count, anomalousCount: last.anomalousCount + event.predictedLabel, averageRiskScore: (last.averageRiskScore * last.assessmentCount + event.riskScore) / count };
              } else {
                timeseries.push({ timestamp: bucketAt.toISOString(), assessmentCount: 1, anomalousCount: event.predictedLabel, averageRiskScore: event.riskScore });
              }
              return { ...current, cursor: streamCursorRef.current, updatedAt: event.receivedAt, snapshot: { ...previous, totalAssessments: total, anomalousCount: anomalous, normalCount, warningCount, blockCount, anomalyRate: anomalous / total, averageRiskScore: (previous.averageRiskScore * previous.totalAssessments + event.riskScore) / total, recentAssessments, modelVersions, timeseries } };
            });
          }, controller.signal);
          if (!cancelled) setStreamStatus("Đang kết nối lại");
        } catch (streamError) {
          if (cancelled || (streamError instanceof DOMException && streamError.name === "AbortError")) return;
          if (streamError instanceof Error && "status" in streamError && [401, 403].includes((streamError as { status?: number }).status ?? 0)) {
            clearAdminSession();
            navigate(routes.login);
            setStreamStatus("Phiên đăng nhập hết hạn");
            return;
          }
          setStreamStatus("Mất kết nối");
        }
        if (cancelled) return;
        retryTimer = window.setTimeout(() => { void run(); }, delay);
        delay = Math.min(delay * 2, 15000);
        return;
      }
    };
    void run();
    return () => {
      cancelled = true;
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
      controller?.abort();
    };
  }, [hasResponse]);

  const refresh = () => {
    if (isLoading || isRefreshing) return;
    setIsRefreshing(true);
    setIsLoading(true);
    setShowSkeleton(false);
    const shouldShowSkeleton = response === null;
    skeletonOnLoadRef.current = shouldShowSkeleton;
    setSkeletonForRequest(shouldShowSkeleton);
    setError(null);
    setRequestVersion((version) => version + 1);
  };

  const changeRange = (nextRange: TimeRange) => {
    if (nextRange === range) return;
    setResponse(null);
    setError(null);
    setIsLoading(true);
    setShowSkeleton(false);
    skeletonOnLoadRef.current = true;
    setSkeletonForRequest(true);
    setRange(nextRange);
  };

  const snapshot = response?.snapshot;
  const busy = isLoading || isRefreshing;

  return (
    <div className="grid gap-1" aria-busy={busy} aria-label="Dashboard giám sát rủi ro">
      <MonitorHeader range={range} onRangeChange={changeRange} isRefreshing={busy} onRefresh={refresh} streamStatus={streamStatus} />
      {error && !snapshot ? <DashboardError message={error} onRetry={refresh} /> : showSkeleton && skeletonForRequest ? <MonitorDashboardSkeleton /> : snapshot ? <>
        <div className="grid grid-cols-2 gap-1 lg:grid-cols-6">{metricsForSnapshot(snapshot).map((metric) => <MetricCard key={metric.label} metric={metric} />)}</div>
        <div className="grid grid-cols-12 items-stretch gap-1"><div className="col-span-12 xl:col-span-8 h-full"><RiskTrendChart series={snapshot.timeseries} range={range} /></div><div className="col-span-12 xl:col-span-4 h-full"><RiskBreakdown snapshot={snapshot} /></div></div>
        <div className="grid grid-cols-12 gap-1"><div className="col-span-12"><RiskRequestsTable assessments={snapshot.recentAssessments} /></div></div>
        <div className={`${mono} flex items-center justify-end gap-1 pt-1 text-[10px] text-(--admin-text-muted)`.trim()}><FontAwesomeIcon icon={faCircleExclamation} aria-hidden="true" />Cập nhật lúc {response.updatedAt}{response.dataStatus === "stale" ? " · DỮ LIỆU CŨ" : ""}</div>
      </> : null}
    </div>
  );
}

function metricsForSnapshot(snapshot: MonitorSnapshot): Metric[] {
  return [
    { label: "Yêu cầu đã chấm điểm", value: String(snapshot.totalAssessments), note: "Trong khoảng thời gian đã chọn", icon: faBell },
    { label: "Cảnh báo", value: String(snapshot.warningCount), note: "Còn chờ xác nhận", tone: "primary", icon: faCircleExclamation },
    { label: "Chặn", value: String(snapshot.blockCount), note: "Cần kiểm tra", tone: "danger", icon: faCircleExclamation },
    { label: "Tỷ lệ bất thường", value: `${(snapshot.anomalyRate * 100).toFixed(2)}%`, tone: "primary", icon: faArrowUp },
    { label: "Điểm rủi ro trung bình", value: snapshot.averageRiskScore.toFixed(4), icon: faStream },
    { label: "Phiên bản mô hình", value: String(snapshot.modelVersions.length), icon: faArrowsRotate },
  ];
}
