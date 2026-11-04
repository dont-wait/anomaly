import { useMemo, useState } from "react";
import { Bar, Doughnut } from "react-chartjs-2";
import {
  ArcElement,
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  Tooltip,
} from "chart.js";
import type { TransactionReportHourly, TransactionReportSummary } from "./api/transactionsReportApi";

ChartJS.register(ArcElement, BarElement, CategoryScale, Legend, LinearScale, Tooltip);

type ChartMode = "count" | "amount";

function money(value: number) {
  return `${value.toLocaleString("vi-VN")} VND`;
}

export function DailyReportChart({ hourly, date, summary }: { hourly: TransactionReportHourly[]; date: string; summary?: TransactionReportSummary }) {
  const [mode, setMode] = useState<ChartMode>("count");
  const labels = hourly.map((point) => point.hour);
  const barData = useMemo(() => ({
    labels,
    datasets: [
      { label: "Hoàn tất", data: hourly.map((point) => mode === "count" ? point.success : point.amount), backgroundColor: "#75d9a4", borderRadius: 3, stack: "status" },
      { label: "Thất bại", data: hourly.map((point) => mode === "count" ? point.failed : 0), backgroundColor: "#ff8f86", borderRadius: 3, stack: "status" },
      { label: "Đang xử lý", data: hourly.map((point) => mode === "count" ? point.pending : 0), backgroundColor: "#f6d58a", borderRadius: 3, stack: "status" },
    ],
  }), [hourly, labels, mode]);
  const pieData = {
    labels: ["Hoàn tất", "Thất bại khác", "Bị chặn"],
    datasets: [{ data: [summary?.successCount || 0, Math.max(0, (summary?.failedCount || 0) - (summary?.blockedCount || 0)), summary?.blockedCount || 0], backgroundColor: ["#75d9a4", "#ff8f86", "#f6d58a"], borderColor: "#202125", borderWidth: 3, hoverOffset: 5 }],
  };
  const barOptions = {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { mode: "index" as const, intersect: false },
    plugins: { legend: { position: "top" as const, align: "end" as const, labels: { color: "#8d90a0", boxWidth: 10, font: { size: 10 } } }, tooltip: { callbacks: { label: (context: { dataset: { label?: string }; parsed: { y: number } }) => `${context.dataset.label}: ${mode === "amount" ? money(context.parsed.y) : context.parsed.y}` } } },
    scales: { x: { stacked: true, grid: { display: false }, ticks: { color: "#8d90a0", maxTicksLimit: 12 } }, y: { stacked: true, beginAtZero: true, grid: { color: "#272b33" }, ticks: { color: "#8d90a0", precision: 0, callback: (tick: string | number) => mode === "amount" ? `${Number(tick) / 1000000}M` : tick } } },
  };
  const pieOptions = { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: "right" as const, labels: { color: "#8d90a0", boxWidth: 10, font: { size: 10 } } } } };

  return <section className="bg-(--admin-surface-subtle) rounded-(--admin-radius-md) p-4" data-testid="daily-report-chart">
    <div className="mb-4 flex flex-wrap items-center justify-between gap-2"><div><h2 className="text-[16px] font-semibold text-(--admin-text)">Tổng quan trong ngày</h2><p className="text-[11px] text-(--admin-text-muted)">Phân bổ theo giờ · {date}</p></div><div className="bg-(--admin-surface) flex rounded-sm p-0.5" role="group" aria-label="Chỉ số biểu đồ">{(["count", "amount"] as const).map((value) => <button key={value} type="button" aria-pressed={mode === value} onClick={() => setMode(value)} className={`rounded-sm px-2.5 py-1 text-[10px] font-semibold ${mode === value ? "bg-(--admin-primary) text-white" : "text-(--admin-text-muted) hover:text-(--admin-text)"}`}>{value === "count" ? "Số lượng" : "Doanh số"}</button>)}</div></div>
    <div className="grid gap-5 lg:grid-cols-[minmax(0,1.7fr)_minmax(220px,0.8fr)]"><div className="h-70 min-w-0">{hourly.length ? <Bar data={barData} options={barOptions} /> : <div className="grid h-full place-items-center text-[12px] text-(--admin-text-muted)">Chưa có giao dịch trong ngày.</div>}</div><div className="h-70 min-w-0"><Doughnut data={pieData} options={pieOptions} /></div></div>
  </section>;
}
