import { API_BASE_URL, readAdminSession } from "@/features/auth/api/adminSession";
import type { TimelineCategory, TimelineResponse } from "../model";

export class AuditApiError extends Error {
  readonly status?: number;

  constructor(message = "Không thể tải lịch sử tài khoản", status?: number) {
    super(message);
    this.name = "AuditApiError";
    this.status = status;
  }
}

function authHeaders(): HeadersInit {
  const session = readAdminSession();
  return session ? { Authorization: `Bearer ${session.accessToken}` } : {};
}

export async function getAccountTimeline(
  account: string,
  options: {
    category: TimelineCategory;
    from?: string;
    to?: string;
    limit?: number;
    cursor?: string;
    signal?: AbortSignal;
  },
): Promise<TimelineResponse> {
  const query = new URLSearchParams({
    category: options.category,
    limit: String(options.limit || 50),
  });
  if (options.from && options.to) {
    query.set("from", options.from);
    query.set("to", options.to);
  }
  if (options.cursor) query.set("cursor", options.cursor);

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/admin/audit/accounts/${encodeURIComponent(account)}/events?${query.toString()}`, {
      headers: authHeaders(),
      signal: options.signal,
    });
  } catch {
    throw new AuditApiError();
  }
  if (!response.ok) {
    const message = response.status === 413
      ? "Lịch sử quá lớn, hãy thu hẹp khoảng thời gian."
      : response.status === 404
        ? "Không tìm thấy tài khoản."
        : "Không thể tải lịch sử tài khoản.";
    throw new AuditApiError(message, response.status);
  }
  try {
    return (await response.json()) as TimelineResponse;
  } catch {
    throw new AuditApiError("Dữ liệu lịch sử không hợp lệ", response.status);
  }
}
