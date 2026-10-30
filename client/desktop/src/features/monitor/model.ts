export type TimeRange = "24h" | "7d" | "30d";

export interface RiskAssessment {
  assessmentId: string;
  transactionId: string;
  riskScore: number;
  predictedLabel: 0 | 1;
  riskLevel: "NORMAL" | "WARNING" | "BLOCK";
  decision: "ALLOW" | "BLOCK";
  threshold: number;
  modelVersion: string;
  featureVersion: string;
  receivedAt: string;
  transaction: RiskTransactionContext | null;
}

export interface RiskTransactionContext {
  transactionId: string;
  sourceAccountNo: string;
  sourceAccountName: string;
  destinationAccountNo: string;
  destinationName: string;
  amount: number;
  fee: number;
  currency: string;
  channel: string;
  status: string;
  createdAt: string;
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
  normalCount: number;
  warningCount: number;
  blockCount: number;
  transactionStatus?: Record<string, number>;
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
