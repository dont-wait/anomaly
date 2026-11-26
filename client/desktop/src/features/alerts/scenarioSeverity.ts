import type { RiskScenarioSeverity } from "@/features/monitor/model";

export const severityLabel: Record<RiskScenarioSeverity, string> = {
  high: "Cao",
  medium: "Trung bình",
  low: "Thấp",
};

export const severityClass: Record<RiskScenarioSeverity, string> = {
  high: "bg-(--admin-danger-soft) text-(--admin-danger)",
  medium: "bg-(--admin-warning-soft) text-(--admin-warning-foreground)",
  low: "bg-(--admin-surface-hover) text-(--admin-text-secondary)",
};
