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
  fill: "rgba(37, 99, 235, 0.22)",
  grid: "#272b33",
  muted: "#8d90a0",
  danger: "#d52022",
  surface: "#2a2a2c",
  text: "#e5e1e4",
};

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
    afterDatasetsDraw(chart) {
    const meta = chart.getDatasetMeta(0);
    const peakElement = meta.data[peakIndex];
    if (!peakElement) return;

    const activeElement = chart.tooltip?.getActiveElements()[0]?.element;
    const { ctx, chartArea } = chart;
    const peak = peakElement.getProps(["x", "y"], true);

    ctx.save();
    ctx.setLineDash([]);
    ctx.fillStyle = chartColors.danger;
    ctx.beginPath();
    ctx.arc(peak.x, peak.y, 4, 0, Math.PI * 2);
    ctx.fill();

    if (activeElement) {
      const active = activeElement.getProps(["x", "y"], true);
      ctx.setLineDash([3, 3]);
      ctx.strokeStyle = "rgba(180, 197, 255, 0.8)";
      ctx.beginPath();
      ctx.moveTo(active.x, chartArea.top);
      ctx.lineTo(active.x, chartArea.bottom);
      ctx.stroke();
    }
    ctx.restore();
    },
  };
}

interface RiskTrendChartProps {
  series: RiskSeriesPoint[];
}

export function RiskTrendChart({ series }: RiskTrendChartProps) {
  const values = series.map((point) => point.riskScoreAverage);
  const peakIndex = values.indexOf(Math.max(...values));
  const labels = series.map((point) => formatTime(point.timestamp));

  const data = {
    labels,
    datasets: [
      {
        label: "",
        data: values,
        borderColor: chartColors.border,
        backgroundColor: chartColors.fill,
        borderWidth: 2,
        fill: true,
        pointRadius: 0,
        pointHoverRadius: 4,
        pointHoverBackgroundColor: chartColors.border,
        pointHoverBorderColor: chartColors.surface,
        pointHoverBorderWidth: 2,
        tension: 0.08,
      },
    ],
  };

  const options: ChartOptions<"line"> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: { duration: 350 },
    interaction: { mode: "index", intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        enabled: true,
        displayColors: false,
        backgroundColor: chartColors.surface,
        borderColor: "#434655",
        borderWidth: 1,
        titleColor: chartColors.border,
        bodyColor: chartColors.text,
        padding: 10,
        titleFont: { family: "Geist Mono", size: 10, weight: 600 },
        bodyFont: { family: "Geist Mono", size: 10 },
        callbacks: {
          title: (items) => `${formatTime(series[items[0]?.dataIndex ?? 0]?.timestamp ?? "")} UTC+7`,
          label: (item) => {
            const point = series[item.dataIndex];
            return [
              `Risk score: ${item.parsed.y}pt`,
              `${point.alertCount} alerts`,
              `${point.highRiskCount} high-risk`,
            ];
          },
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
          font: { family: "Geist Mono", size: 10 },
        },
      },
      y: {
        min: 0,
        max: 100,
        grid: { color: chartColors.grid, drawTicks: false },
        border: { display: false },
        ticks: { display: false },
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
        <div className="font-(family-name:--admin-font-mono) flex flex-wrap items-center gap-3 text-[10px] text-(--admin-text-muted)">
          <span className="flex items-center gap-1"><span className="h-0.5 w-3 bg-(--admin-primary)" />Thể tích rủi ro</span>
          <span className="flex items-center gap-1"><span className="h-2 w-2 rounded-sm bg-(--admin-danger)" />Ngưỡng cực đại (92pt)</span>
        </div>
      </div>
      <div className="relative my-2 h-50 w-full">
        <Line
          data={data}
          options={options}
          plugins={[createRiskMarkerPlugin(peakIndex)]}
          aria-label="Biểu đồ điểm rủi ro trong khoảng thời gian đã chọn"
        />
      </div>
      <div className="font-(family-name:--admin-font-mono) flex items-center justify-between gap-2 pt-2 text-[10px] text-(--admin-text-muted)">
        <span>00:00</span><span>04:00</span><span>08:00</span><span>12:00</span><span>16:00</span><span>20:00</span><span>HIỆN TẠI (23:59)</span>
      </div>
    </section>
  );
}
