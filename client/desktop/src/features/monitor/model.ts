export type TimeRange = "24h" | "7d" | "30d";

export interface RiskAssessment {
  assessmentId: string;
  transactionId: string;
  riskScore: number;
  predictedLabel: 0 | 1;
  riskLevel: "NORMAL" | "WARNING" | "BLOCK";
  decision: "ALLOW" | "BLOCK";
  /** Server-derived explanation of `decision`; absent for old records that cannot be derived. */
  decisionReason?: string;
  /** Unverified, unsigned notes on unusual inputs from the model service. Not feature attribution. */
  topFactors?: RiskFactor[];
  threshold: number;
  modelVersion: string;
  featureVersion: string;
  receivedAt: string;
  handled?: boolean;
  handledAt?: string;
  handledBy?: string;
  transaction: RiskTransactionContext | null;
}

export interface RiskFactor {
  feature: string;
  /** Standard deviations from the training mean (numeric features). */
  zScore?: number;
  /** Raw value for binary flags and rare fractions. */
  value?: number;
}

export interface RiskTransactionContext {
  transactionId: string;
  sourceAccountId?: string;
  sourceAccountStatus?: string;
  sourceAccountNo: string;
  sourceAccountName: string;
  destinationAccountId?: string;
  destinationAccountStatus?: string;
  destinationAccountNo: string;
  destinationName: string;
  amount: number;
  fee: number;
  currency: string;
  channel: string;
  status: string;
  createdAt: string;
  otpExpiresAt?: string;
  otpVerifiedAt?: string;
  completedAt?: string;
  failedAt?: string;
  cancelledAt?: string;
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
  range: TimeRange | string;
  updatedAt: string;
  dataStatus: "fresh" | "stale";
  cursor: string;
  snapshot: MonitorSnapshot;
}

export type RiskAssessmentEvent = RiskAssessment;

export interface TransactionStatusEvent {
  transactionId: string;
  status: string;
  statusAt: string;
  otpExpiresAt?: string;
  otpVerifiedAt?: string;
  completedAt?: string;
  failedAt?: string;
  cancelledAt?: string;
}

export type RiskStreamEvent =
  | { kind: "assessment"; event: RiskAssessmentEvent }
  | { kind: "transaction_status"; event: TransactionStatusEvent }
  | { kind: "alert_count"; count: number };

export const ranges: { value: TimeRange; label: string }[] = [
  { value: "24h", label: "24h" },
  { value: "7d", label: "7 ngày" },
  { value: "30d", label: "30 ngày" },
];
