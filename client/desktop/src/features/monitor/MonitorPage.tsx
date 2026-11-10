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
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { navigate, routes } from "@/app/routes";
import { clearAdminSession } from "@/features/auth/api/adminSession";
import {
  consumeRiskStream,
  getMonitorDashboard,
  shouldAdvanceMonitorCursor,
} from "./api/monitorApi";
import { RiskTrendChart } from "./components/RiskTrendChart";
import { MonitorDashboardSkeleton } from "./components/MonitorDashboardSkeleton";
import {
  ranges,
  type MonitorDashboardResponse,
  type MonitorSnapshot,
  type RiskAssessment,
  type TimeRange,
} from "./model";

const surface = "bg-(--admin-surface)";
const surfaceSubtle = "bg-(--admin-surface-subtle)";
const surfaceHover = "hover:bg-(--admin-surface-hover)";
const dashboardCard =
  "bg-(--admin-dashboard-card-background) border border-(--admin-dashboard-card-border) shadow-(--admin-dashboard-card-shadow)";
const dashboardControl =
  "bg-(--admin-dashboard-control-background) border border-(--admin-dashboard-control-border)";
const mono = "font-(family-name:--admin-font-mono)";
const LOADING_SKELETON_DELAY_MS = 100;
const MIN_SKELETON_VISIBLE_MS = 240;
const TRANSACTION_STATUS_REFRESH_MS = 30_000;
const formatMoney = (amount: number, currency: string) =>
  `${amount.toLocaleString("vi-VN")} ${currency || "VND"}`;

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
  const icon = status
    ? transactionStatusIcons[status] || faCircleQuestion
    : faCircleQuestion;
  const color =
    status === "success"
      ? "text-(--admin-success-foreground)"
      : status === "awaiting_otp"
        ? "text-(--admin-warning-foreground)"
        : status === "failed" || status === "cancelled" || status === "expired"
          ? "text-(--admin-danger-foreground)"
          : "text-(--admin-text-muted)";
  return (
    <span
      className={`${color} inline-flex w-7 justify-center`}
      title={label}
      aria-label={label}
    >
      <FontAwesomeIcon icon={icon} aria-hidden="true" />
    </span>
  );
}

interface Metric {
  label: string;
  value: string;
  note?: string;
  icon: IconDefinition;
  tone?: "default" | "danger" | "primary" | "warning";
  change?: string;
  trend?: "up" | "down" | "flat";
}

function MetricCard({ metric }: { metric: Metric }) {
  const valueClass =
    metric.tone === "danger"
      ? "text-(--admin-danger)"
      : metric.tone === "warning"
        ? "text-(--admin-metric-warning)"
        : metric.tone === "primary"
          ? "text-(--admin-primary)"
          : "text-(--admin-text)";
  const changeClass =
    metric.trend === "up"
      ? "text-(--admin-trend-up)"
      : "text-(--admin-text-secondary)";

  return (
    <article
      className={`${dashboardCard} flex min-h-28 flex-col justify-between rounded-(--admin-radius-lg) p-4`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-[11px] font-semibold tracking-[0.02em] text-(--admin-text-secondary)">
          {metric.label}
        </span>
        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-(--admin-radius-md) bg-(--admin-metric-icon-background) text-(--admin-metric-icon-foreground)">
          <FontAwesomeIcon
            icon={metric.icon}
            className="text-[13px]"
            aria-hidden="true"
          />
        </span>
      </div>
      <div className="mt-2 flex items-baseline justify-between gap-2">
        <strong
          className={`${mono} text-[26px] leading-7 font-bold tracking-[-0.03em] ${valueClass}`}
        >
          {metric.value}
        </strong>
        {metric.change && (
          <span
            className={`${mono} flex items-center gap-0.5 whitespace-nowrap text-[11px] ${changeClass}`}
          >
            {metric.trend === "up" ? (
              <FontAwesomeIcon icon={faArrowUp} aria-hidden="true" />
            ) : (
              <FontAwesomeIcon icon={faArrowDown} aria-hidden="true" />
            )}
            {metric.change}
          </span>
        )}
      </div>
      {metric.note && (
        <span className="mt-2 text-[11px] leading-4 text-(--admin-text-muted)">
          {metric.note}
        </span>
      )}
    </article>
  );
}

function MonitorHeader({
  range,
  onRangeChange,
  isRefreshing,
  onRefresh,
  streamStatus,
}: {
  range: TimeRange;
  onRangeChange: (range: TimeRange) => void;
  isRefreshing: boolean;
  onRefresh: () => void;
  streamStatus: string;
}) {
  return (
    <section
      className={`${dashboardCard} flex flex-col justify-between gap-4 rounded-(--admin-radius-lg) p-4 md:flex-row md:items-center`}
    >
      <div className="flex min-w-0 items-start gap-2.5">
        <span
          className="mt-1 h-9 w-1.5 shrink-0 rounded-full bg-(--admin-primary)"
          aria-hidden="true"
        />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <p
              className={`${mono} text-[10px] font-semibold tracking-[0.08em] text-(--admin-primary) uppercase`}
            >
              OPERATIONS / RISK MONITOR
            </p>
            <h1 className="mt-1 text-[20px] leading-6 font-bold tracking-[-0.025em] text-(--admin-text)">
              Giám sát rủi ro giao dịch
            </h1>
          </div>
          <p className="mt-1 text-[12px] leading-5 text-(--admin-text-muted)">
            Theo dõi yêu cầu giao dịch và cảnh báo rủi ro theo thời gian thực
          </p>
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2 self-start md:self-auto">
        <div
          className={`${dashboardControl} flex rounded-(--admin-radius-md) p-1`}
          role="group"
          aria-label="Khoảng thời gian"
        >
          {ranges.map((item) => (
            <button
              key={item.value}
              className={`rounded-(--admin-radius-sm) px-3 py-1.5 text-[11px] font-semibold transition-colors ${range === item.value ? "bg-(--admin-primary-soft) text-(--admin-primary)" : "text-(--admin-text-muted) hover:bg-(--admin-surface-hover) hover:text-(--admin-text)"}`}
              type="button"
              aria-pressed={range === item.value}
              onClick={() => onRangeChange(item.value)}
            >
              {item.label}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2 rounded-(--admin-radius-md) border border-(--admin-live-border) bg-(--admin-live-background) px-3 py-2">
          <span
            className="h-2 w-2 animate-pulse rounded-full bg-(--admin-live-dot)"
            aria-hidden="true"
          />
          <span
            className={`${mono} text-[10px] font-semibold uppercase tracking-[0.06em] text-(--admin-live-foreground)`}
          >
            {streamStatus}
          </span>
        </div>
        <button
          className={`${dashboardControl} ${surfaceHover} flex h-9 items-center gap-2 rounded-(--admin-radius-md) px-3 text-[11px] font-semibold text-(--admin-text) shadow-(--admin-shadow-card) transition-colors`}
          type="button"
          onClick={onRefresh}
          disabled={isRefreshing}
        >
          <FontAwesomeIcon
            icon={faArrowsRotate}
            className={isRefreshing ? "animate-spin" : ""}
            aria-hidden="true"
          />
          {isRefreshing ? "Đang cập nhật..." : "Làm mới"}
        </button>
      </div>
    </section>
  );
}

function RiskBreakdown({ snapshot }: { snapshot: MonitorSnapshot }) {
  const totalAssessments = snapshot.totalAssessments;
  const maxModelVersionCount = Math.max(
    ...snapshot.modelVersions.map((item) => item.count),
    1,
  );
  const labelItems = [
    { label: "Bình thường", value: snapshot.normalCount, tone: "primary" },
    { label: "Cảnh báo", value: snapshot.warningCount, tone: "secondary" },
    { label: "Chặn", value: snapshot.blockCount, tone: "danger" },
  ];
  const severityTone: Record<string, string> = {
    danger: "bg-(--admin-danger)",
    primary: "bg-(--admin-primary)",
    secondary: "bg-(--admin-risk-secondary)",
    muted: "bg-(--admin-surface-hover)",
  };
  return (
    <section
      className={`${dashboardCard} h-full flex flex-col justify-between gap-6 rounded-(--admin-radius-lg) p-4`}
    >
      <div>
        <div className="flex items-center justify-between gap-2 pb-2">
          <h2 className="text-[14px] leading-5 font-semibold text-(--admin-text)">
            Phân bố mức rủi ro
          </h2>
          <span className={`${mono} text-[11px] text-(--admin-text-muted)`}>
            {totalAssessments} yêu cầu
          </span>
        </div>
        <div className="my-3 flex h-2.5 overflow-hidden rounded-full bg-(--admin-surface-hover)">
          {labelItems.map((item) => (
            <div
              key={item.label}
              className={severityTone[item.tone]}
              style={{
                width: totalAssessments
                  ? `${(item.value / totalAssessments) * 100}%`
                  : "0%",
              }}
              title={`${item.label}: ${item.value}`}
            />
          ))}
        </div>
        <div className="grid grid-cols-3 gap-1">
          {labelItems.map((item) => (
            <div
              key={item.label}
              className={`${dashboardControl} flex items-center justify-between rounded-(--admin-radius-sm) px-2 py-1.5`}
            >
              <span className="flex items-center gap-1 text-[11px] font-semibold uppercase text-(--admin-text-muted)">
                <span
                  className={`h-2 w-2 rounded-sm ${severityTone[item.tone]}`}
                />
                {item.label}
              </span>
              <strong
                className={`${mono} text-[12px] ${item.tone === "danger" ? "text-(--admin-danger)" : item.tone === "primary" ? "text-(--admin-primary)" : "text-(--admin-text)"}`}
              >
                {String(item.value).padStart(2, "0")}
              </strong>
            </div>
          ))}
        </div>
      </div>
      <div>
        <span
          className={`${mono} mb-3 block text-[10px] font-semibold uppercase tracking-[0.06em] text-(--admin-text-muted)`}
        >
          Phiên bản mô hình
        </span>
        <div className="grid gap-2">
          {snapshot.modelVersions.map((item) => (
            <div key={item.label}>
              <div className="mb-0.5 flex items-center justify-between gap-2 text-[12px] text-(--admin-text-secondary)">
                <span className="truncate">{item.label}</span>
                <span className={mono}>{item.count}</span>
              </div>
              <div className="h-1 overflow-hidden rounded-full bg-(--admin-surface-hover)">
                <div
                  className="h-full rounded-full bg-(--admin-primary)"
                  style={{
                    width: `${(item.count / maxModelVersionCount) * 100}%`,
                  }}
                />
              </div>
            </div>
          ))}
          {snapshot.modelVersions.length === 0 && (
            <span className="text-[12px] text-(--admin-text-muted)">
              Chưa có dữ liệu.
            </span>
          )}
        </div>
      </div>
    </section>
  );
}

function RiskRequestsTable({
  assessments,
  onSelect,
}: {
  assessments: RiskAssessment[];
  onSelect: (assessment: RiskAssessment) => void;
}) {
  const recentAssessments = [...assessments].reverse().slice(0, 5);
  const filteredAssessments = [...recentAssessments].reverse();

  return (
    <section
      className={`${dashboardCard} flex min-h-80 flex-col justify-between rounded-(--admin-radius-lg) p-4`}
    >
      <div>
        <div className="flex flex-wrap items-center justify-between gap-2 pb-2">
          <div className="flex items-center gap-2">
            <h2 className="text-[14px] leading-5 font-semibold text-(--admin-text)">
              Các yêu cầu gần đây
            </h2>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-[12px] text-(--admin-text-muted)">
              Hiển thị {recentAssessments.length} yêu cầu mới nhất
            </span>
            <button
              className="h-9 rounded-(--admin-radius-md) border border-(--admin-primary) bg-(--admin-primary) px-3 text-[11px] font-semibold text-white shadow-(--admin-shadow-card) transition-colors hover:bg-(--admin-primary-strong)"
              type="button"
              onClick={() => navigate(routes.audit)}
            >
              Xem tất cả
            </button>
          </div>
        </div>
        {filteredAssessments.length === 0 ? (
          <div className="grid min-h-48 place-items-center text-[12px] text-(--admin-text-muted)">
            {assessments.length === 0
              ? "Chưa có dữ liệu trong khoảng thời gian này."
              : "Không tìm thấy kết quả phù hợp."}
          </div>
        ) : (
          <div className="w-full overflow-x-auto">
            <table className="w-full min-w-250 text-left">
              <thead>
                <tr
                  className={`${dashboardControl} h-9 text-[10px] font-semibold uppercase tracking-[0.05em] text-(--admin-text-muted)`}
                >
                  <th className="px-2">Mã giao dịch</th>
                  <th className="px-2">Tài khoản gửi</th>
                  <th className="px-2 text-right">Số tiền</th>
                  <th className="px-2 text-center">Điểm rủi ro</th>
                  <th className="px-2 text-center">Mức rủi ro</th>
                  <th className="px-2 text-center">Kết quả</th>
                  <th className="px-2 text-center">Trạng thái</th>
                  <th className="px-2 text-center">Ngưỡng</th>
                  <th className="px-2 text-right">Thời điểm</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-(--admin-dashboard-divider)">
                {[...filteredAssessments].reverse().map((assessment) => (
                  <tr
                    key={assessment.assessmentId}
                    onClick={() => onSelect(assessment)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ")
                        onSelect(assessment);
                    }}
                    role="button"
                    tabIndex={0}
                    className={`h-12 cursor-pointer transition-colors ${surfaceHover}`}
                  >
                    <td
                      className={`${mono} px-2 text-[11px] font-bold text-(--admin-primary)`}
                    >
                      {assessment.transactionId}
                    </td>
                    <td
                      className="max-w-35 truncate px-2 text-[12px] text-(--admin-text-secondary)"
                      title={
                        assessment.transaction?.sourceAccountName || undefined
                      }
                    >
                      {assessment.transaction?.sourceAccountNo || "—"}
                    </td>
                    <td
                      className={`${mono} px-2 text-right text-[11px] text-(--admin-text)`}
                    >
                      {assessment.transaction
                        ? formatMoney(
                            assessment.transaction.amount,
                            assessment.transaction.currency,
                          )
                        : "—"}
                    </td>
                    <td
                      className={`${mono} px-2 text-center text-[12px] font-semibold text-(--admin-text)`}
                    >
                      {assessment.riskScore.toFixed(4)}
                    </td>
                    <td className="px-2 text-center">
                      <span
                        className={`${mono} rounded-sm px-1.5 py-1 text-[10px] font-bold ${assessment.riskLevel === "BLOCK" ? "bg-(--admin-risk-block-background) text-(--admin-risk-block-foreground)" : assessment.riskLevel === "WARNING" ? "bg-(--admin-risk-warning-background) text-(--admin-risk-warning-foreground)" : "bg-(--admin-surface-hover) text-(--admin-text-secondary)"}`}
                      >
                        {riskLevelLabel(assessment.riskLevel)}
                      </span>
                    </td>
                    <td
                      className={`${mono} px-2 text-center text-[11px] font-bold ${assessment.decision === "BLOCK" ? "text-(--admin-danger)" : "text-(--admin-primary)"}`}
                    >
                      {decisionLabel(assessment.decision)}
                    </td>
                    <td className="px-2 text-center">
                      <TransactionStatusIcon
                        status={assessment.transaction?.status}
                      />
                    </td>
                    <td
                      className={`${mono} px-2 text-center text-[12px] text-(--admin-text)`}
                    >
                      {assessment.threshold.toFixed(4)}
                    </td>
                    <td
                      className={`${mono} px-2 text-right text-[10px] text-(--admin-text-muted)`}
                    >
                      {new Date(assessment.receivedAt).toLocaleTimeString(
                        "vi-VN",
                        { hour12: false },
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}

function DashboardError({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <section
      className={`${surfaceSubtle} grid min-h-55 place-items-center rounded-(--admin-radius-md) p-6 text-center`}
      role="alert"
    >
      <div className="grid max-w-105 gap-2">
        <FontAwesomeIcon
          icon={faCircleExclamation}
          className="mx-auto text-2xl text-(--admin-danger)"
          aria-hidden="true"
        />
        <strong className="text-[15px] text-(--admin-text)">
          Không thể tải dữ liệu giám sát
        </strong>
        <span className="text-[12px] leading-5 text-(--admin-text-muted)">
          {message}
        </span>
        <button
          className="mx-auto mt-1 h-8 rounded-sm bg-(--admin-primary) px-3 text-[12px] font-semibold text-white hover:bg-(--admin-primary-strong)"
          type="button"
          onClick={onRetry}
        >
          Thử lại
        </button>
      </div>
    </section>
  );
}

function formatDateTime(value?: string): string {
  if (!value) return "—";
  return new Intl.DateTimeFormat("vi-VN", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(new Date(value));
}

function AssessmentDetailsDrawer({
  assessment,
  onClose,
}: {
  assessment: RiskAssessment;
  onClose: () => void;
}) {
  const transaction = assessment.transaction;
  const timeline: { label: string; value?: string; current?: boolean }[] = [
    { label: "Yêu cầu được tạo", value: transaction?.createdAt },
    { label: "Đã chấm điểm", value: assessment.receivedAt },
  ];
  if (transaction?.otpVerifiedAt)
    timeline.push({
      label: "OTP đã xác nhận",
      value: transaction.otpVerifiedAt,
    });
  if (transaction?.status === "awaiting_otp")
    timeline.push({ label: "Chờ OTP", current: true });
  else if (transaction?.status === "expired")
    timeline.push({
      label: "Hết hạn OTP",
      value: transaction.otpExpiresAt,
      current: !transaction.otpExpiresAt,
    });
  else if (transaction?.status === "success")
    timeline.push({
      label: "Đã hoàn tất",
      value: transaction.completedAt,
      current: !transaction.completedAt,
    });
  else if (transaction?.status === "failed")
    timeline.push({
      label: "Bị từ chối",
      value: transaction.failedAt,
      current: !transaction.failedAt,
    });
  else if (transaction?.status === "cancelled")
    timeline.push({
      label: "Đã huỷ",
      value: transaction.cancelledAt,
      current: !transaction.cancelledAt,
    });

  return (
    <div
      className="fixed inset-0 z-50 flex justify-end bg-black/40"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <aside
        className={`${surfaceSubtle} h-full w-full max-w-105 overflow-y-auto p-4 shadow-2xl`}
        role="dialog"
        aria-modal="true"
        aria-label="Chi tiết yêu cầu chấm điểm"
      >
        <div className="flex items-start justify-between gap-3 border-b border-(--admin-border) pb-3">
          <div>
            <p
              className={`${mono} text-[10px] uppercase tracking-[0.08em] text-(--admin-text-muted)`}
            >
              Chi tiết yêu cầu
            </p>
            <h2 className="mt-1 text-[18px] font-semibold text-(--admin-text)">
              {assessment.transactionId}
            </h2>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Đóng chi tiết"
            className={`${surface} ${surfaceHover} grid h-8 w-8 place-items-center rounded-sm text-(--admin-text-secondary)`}
          >
            <FontAwesomeIcon icon={faXmark} aria-hidden="true" />
          </button>
        </div>
        <div className="mt-4 grid grid-cols-2 gap-2">
          <div className={`${surface} rounded-sm p-2`}>
            <span className="text-[10px] text-(--admin-text-muted)">
              Mức rủi ro
            </span>
            <strong className="mt-1 block text-[13px] text-(--admin-text)">
              {riskLevelLabel(assessment.riskLevel)}
            </strong>
          </div>
          <div className={`${surface} rounded-sm p-2`}>
            <span className="text-[10px] text-(--admin-text-muted)">
              Kết quả
            </span>
            <strong className="mt-1 block text-[13px] text-(--admin-text)">
              {decisionLabel(assessment.decision)}
            </strong>
          </div>
          <div className={`${surface} rounded-sm p-2`}>
            <span className="text-[10px] text-(--admin-text-muted)">
              Điểm rủi ro
            </span>
            <strong className="mt-1 block text-[13px] text-(--admin-text)">
              {assessment.riskScore.toFixed(4)}
            </strong>
          </div>
          <div className={`${surface} rounded-sm p-2`}>
            <span className="text-[10px] text-(--admin-text-muted)">
              Trạng thái
            </span>
            <div className="mt-1">
              <TransactionStatusIcon status={transaction?.status} />
            </div>
          </div>
        </div>
        <dl className="mt-4 grid gap-2 text-[12px]">
          <div className="flex justify-between gap-4">
            <dt className="text-(--admin-text-muted)">Số tiền</dt>
            <dd className={`${mono} text-right text-(--admin-text)`}>
              {transaction
                ? formatMoney(transaction.amount, transaction.currency)
                : "—"}
            </dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt className="text-(--admin-text-muted)">Tài khoản gửi</dt>
            <dd className="text-right text-(--admin-text)">
              {transaction?.sourceAccountNo || "—"}
            </dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt className="text-(--admin-text-muted)">Người nhận</dt>
            <dd className="text-right text-(--admin-text)">
              {transaction?.destinationName || "—"}
            </dd>
          </div>
          <div className="flex justify-between gap-4">
            <dt className="text-(--admin-text-muted)">Chấm điểm lúc</dt>
            <dd className="text-right text-(--admin-text)">
              {formatDateTime(assessment.receivedAt)}
            </dd>
          </div>
        </dl>
        <div className="mt-5 border-t border-(--admin-border) pt-4">
          <h3 className="text-[13px] font-semibold text-(--admin-text)">
            Diễn biến
          </h3>
          <ol className="mt-3 grid gap-3">
            {timeline.map((item) => (
              <li key={item.label} className="flex gap-2 text-[12px]">
                <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-(--admin-primary)" />
                <div>
                  <strong className="font-medium text-(--admin-text)">
                    {item.label}
                  </strong>
                  <div className="text-(--admin-text-muted)">
                    {item.current ? "Hiện tại" : formatDateTime(item.value)}
                  </div>
                </div>
              </li>
            ))}
          </ol>
        </div>
      </aside>
    </div>
  );
}

export function MonitorPage() {
  const [range, setRange] = useState<TimeRange>("24h");
  const [response, setResponse] = useState<MonitorDashboardResponse | null>(
    null,
  );
  const [isLoading, setIsLoading] = useState(true);
  const [showSkeleton, setShowSkeleton] = useState(false);
  const [skeletonForRequest, setSkeletonForRequest] = useState(true);
  const skeletonOnLoadRef = useRef(true);
  const skeletonVisibleAtRef = useRef<number | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [requestVersion, setRequestVersion] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [selectedAssessment, setSelectedAssessment] =
    useState<RiskAssessment | null>(null);
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
        if (
          shouldAdvanceMonitorCursor(
            streamCursorRef.current,
            nextResponse.cursor,
          )
        ) {
          streamCursorRef.current = nextResponse.cursor;
        }
        setResponse(nextResponse);
      })
      .catch((requestError: unknown) => {
        if (!active) return;
        if (
          requestError instanceof Error &&
          "status" in requestError &&
          [401, 403].includes((requestError as { status?: number }).status ?? 0)
        ) {
          clearAdminSession();
          navigate(routes.login);
          setStreamStatus("Phiên đăng nhập hết hạn");
          return;
        }
        requestFailed = true;
        streamCursorRef.current = "";
        setResponse(null);
        setError(
          requestError instanceof Error
            ? requestError.message
            : "Vui lòng thử lại sau.",
        );
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
        const remainingVisibleTime =
          skeletonVisibleAt === null
            ? 0
            : Math.max(
                0,
                MIN_SKELETON_VISIBLE_MS -
                  (performance.now() - skeletonVisibleAt),
              );

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
    let hasConnected = false;
    if (!streamCursorRef.current)
      streamCursorRef.current = initialResponse.cursor;
    let delay = 1000;

    const run = async () => {
      while (!cancelled) {
        controller = new AbortController();
        setStreamStatus("Đang kết nối");
        try {
          await consumeRiskStream(
            streamCursorRef.current,
            (event, nextCursor) => {
              if (cancelled) return;
              if (nextCursor) streamCursorRef.current = nextCursor;
              setStreamStatus("Trực tiếp");
              if (event.kind === "transaction_status") {
                const statusEvent = event.event;
                const updateAssessment = (
                  assessment: RiskAssessment,
                ): RiskAssessment => {
                  if (
                    assessment.transactionId !== statusEvent.transactionId ||
                    !assessment.transaction
                  )
                    return assessment;
                  return {
                    ...assessment,
                    transaction: {
                      ...assessment.transaction,
                      status: statusEvent.status,
                      otpExpiresAt: statusEvent.otpExpiresAt,
                      otpVerifiedAt: statusEvent.otpVerifiedAt,
                      completedAt: statusEvent.completedAt,
                      failedAt: statusEvent.failedAt,
                      cancelledAt: statusEvent.cancelledAt,
                    },
                  };
                };
                setSelectedAssessment((current) =>
                  current ? updateAssessment(current) : current,
                );
                setResponse((current) => {
                  if (!current) return current;
                  const previousAssessment =
                    current.snapshot.recentAssessments.find(
                      (item) =>
                        item.transactionId === statusEvent.transactionId,
                    );
                  const previousStatus =
                    previousAssessment?.transaction?.status;
                  const transactionStatus = {
                    ...(current.snapshot.transactionStatus ?? {}),
                  };
                  if (previousStatus && previousStatus !== statusEvent.status) {
                    transactionStatus[previousStatus] = Math.max(
                      0,
                      (transactionStatus[previousStatus] ?? 0) - 1,
                    );
                    transactionStatus[statusEvent.status] =
                      (transactionStatus[statusEvent.status] ?? 0) + 1;
                  }
                  return {
                    ...current,
                    snapshot: {
                      ...current.snapshot,
                      transactionStatus,
                      recentAssessments:
                        current.snapshot.recentAssessments.map(
                          updateAssessment,
                        ),
                    },
                  };
                });
                return;
              }
              if (event.kind === "alert_count") return;
              const assessmentEvent = event.event;
              setResponse((current) => {
                if (
                  !current ||
                  current.snapshot.recentAssessments.some(
                    (item) =>
                      item.assessmentId === assessmentEvent.assessmentId,
                  )
                )
                  return current;
                const previous = current.snapshot;
                const total = previous.totalAssessments + 1;
                const anomalous =
                  previous.anomalousCount + assessmentEvent.predictedLabel;
                const normalCount =
                  previous.normalCount +
                  (assessmentEvent.riskLevel === "NORMAL" ? 1 : 0);
                const warningCount =
                  previous.warningCount +
                  (assessmentEvent.riskLevel === "WARNING" ? 1 : 0);
                const blockCount =
                  previous.blockCount +
                  (assessmentEvent.riskLevel === "BLOCK" ? 1 : 0);
                const recentAssessments = [
                  ...previous.recentAssessments,
                  assessmentEvent,
                ].slice(-50);
                const existingVersion = previous.modelVersions.find(
                  (item) => item.label === assessmentEvent.modelVersion,
                );
                const modelVersions = existingVersion
                  ? previous.modelVersions.map((item) =>
                      item.label === assessmentEvent.modelVersion
                        ? { ...item, count: item.count + 1 }
                        : item,
                    )
                  : [
                      ...previous.modelVersions,
                      { label: assessmentEvent.modelVersion, count: 1 },
                    ];
                const bucketAt = new Date(assessmentEvent.receivedAt);
                bucketAt.setMinutes(0, 0, 0);
                const timeseries = [...previous.timeseries];
                const last = timeseries[timeseries.length - 1];
                if (
                  last &&
                  new Date(last.timestamp).getTime() === bucketAt.getTime()
                ) {
                  const count = last.assessmentCount + 1;
                  timeseries[timeseries.length - 1] = {
                    ...last,
                    assessmentCount: count,
                    anomalousCount:
                      last.anomalousCount + assessmentEvent.predictedLabel,
                    averageRiskScore:
                      (last.averageRiskScore * last.assessmentCount +
                        assessmentEvent.riskScore) /
                      count,
                  };
                } else {
                  timeseries.push({
                    timestamp: bucketAt.toISOString(),
                    assessmentCount: 1,
                    anomalousCount: assessmentEvent.predictedLabel,
                    averageRiskScore: assessmentEvent.riskScore,
                  });
                }
                return {
                  ...current,
                  cursor: streamCursorRef.current,
                  updatedAt: assessmentEvent.receivedAt,
                  snapshot: {
                    ...previous,
                    totalAssessments: total,
                    anomalousCount: anomalous,
                    normalCount,
                    warningCount,
                    blockCount,
                    anomalyRate: anomalous / total,
                    averageRiskScore:
                      (previous.averageRiskScore * previous.totalAssessments +
                        assessmentEvent.riskScore) /
                      total,
                    recentAssessments,
                    modelVersions,
                    timeseries,
                  },
                };
              });
            },
            controller.signal,
            () => {
              if (hasConnected) setRequestVersion((version) => version + 1);
              hasConnected = true;
            },
          );
          if (!cancelled) setStreamStatus("Đang kết nối lại");
        } catch (streamError) {
          if (
            cancelled ||
            (streamError instanceof DOMException &&
              streamError.name === "AbortError")
          )
            return;
          if (
            streamError instanceof Error &&
            "status" in streamError &&
            [401, 403].includes(
              (streamError as { status?: number }).status ?? 0,
            )
          ) {
            clearAdminSession();
            navigate(routes.login);
            setStreamStatus("Phiên đăng nhập hết hạn");
            return;
          }
          setStreamStatus("Mất kết nối");
        }
        if (cancelled) return;
        retryTimer = window.setTimeout(() => {
          void run();
        }, delay);
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
    <div
      className="grid gap-4"
      aria-busy={busy}
      aria-label="Dashboard giám sát rủi ro"
    >
      <MonitorHeader
        range={range}
        onRangeChange={changeRange}
        isRefreshing={busy}
        onRefresh={refresh}
        streamStatus={streamStatus}
      />
      {error && !snapshot ? (
        <DashboardError message={error} onRetry={refresh} />
      ) : showSkeleton && skeletonForRequest ? (
        <MonitorDashboardSkeleton />
      ) : snapshot ? (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-6">
            {metricsForSnapshot(snapshot).map((metric) => (
              <MetricCard key={metric.label} metric={metric} />
            ))}
          </div>
          <div className="grid grid-cols-12 items-stretch gap-4">
            <div className="col-span-12 h-full xl:col-span-8">
              <RiskTrendChart series={snapshot.timeseries} range={range} />
            </div>
            <div className="col-span-12 h-full xl:col-span-4">
              <RiskBreakdown snapshot={snapshot} />
            </div>
          </div>
          <div className="grid grid-cols-12 gap-4">
            <div className="col-span-12">
              <RiskRequestsTable
                assessments={snapshot.recentAssessments}
                onSelect={setSelectedAssessment}
              />
            </div>
          </div>
          <div
            className={`${mono} flex items-center justify-end gap-1 pt-1 text-[10px] text-(--admin-text-muted)`.trim()}
          >
            <FontAwesomeIcon icon={faCircleExclamation} aria-hidden="true" />
            Cập nhật lúc {response.updatedAt}
            {response.dataStatus === "stale" ? " · DỮ LIỆU CŨ" : ""}
          </div>
        </>
      ) : null}
      {selectedAssessment && (
        <AssessmentDetailsDrawer
          assessment={selectedAssessment}
          onClose={() => setSelectedAssessment(null)}
        />
      )}
    </div>
  );
}

function metricsForSnapshot(snapshot: MonitorSnapshot): Metric[] {
  const statuses = snapshot.transactionStatus ?? {};
  const awaitingOTP = statuses.awaiting_otp ?? 0;
  const completed = statuses.success ?? 0;
  const notCompleted = (statuses.failed ?? 0) + (statuses.cancelled ?? 0);
  return [
    {
      label: "Yêu cầu đã chấm điểm",
      value: String(snapshot.totalAssessments),
      note: "Trong khoảng thời gian đã chọn",
      icon: faBell,
    },
    {
      label: "Chờ OTP",
      value: String(awaitingOTP),
      note: "Chưa hoàn tất",
      tone: "primary",
      icon: faClock,
    },
    {
      label: "Đã hoàn tất",
      value: String(completed),
      note: "Đã xác nhận",
      tone: "primary",
      icon: faCircleCheck,
    },
    {
      label: "Chưa hoàn tất",
      value: String(notCompleted),
      note: "Bị từ chối hoặc đã huỷ",
      icon: faCircleXmark,
    },
    {
      label: "Cảnh báo",
      value: String(snapshot.warningCount),
      note: "Cần lưu ý",
      tone: "warning",
      icon: faCircleExclamation,
    },
    {
      label: "Chặn",
      value: String(snapshot.blockCount),
      note: "Cần kiểm tra",
      tone: "danger",
      icon: faCircleExclamation,
    },
  ];
}
