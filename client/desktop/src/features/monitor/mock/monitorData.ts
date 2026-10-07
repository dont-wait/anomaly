import type { IconDefinition } from "@fortawesome/fontawesome-svg-core";
import {
  faBell,
  faChartLine,
  faMagnifyingGlass,
  faMoneyBillTransfer,
  faTriangleExclamation,
} from "@fortawesome/free-solid-svg-icons";

export type TimeRange = "24h" | "7d" | "30d";
export type MetricTone = "default" | "danger" | "primary";
export type Trend = "up" | "down" | "flat";

export interface Metric {
  label: string;
  value: string;
  change?: string;
  note: string;
  trend?: Trend;
  tone?: MetricTone;
  icon: IconDefinition;
}

export interface AccountRisk {
  accountNo: string;
  customerName: string;
  score: number;
  alerts: number;
  amountAtRisk: number;
}

export interface AlertEvent {
  severity: "CRITICAL" | "HIGH" | "MEDIUM";
  title: string;
  accountNo: string;
  detail: string;
  time: string;
}

export interface RiskSeriesPoint {
  timestamp: string;
  alertCount: number;
  highRiskCount: number;
  riskScoreAverage: number;
}

export interface DashboardSnapshot {
  metrics: Metric[];
  timeseries: RiskSeriesPoint[];
  severities: { label: string; value: number; tone: string }[];
  detectionVectors: { label: string; value: number; tone: string }[];
  accounts: AccountRisk[];
  events: AlertEvent[];
}

export interface MonitorDashboardResponse {
  range: TimeRange;
  updatedAt: string;
  dataStatus: "fresh" | "stale";
  snapshot: DashboardSnapshot;
}

export const ranges: { value: TimeRange; label: string }[] = [
  { value: "24h", label: "24h" },
  { value: "7d", label: "7 ngày" },
  { value: "30d", label: "30 ngày" },
];

function createTimeseries(values: number[]): RiskSeriesPoint[] {
  return values.map((riskScoreAverage, index) => {
    const timestamp = new Date("2026-10-01T00:00:00+07:00");
    timestamp.setMinutes(timestamp.getMinutes() + index * 60);

    return {
      timestamp: timestamp.toISOString(),
      alertCount: Math.max(1, Math.round(riskScoreAverage / 4)),
      highRiskCount: Math.max(0, Math.round(riskScoreAverage / 20)),
      riskScoreAverage,
    };
  });
}

export const dashboardData: Record<TimeRange, DashboardSnapshot> = {
  "24h": {
    metrics: [
      { label: "Cảnh báo mới (24h)", value: "38", change: "+12.5%", trend: "up", note: "vs chu kỳ trước", icon: faBell },
      { label: "Critical / High mở", value: "07", change: "-8.2%", trend: "down", note: "Yêu cầu xử lý tức thì", tone: "danger", icon: faTriangleExclamation },
      { label: "Tổng giá trị rủi ro", value: "128.500.000 ₫", change: "+4.1%", trend: "up", note: "Tổng tiền trong cảnh báo mở", icon: faMoneyBillTransfer },
      { label: "TK đang điều tra", value: "12", note: "Đã tạm khoá rút 05 TK", icon: faMagnifyingGlass },
      { label: "Tỷ lệ False Positive", value: "8.4%", change: "-2.1%", trend: "down", note: "Mục tiêu quý < 10%", tone: "primary", icon: faChartLine },
    ],
    timeseries: createTimeseries([12, 15, 22, 19, 31, 27, 39, 42, 46, 60, 71, 82, 76, 91, 100, 83, 59, 47, 44, 37, 29, 24, 15, 19]),
    severities: [
      { label: "Critical", value: 7, tone: "danger" },
      { label: "High", value: 14, tone: "primary" },
      { label: "Medium", value: 12, tone: "secondary" },
      { label: "Low", value: 5, tone: "muted" },
    ],
    detectionVectors: [
      { label: "Tần suất giao dịch (Velocity)", value: 42, tone: "primary" },
      { label: "Vi phạm định vị (Geofence)", value: 28, tone: "secondary" },
      { label: "Giao dịch đột biến (High Amount)", value: 18, tone: "outline" },
      { label: "Thiết bị can thiệp (Rooted Device)", value: 12, tone: "muted" },
    ],
    accounts: [
      { accountNo: "ACC-001234", customerName: "Nguyen Van A", score: 92, alerts: 4, amountAtRisk: 45000000 },
      { accountNo: "ACC-008921", customerName: "Tran Thi B", score: 88, alerts: 3, amountAtRisk: 32800000 },
      { accountNo: "ACC-004419", customerName: "Le Hoang D", score: 76, alerts: 2, amountAtRisk: 24500000 },
      { accountNo: "ACC-003112", customerName: "Pham Minh K", score: 71, alerts: 2, amountAtRisk: 16200000 },
      { accountNo: "ACC-007802", customerName: "Vo Thi Ngoc T", score: 64, alerts: 1, amountAtRisk: 10000000 },
    ],
    events: [
      { severity: "CRITICAL", title: "GEO_ANOMALY: 2 IP cách 1400km trong 3m", accountNo: "ACC-001234", detail: "IP: 113.161.44.82 (VN) → 45.112.8.10 (SG)", time: "23:59:12" },
      { severity: "HIGH", title: "VELOCITY: 5 giao dịch tức thì qua OTP", accountNo: "ACC-008921", detail: "Khối lượng: 32.800.000 ₫ trong 45 giây", time: "23:58:45" },
      { severity: "HIGH", title: "DEVICE: Magisk / Fridascan detected", accountNo: "ACC-004419", detail: "Model: SM-G998B (Android 14 Unlocked)", time: "23:57:30" },
      { severity: "MEDIUM", title: "CREDENTIAL_REUSE: 3 TK cùng vân tay trình duyệt", accountNo: "ACC-003112", detail: "Fingerprint: 89f41b...a910", time: "23:55:04" },
    ],
  },
  "7d": {
    metrics: [
      { label: "Cảnh báo mới (7 ngày)", value: "214", change: "+8.1%", trend: "up", note: "vs chu kỳ trước", icon: faBell },
      { label: "Critical / High mở", value: "19", change: "-4.6%", trend: "down", note: "Yêu cầu xử lý tức thì", tone: "danger", icon: faTriangleExclamation },
      { label: "Tổng giá trị rủi ro", value: "742.800.000 ₫", change: "+6.8%", trend: "up", note: "Tổng tiền trong cảnh báo mở", icon: faMoneyBillTransfer },
      { label: "TK đang điều tra", value: "27", note: "Đã tạm khoá rút 09 TK", icon: faMagnifyingGlass },
      { label: "Tỷ lệ False Positive", value: "8.9%", change: "-1.4%", trend: "down", note: "Mục tiêu quý < 10%", tone: "primary", icon: faChartLine },
    ],
    timeseries: createTimeseries([25, 32, 30, 42, 36, 49, 44, 57, 62, 54, 69, 63, 78, 72, 86, 81, 74, 66, 71, 59, 63, 52, 47, 55]),
    severities: [
      { label: "Critical", value: 19, tone: "danger" },
      { label: "High", value: 69, tone: "primary" },
      { label: "Medium", value: 83, tone: "secondary" },
      { label: "Low", value: 43, tone: "muted" },
    ],
    detectionVectors: [
      { label: "Tần suất giao dịch (Velocity)", value: 45, tone: "primary" },
      { label: "Vi phạm định vị (Geofence)", value: 25, tone: "secondary" },
      { label: "Giao dịch đột biến (High Amount)", value: 19, tone: "outline" },
      { label: "Thiết bị can thiệp (Rooted Device)", value: 11, tone: "muted" },
    ],
    accounts: [],
    events: [],
  },
  "30d": {
    metrics: [
      { label: "Cảnh báo mới (30 ngày)", value: "864", change: "+3.4%", trend: "up", note: "vs chu kỳ trước", icon: faBell },
      { label: "Critical / High mở", value: "41", change: "-2.8%", trend: "down", note: "Yêu cầu xử lý tức thì", tone: "danger", icon: faTriangleExclamation },
      { label: "Tổng giá trị rủi ro", value: "2.840.000.000 ₫", change: "+5.2%", trend: "up", note: "Tổng tiền trong cảnh báo mở", icon: faMoneyBillTransfer },
      { label: "TK đang điều tra", value: "58", note: "Đã tạm khoá rút 17 TK", icon: faMagnifyingGlass },
      { label: "Tỷ lệ False Positive", value: "9.1%", change: "-0.7%", trend: "down", note: "Mục tiêu quý < 10%", tone: "primary", icon: faChartLine },
    ],
    timeseries: createTimeseries([34, 37, 43, 40, 47, 52, 48, 59, 56, 64, 70, 66, 74, 82, 79, 88, 91, 84, 76, 72, 69, 61, 65, 58]),
    severities: [
      { label: "Critical", value: 41, tone: "danger" },
      { label: "High", value: 274, tone: "primary" },
      { label: "Medium", value: 356, tone: "secondary" },
      { label: "Low", value: 193, tone: "muted" },
    ],
    detectionVectors: [
      { label: "Tần suất giao dịch (Velocity)", value: 44, tone: "primary" },
      { label: "Vi phạm định vị (Geofence)", value: 27, tone: "secondary" },
      { label: "Giao dịch đột biến (High Amount)", value: 17, tone: "outline" },
      { label: "Thiết bị can thiệp (Rooted Device)", value: 12, tone: "muted" },
    ],
    accounts: [],
    events: [],
  },
};
