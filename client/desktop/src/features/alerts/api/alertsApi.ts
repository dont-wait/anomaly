import {
  API_BASE_URL,
  readAdminSession,
} from "@/features/auth/api/adminSession";
import { getMonitorDashboard as fetchMonitorDashboard } from "@/features/monitor/api/monitorApi";
import type { TimeRange } from "@/features/monitor/model";

export type { TimeRange };

export type HandledFilter = "unhandled" | "handled" | "all";

export function getMonitorDashboard(range: TimeRange, handled: HandledFilter) {
  return fetchMonitorDashboard(range, undefined, handled);
}

export async function updateAssessmentHandled(
  assessmentId: string,
  handled: boolean,
): Promise<void> {
  const session = readAdminSession();
  const response = await fetch(
    `${API_BASE_URL}/api/admin/risk-assessments/${encodeURIComponent(assessmentId)}/handled`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        ...(session ? { Authorization: `Bearer ${session.accessToken}` } : {}),
      },
      body: JSON.stringify({ handled }),
    },
  );
  if (!response.ok) throw new Error("Không thể cập nhật trạng thái cảnh báo.");
}

export async function updateAccountStatus(
  accountId: string,
  status: "active" | "locked",
  accountNo?: string,
  reason?: string,
): Promise<void> {
  const session = readAdminSession();
  const response = await fetch(
    `${API_BASE_URL}/api/admin/accounts/${encodeURIComponent(accountId)}/status`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
        ...(session ? { Authorization: `Bearer ${session.accessToken}` } : {}),
      },
      body: JSON.stringify({ status, accountNo, reason }),
    },
  );
  if (!response.ok) throw new Error("Không thể cập nhật trạng thái tài khoản.");
}
