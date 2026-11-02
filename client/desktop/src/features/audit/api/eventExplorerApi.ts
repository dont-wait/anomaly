import { API_BASE_URL, readAdminSession } from "@/features/auth/api/adminSession";
import type { EventExplorerCategory, EventExplorerResponse } from "../model";

export class EventExplorerApiError extends Error {
  readonly status?: number;

  constructor(message = "Không thể tải event", status?: number) {
    super(message);
    this.name = "EventExplorerApiError";
    this.status = status;
  }
}

export interface EventExplorerQuery {
  searchType: "transactionId" | "account";
  search: string;
  category: EventExplorerCategory;
  eventType: string;
  status: string;
  from: string;
  to: string;
  limit?: number;
  cursor?: string;
  signal?: AbortSignal;
}

function authHeaders(): HeadersInit {
  const session = readAdminSession();
  return session ? { Authorization: `Bearer ${session.accessToken}` } : {};
}

export async function getEventExplorer(options: EventExplorerQuery): Promise<EventExplorerResponse> {
  const query = new URLSearchParams({
    category: options.category,
    limit: String(options.limit || 20),
  });
  if (options.search.trim()) query.set(options.searchType, options.search.trim());
  if (options.eventType.trim()) query.set("eventType", options.eventType.trim());
  if (options.status.trim()) query.set("status", options.status.trim());
  if (options.from) query.set("from", options.from);
  if (options.to) query.set("to", options.to);
  if (options.cursor) query.set("cursor", options.cursor);

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/admin/audit/events?${query.toString()}`, {
      headers: authHeaders(),
      signal: options.signal,
    });
  } catch {
    throw new EventExplorerApiError();
  }
  if (!response.ok) {
    const message = response.status === 413
      ? "Phạm vi tìm kiếm quá rộng, hãy thêm bộ lọc."
      : response.status === 401 || response.status === 403
        ? "Phiên đăng nhập không có quyền tra cứu event."
        : "Không thể tải event.";
    throw new EventExplorerApiError(message, response.status);
  }
  try {
    return (await response.json()) as EventExplorerResponse;
  } catch {
    throw new EventExplorerApiError("Dữ liệu event không hợp lệ", response.status);
  }
}

export async function getEventExplorerDetail(item: { eventId: string; sourceStream: string; sourceRevision: number }): Promise<EventExplorerResponse["items"][number]> {
  const query = new URLSearchParams({
    stream: item.sourceStream,
    revision: String(item.sourceRevision),
  });
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/admin/audit/events/${encodeURIComponent(item.eventId)}?${query.toString()}`, {
      headers: authHeaders(),
    });
  } catch {
    throw new EventExplorerApiError();
  }
  if (!response.ok) {
    throw new EventExplorerApiError(response.status === 404 ? "Không tìm thấy event." : "Không thể tải chi tiết event.", response.status);
  }
  try {
    return (await response.json()) as EventExplorerResponse["items"][number];
  } catch {
    throw new EventExplorerApiError("Dữ liệu chi tiết event không hợp lệ", response.status);
  }
}
