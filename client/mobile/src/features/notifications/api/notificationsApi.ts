import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { ApiError, joinUrl, requestApi, toApiError } from "@/shared/lib/http";
import type {
  AppNotification,
  NotificationPage,
  NotificationSeverity,
} from "../model";
import { createSseParser } from "./sse";

interface NotificationDto {
  id: string;
  type: string;
  category: string;
  severity: NotificationSeverity;
  title: string;
  body: string;
  data?: Record<string, string> | null;
  occurredAt: string;
  createdAt: string;
  read: boolean;
}

export function toNotification(dto: NotificationDto): AppNotification {
  return {
    ...dto,
    data: dto.data ?? {},
    occurredAt: new Date(dto.occurredAt),
    createdAt: new Date(dto.createdAt),
  };
}

export async function listNotifications(
  token: string,
  options: {
    cursor?: string | null;
    limit?: number;
    unreadOnly?: boolean;
    signal?: AbortSignal;
  } = {},
): Promise<NotificationPage> {
  const params = new URLSearchParams();
  if (options.cursor) params.set("cursor", options.cursor);
  params.set("limit", String(options.limit ?? 20));
  if (options.unreadOnly) params.set("unread", "true");
  const data = await requestApi<{
    items: NotificationDto[] | null;
    nextCursor: string;
  }>(`${API_ENDPOINTS.NOTIFICATIONS.LIST}?${params}`, {
    token,
    signal: options.signal,
  });
  return {
    items: (data.items ?? []).map(toNotification),
    nextCursor: data.nextCursor || null,
  };
}

export async function getUnreadCount(
  token: string,
  signal?: AbortSignal,
): Promise<number> {
  const data = await requestApi<{ count: number }>(
    API_ENDPOINTS.NOTIFICATIONS.UNREAD_COUNT,
    { token, signal },
  );
  return data.count;
}

export async function markNotificationRead(
  token: string,
  id: string,
): Promise<void> {
  await requestApi(API_ENDPOINTS.NOTIFICATIONS.MARK_READ(id), {
    method: "PATCH",
    token,
  });
}

export async function markAllNotificationsRead(
  token: string,
  before: Date = new Date(),
): Promise<number> {
  const data = await requestApi<{ updated: number }>(
    API_ENDPOINTS.NOTIFICATIONS.READ_ALL,
    { method: "POST", token, body: { before: before.toISOString() } },
  );
  return data.updated;
}

export interface StreamHandlers {
  onOpen?: () => void;
  onNotification: (notification: AppNotification, eventId: string) => void;
  onUnreadCount: (count: number) => void;
}

/**
 * Mở luồng SSE bằng fetch (EventSource không gửi được header Authorization).
 * Promise kết thúc khi server đóng luồng hoặc `signal` bị hủy; lỗi HTTP ném
 * `ApiError` để caller quyết định đăng xuất hay kết nối lại.
 */
export async function openNotificationStream(
  token: string,
  lastEventId: string,
  handlers: StreamHandlers,
  signal: AbortSignal,
): Promise<void> {
  const query = lastEventId ? `?cursor=${encodeURIComponent(lastEventId)}` : "";
  let response: Response;
  try {
    response = await fetch(
      joinUrl(`${API_ENDPOINTS.NOTIFICATIONS.STREAM}${query}`),
      {
        headers: {
          Authorization: `Bearer ${token}`,
          Accept: "text/event-stream",
        },
        signal,
      },
    );
  } catch {
    throw new ApiError(0, "Không thể kết nối luồng thông báo.");
  }
  if (!response.ok) {
    let body: unknown = null;
    try {
      body = JSON.parse(await response.text());
    } catch {
      // body không phải JSON
    }
    throw toApiError(response.status, body);
  }
  if (!response.body)
    throw new ApiError(0, "Luồng thông báo không có dữ liệu.");
  handlers.onOpen?.();

  const parser = createSseParser((message) => {
    try {
      if (message.event === "notification") {
        handlers.onNotification(
          toNotification(JSON.parse(message.data) as NotificationDto),
          message.id,
        );
      } else if (message.event === "unread-count") {
        handlers.onUnreadCount(
          (JSON.parse(message.data) as { count: number }).count,
        );
      }
    } catch {
      // Bỏ qua event hỏng thay vì cắt cả luồng
    }
  });
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  try {
    while (!signal.aborted) {
      const chunk = await reader.read();
      if (chunk.done) break;
      parser.push(decoder.decode(chunk.value, { stream: true }));
    }
  } catch (error) {
    if (signal.aborted) return;
    throw error instanceof ApiError
      ? error
      : new ApiError(0, "Luồng thông báo bị gián đoạn.");
  } finally {
    reader.releaseLock();
  }
}
