import {
  dashboardData,
  type MonitorDashboardResponse,
  type TimeRange,
} from "../mock/monitorData";

const MOCK_DELAY_MS = 180;
let shouldFail = false;

export class MonitorApiError extends Error {
  constructor(message = "Không thể tải dữ liệu giám sát rủi ro") {
    super(message);
    this.name = "MonitorApiError";
  }
}

export function setMockMonitorFailure(value: boolean) {
  shouldFail = value;
}

export async function getMonitorDashboard(
  range: TimeRange,
): Promise<MonitorDashboardResponse> {
  await new Promise((resolve) => window.setTimeout(resolve, MOCK_DELAY_MS));

  if (shouldFail) {
    throw new MonitorApiError();
  }

  return {
    range,
    updatedAt: "23:59:12",
    dataStatus: "fresh",
    snapshot: dashboardData[range],
  };
}
