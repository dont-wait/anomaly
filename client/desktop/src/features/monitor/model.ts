export type TimeRange = "24h" | "7d" | "30d";

export interface RiskAssessment {
  assessmentId: string;
  transactionId: string;
  riskScore: number;
  predictedLabel: 0 | 1;
  threshold: number;
  modelVersion: string;
  featureVersion: string;
  receivedAt: string;
}

export interface RiskSeriesPoint {
  timestamp: string;
  assessmentCount: number;
  anomalousCount: number;
  averageRiskScore: number;
}

export interface ModelVersionCount {
  label: string;
  count: number;
}

export interface MonitorSnapshot {
  totalAssessments: number;
  anomalousCount: number;
  anomalyRate: number;
  averageRiskScore: number;
  timeseries: RiskSeriesPoint[];
  recentAssessments: RiskAssessment[];
  modelVersions: ModelVersionCount[];
}

export interface MonitorDashboardResponse {
  range: TimeRange;
  updatedAt: string;
  dataStatus: "fresh" | "stale";
  cursor: string;
  snapshot: MonitorSnapshot;
}

export type RiskAssessmentEvent = RiskAssessment;

export const ranges: { value: TimeRange; label: string }[] = [
  { value: "24h", label: "24h" },
  { value: "7d", label: "7 ngày" },
  { value: "30d", label: "30 ngày" },
];
