import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { navigate } from "@/app/routes";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import { useAuth } from "@/features/auth/useAuth";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { ApiError } from "@/shared/lib/http";
import { toast, type ToastTone } from "@/shared/notifications/toast";
import {
  getUnreadCount,
  markAllNotificationsRead,
  markNotificationRead,
  openNotificationStream,
} from "./api/notificationsApi";
import type { AppNotification, NotificationSeverity } from "./model";
import {
  NotificationContext,
  type NotificationContextValue,
} from "./notificationContext";
import { routeForNotification } from "./notificationRoutes";

const STREAM_RETRY_MIN_MS = 1000;
const STREAM_RETRY_MAX_MS = 30000;
const MAX_LIVE_ITEMS = 50;

const SEVERITY_TONE: Record<NotificationSeverity, ToastTone> = {
  INFO: "info",
  SUCCESS: "success",
  WARNING: "warning",
  CRITICAL: "error",
};

function announce(notification: AppNotification) {
  toast.show(
    notification.body,
    SEVERITY_TONE[notification.severity] ?? "info",
    {
      title: notification.title,
      onClick: () => navigate(routeForNotification(notification)),
    },
  );
}

/**
 * Giữ một luồng SSE trong suốt phiên đăng nhập: badge, danh sách mới nhận
 * và banner trong app. Đóng luồng khi đăng xuất; 401 thì đăng xuất.
 */
export function NotificationProvider({ children }: { children: ReactNode }) {
  const { status, token, logout } = useAuth();
  const [unreadCount, setUnreadCount] = useState(0);
  const [live, setLive] = useState<AppNotification[]>([]);
  const logoutRef = useRef(logout);
  useEffect(() => {
    logoutRef.current = logout;
  }, [logout]);

  const activeToken =
    status === AUTH_STATUS.AUTHENTICATED && token ? token : null;

  useEffect(() => {
    if (!activeToken) return;
    const controller = new AbortController();
    let cancelled = false;
    let delay = STREAM_RETRY_MIN_MS;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    // Chỉ giữ trong bộ nhớ: đủ để nối lại không mất thông báo trong phiên.
    let lastEventId = "";
    const seen = new Set<string>();

    const run = async () => {
      try {
        await openNotificationStream(
          activeToken,
          lastEventId,
          {
            onOpen: () => {
              delay = STREAM_RETRY_MIN_MS;
            },
            onNotification: (notification, eventId) => {
              if (eventId) lastEventId = eventId;
              // Replay after a reconnect can resend what we already showed.
              if (seen.has(notification.id)) return;
              seen.add(notification.id);
              setLive((current) =>
                [notification, ...current].slice(0, MAX_LIVE_ITEMS),
              );
              announce(notification);
            },
            onUnreadCount: setUnreadCount,
            onCursor: (eventId) => {
              lastEventId = eventId;
            },
          },
          controller.signal,
        );
      } catch (error) {
        if (cancelled) return;
        if (
          error instanceof ApiError &&
          error.status === HTTP_STATUS.UNAUTHORIZED
        ) {
          logoutRef.current();
          return;
        }
      }
      if (cancelled) return;
      retryTimer = setTimeout(() => void run(), delay);
      delay = Math.min(delay * 2, STREAM_RETRY_MAX_MS);
    };
    void run();

    return () => {
      cancelled = true;
      clearTimeout(retryTimer);
      controller.abort();
      // Logout or token change: never show the previous session's state.
      setUnreadCount(0);
      setLive([]);
    };
  }, [activeToken]);

  const refreshCount = useCallback(async () => {
    if (!activeToken) return;
    try {
      setUnreadCount(await getUnreadCount(activeToken));
    } catch {
      // Badge sẽ được cập nhật lại ở event unread-count tiếp theo
    }
  }, [activeToken]);

  const markRead = useCallback(
    async (notification: AppNotification) => {
      if (!activeToken || notification.read) return;
      await markNotificationRead(activeToken, notification.id);
      setLive((current) =>
        current.map((item) =>
          item.id === notification.id ? { ...item, read: true } : item,
        ),
      );
      setUnreadCount((count) => Math.max(0, count - 1));
      void refreshCount();
    },
    [activeToken, refreshCount],
  );

  const markAllRead = useCallback(async () => {
    if (!activeToken) return;
    await markAllNotificationsRead(activeToken);
    setLive((current) => current.map((item) => ({ ...item, read: true })));
    setUnreadCount(0);
    void refreshCount();
  }, [activeToken, refreshCount]);

  const value = useMemo<NotificationContextValue>(
    () => ({ unreadCount, live, markRead, markAllRead }),
    [unreadCount, live, markRead, markAllRead],
  );

  return (
    <NotificationContext.Provider value={value}>
      {children}
    </NotificationContext.Provider>
  );
}
