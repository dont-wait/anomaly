import { useRef, useState } from "react";
import type { Plugin, ChartOptions } from "chart.js";
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
} from "chart.js";
import { Line } from "react-chartjs-2";
import type { RiskSeriesPoint } from "../mock/monitorData";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Tooltip);

const chartColors = {
  border: "#b4c5ff",
  fill: "rgba(37, 99, 235, 0.13)",
  grid: "#272b33",
  muted: "#8d90a0",
  danger: "#d52022",
  surface: "#2a2a2c",
  text: "#e5e1e4",
};

interface TooltipState {
  left: number;
  top: number;
  timestamp: string;
  riskScore: number;
  alertCount: number;
  highRiskCount: number;
}

function formatTime(timestamp: string) {
  return new Intl.DateTimeFormat("vi-VN", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZone: "Asia/Ho_Chi_Minh",
  }).format(new Date(timestamp));
}

function createRiskMarkerPlugin(peakIndex: number): Plugin<"line"> {
  return {
    id: "risk-marker",
    beforeDatasetsDraw(chart) {
      chart.ctx.save();
      chart.ctx.shadowColor = "rgba(180, 197, 255, 0.22)";
      chart.ctx.shadowBlur = 8;
      chart.ctx.shadowOffsetY = 1;
    },
    afterDatasetsDraw(chart) {
      const meta = chart.getDatasetMeta(0);
      const peakElement = meta.data[peakIndex];
      const activeElement = chart.tooltip?.getActiveElements()[0]?.element;
      const { ctx, chartArea } = chart;

      ctx.save();
      if (peakElement) {
        const peak = peakElement.getProps(["x", "y"], true);
        ctx.setLineDash([]);
        ctx.fillStyle = chartColors.danger;
        ctx.beginPath();
        ctx.arc(peak.x, peak.y, 3.5, 0, Math.PI * 2);
        ctx.fill();
      }

      if (activeElement) {
        const active = activeElement.getProps(["x", "y"], true);
        ctx.setLineDash([3, 3]);
        ctx.lineWidth = 1;
        ctx.strokeStyle = "rgba(180, 197, 255, 0.75)";
        ctx.beginPath();
        ctx.moveTo(active.x, chartArea.top);
        ctx.lineTo(active.x, chartArea.bottom);
        ctx.stroke();
      }
      ctx.restore();
      chart.ctx.restore();
    },
  };
}

interface RiskTrendChartProps {
  series: RiskSeriesPoint[];
}

export function RiskTrendChart({ series }: RiskTrendChartProps) {
  const [hoverTooltip, setHoverTooltip] = useState<TooltipState | null>(null);
  const tooltipRef = useRef<TooltipState | null>(null);
  const hasData = series.length > 0;
  const values = series.map((point) => point.riskScoreAverage);
  const peakIndex = hasData ? values.indexOf(Math.max(...values)) : -1;
  const peakPoint = series[peakIndex];
  const labels = series.map((point) => formatTime(point.timestamp));

  const updateTooltip = (next: TooltipState | null) => {
    const previous = tooltipRef.current;
    if (
      previous?.left === next?.left &&
      previous?.top === next?.top &&
      previous?.timestamp === next?.timestamp &&
      previous?.riskScore === next?.riskScore &&
      previous?.alertCount === next?.alertCount &&
      previous?.highRiskCount === next?.highRiskCount
    ) {
      return;
    }
    tooltipRef.current = next;
    setHoverTooltip(next);
  };

  const data = {
    labels,
    datasets: [
      {
        label: "",
        data: values,
        borderColor: chartColors.border,
        backgroundColor: chartColors.fill,
        borderWidth: 1.5,
        fill: true,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointHoverBackgroundColor: chartColors.border,
        pointHoverBorderColor: chartColors.surface,
        pointHoverBorderWidth: 2,
        tension: 0.22,
        cubicInterpolationMode: "monotone" as const,
      },
    ],
  };

  const options: ChartOptions<"line"> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: { duration: 250 },
    interaction: { mode: "index", intersect: false },
    layout: { padding: { top: 8, right: 8, bottom: 0, left: 0 } },
    plugins: {
      legend: { display: false },
      tooltip: {
        enabled: false,
        external: ({ chart, tooltip }) => {
          const item = tooltip.dataPoints?.[0];
          const point = item ? series[item.dataIndex] : undefined;
          if (!point || tooltip.opacity === 0) {
            updateTooltip(null);
            return;
          }
          updateTooltip({
            left: Math.min(86, Math.max(14, (tooltip.caretX / chart.width) * 100)),
            top: Math.max(20, (tooltip.caretY / chart.height) * 100 - 8),
            timestamp: formatTime(point.timestamp),
            riskScore: point.riskScoreAverage,
            alertCount: point.alertCount,
            highRiskCount: point.highRiskCount,
          });
        },
      },
    },
    scales: {
      x: {
        grid: { display: false },
        border: { display: false },
        ticks: {
          color: chartColors.muted,
          maxTicksLimit: 7,
          maxRotation: 0,
          padding: 4,
          font: { family: "Geist Mono", size: 10 },
        },
      },
      y: {
        min: 0,
        max: 100,
        grid: { color: chartColors.grid, drawTicks: false },
        border: { display: false },
        ticks: { display: false, stepSize: 25 },
      },
    },
  };

  return (
    <section className="bg-(--admin-surface-subtle) flex min-h-75 flex-col justify-between rounded-(--admin-radius-md) p-3" data-testid="risk-trend-chart">
      <div className="flex flex-col justify-between gap-2 pb-2 md:flex-row md:items-center">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-[16px] leading-5 font-semibold text-(--admin-text)">Risk Trend Timeline (24h)</h2>
          <span className="font-(family-name:--admin-font-mono) text-[10px] tracking-[0.03em] text-(--admin-text-muted)">ANOMALY INDEX / 30m BINS</span>
        </div>
        <div className="font-(family-name:--admin-font-mono) flex flex-wrap items-center gap-2 text-[10px] text-(--admin-text-muted)">
          {peakPoint && <span className="rounded-sm bg-[#3b2930] px-1.5 py-1 text-[#ffb4ab]">PEAK {peakPoint.riskScoreAverage}pt · {formatTime(peakPoint.timestamp)} UTC+7</span>}
          {hasData && <><span className="flex items-center gap-1"><span className="h-0.5 w-3 bg-(--admin-primary)" />Thể tích rủi ro</span><span className="flex items-center gap-1"><span className="h-2 w-2 rounded-sm bg-(--admin-danger)" />Ngưỡng cực đại</span></>}
        </div>
      </div>
      <div className="relative my-2 h-50 w-full">
        {hasData ? <Line
          data={data}
          options={options}
          plugins={[createRiskMarkerPlugin(peakIndex)]}
          aria-label="Biểu đồ điểm rủi ro trong khoảng thời gian đã chọn"
        /> : <div className="absolute inset-0 grid place-items-center rounded-sm border border-dashed border-(--admin-border) bg-(--admin-surface) px-4 text-center" aria-label="Chưa có dữ liệu" role="status">
          <div>
            <strong className="block text-[13px] font-semibold text-(--admin-text)">Chưa có dữ liệu</strong>
            <span className="mt-1 block text-[11px] text-(--admin-text-muted)">Thử chọn khoảng thời gian khác.</span>
          </div>
        </div>}
        {hasData && hoverTooltip && (
          <div
            className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full rounded-sm border border-(--admin-border) bg-(--admin-surface-hover) px-2.5 py-2 shadow-md"
            style={{ left: `${hoverTooltip.left}%`, top: `${hoverTooltip.top}%` }}
            role="status"
            aria-live="polite"
          >
            <strong className="font-(family-name:--admin-font-mono) block whitespace-nowrap text-[10px] text-(--admin-primary)">{hoverTooltip.timestamp} UTC+7</strong>
            <span className="font-(family-name:--admin-font-mono) mt-1 block whitespace-nowrap text-[10px] text-(--admin-text)">Risk score: {hoverTooltip.riskScore}pt</span>
            <span className="font-(family-name:--admin-font-mono) block whitespace-nowrap text-[9px] text-(--admin-text-muted)">{hoverTooltip.alertCount} alerts · {hoverTooltip.highRiskCount} high-risk</span>
          </div>
        )}
      </div>
      <div className="font-(family-name:--admin-font-mono) flex items-center justify-between gap-2 pt-2 text-[10px] text-(--admin-text-muted)">
        {hasData ? <><span>00:00</span><span>04:00</span><span>08:00</span><span>12:00</span><span>16:00</span><span>20:00</span><span>HIỆN TẠI (23:59)</span></> : <span className="mx-auto">Không có điểm dữ liệu trong khoảng thời gian này</span>}
      </div>
    </section>
  );
}
