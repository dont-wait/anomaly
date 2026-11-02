import {
  API_BASE_URL,
  readAdminSession,
} from "@/features/auth/api/adminSession";
import { getMonitorDashboard } from "@/features/monitor/api/monitorApi";
import type { TimeRange } from "@/features/monitor/model";

export { getMonitorDashboard };
export type { TimeRange };

export async function updateAccountStatus(
  accountId: string,
  status: "active" | "locked",
  accountNo?: string,
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
      body: JSON.stringify({ status, accountNo }),
    },
  );
  if (!response.ok) throw new Error("Không thể cập nhật trạng thái tài khoản.");
}
