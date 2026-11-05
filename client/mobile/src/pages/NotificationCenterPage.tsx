import { useEffect, useMemo, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCircleCheck,
  faCircleInfo,
  faShieldHalved,
  faTriangleExclamation,
} from "@fortawesome/free-solid-svg-icons";
import { navigate, routes } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import {
  routeForNotification,
  useNotifications,
  type AppNotification,
} from "@/features/notifications";
import { listNotifications } from "@/features/notifications/api/notificationsApi";
import type { NotificationSeverity } from "@/features/notifications/model";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { PageHeader } from "@/shared/layout";
import { ApiError } from "@/shared/lib/http";
import { toast } from "@/shared/notifications/toast";
import { Button } from "@/shared/ui";

const PAGE_SIZE = 20;

const SEVERITY_STYLE: Record<
  NotificationSeverity,
  { icon: typeof faCircleInfo; className: string }
> = {
  INFO: { icon: faCircleInfo, className: "bg-secondary/10 text-secondary" },
  SUCCESS: { icon: faCircleCheck, className: "bg-emerald-50 text-emerald-600" },
  WARNING: {
    icon: faTriangleExclamation,
    className: "bg-amber-50 text-amber-600",
  },
  CRITICAL: { icon: faShieldHalved, className: "bg-error/10 text-error" },
};

const timeFormat = new Intl.DateTimeFormat("vi-VN", {
  dateStyle: "short",
  timeStyle: "short",
});

type Filter = "all" | "unread";

const NotificationCenterPage = () => {
  const { token, logout } = useAuth();
  const { live, markRead, markAllRead, unreadCount } = useNotifications();
  const [filter, setFilter] = useState<Filter>("all");
  const [items, setItems] = useState<AppNotification[]>([]);
  const [readIds, setReadIds] = useState<ReadonlySet<string>>(new Set());
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  const handleError = (requestError: unknown) => {
    if (
      requestError instanceof ApiError &&
      requestError.status === HTTP_STATUS.UNAUTHORIZED
    ) {
      logout();
      return;
    }
    setError(
      requestError instanceof Error
        ? requestError.message
        : "Không thể tải thông báo.",
    );
  };

  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    void Promise.resolve().then(() => {
      if (controller.signal.aborted) return;
      setLoading(true);
      setError(null);
    });
    listNotifications(token, {
      limit: PAGE_SIZE,
      unreadOnly: filter === "unread",
      signal: controller.signal,
    })
      .then((page) => {
        setItems(page.items);
        setNextCursor(page.nextCursor);
      })
      .catch((requestError: unknown) => {
        if (!controller.signal.aborted) handleError(requestError);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
    // handleError only depends on logout, which is stable.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, filter, reloadKey]);

  const loadMore = () => {
    if (!token || !nextCursor || loadingMore) return;
    setLoadingMore(true);
    listNotifications(token, {
      cursor: nextCursor,
      limit: PAGE_SIZE,
      unreadOnly: filter === "unread",
    })
      .then((page) => {
        setItems((current) => {
          const known = new Set(current.map((item) => item.id));
          return [
            ...current,
            ...page.items.filter((item) => !known.has(item.id)),
          ];
        });
        setNextCursor(page.nextCursor);
      })
      .catch(handleError)
      .finally(() => setLoadingMore(false));
  };

  // Realtime arrivals go on top; local read state overrides server snapshots.
  const visible = useMemo(() => {
    const known = new Set(items.map((item) => item.id));
    const merged = [...live.filter((item) => !known.has(item.id)), ...items]
      .map((item) => (readIds.has(item.id) ? { ...item, read: true } : item))
      .sort((a, b) => b.createdAt.getTime() - a.createdAt.getTime());
    return filter === "unread" ? merged.filter((item) => !item.read) : merged;
  }, [items, live, readIds, filter]);

  const open = async (notification: AppNotification) => {
    try {
      await markRead(notification);
      setReadIds((current) => new Set(current).add(notification.id));
    } catch {
      // Vẫn cho mở chi tiết; trạng thái đọc sẽ đồng bộ lần sau
    }
    navigate(routeForNotification(notification));
  };

  const readAll = async () => {
    try {
      await markAllRead();
      setReadIds(new Set(visible.map((item) => item.id)));
    } catch (requestError) {
      if (
        requestError instanceof ApiError &&
        requestError.status === HTTP_STATUS.UNAUTHORIZED
      ) {
        logout();
        return;
      }
      toast.error("Không thể đánh dấu đã đọc. Vui lòng thử lại.");
    }
  };

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-surface">
      <PageHeader title="Thông báo" onBack={() => navigate(routes.dashboard)} />

      <div className="flex items-center justify-between gap-2 px-4 pt-3 pb-2">
        <div
          className="flex gap-1 rounded-full bg-surface-container p-1"
          role="tablist"
          aria-label="Lọc thông báo"
        >
          {(
            [
              ["all", "Tất cả"],
              ["unread", "Chưa đọc"],
            ] as const
          ).map(([value, label]) => (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={filter === value}
              onClick={() => setFilter(value)}
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition-colors ${
                filter === value
                  ? "bg-surface-container-lowest text-on-surface shadow-sm"
                  : "text-on-surface-variant"
              }`}
            >
              {label}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={() => void readAll()}
          disabled={unreadCount === 0 && visible.every((item) => item.read)}
          className="text-sm font-medium text-secondary-strong disabled:opacity-40"
        >
          Đánh dấu tất cả đã đọc
        </button>
      </div>

      <main className="flex-1 overflow-y-auto px-4 pb-8">
        {loading && visible.length === 0 ? (
          <p
            className="py-10 text-center text-sm text-on-surface-variant"
            role="status"
          >
            Đang tải thông báo...
          </p>
        ) : error && visible.length === 0 ? (
          <div className="space-y-3 py-10 text-center">
            <p className="text-sm text-error">{error}</p>
            <Button
              variant="secondary"
              onClick={() => setReloadKey((key) => key + 1)}
            >
              Thử lại
            </Button>
          </div>
        ) : visible.length === 0 ? (
          <p className="py-10 text-center text-sm text-on-surface-variant">
            {filter === "unread"
              ? "Bạn đã đọc hết thông báo."
              : "Chưa có thông báo nào."}
          </p>
        ) : (
          <ul className="space-y-2" aria-label="Danh sách thông báo">
            {visible.map((item) => {
              const style =
                SEVERITY_STYLE[item.severity] ?? SEVERITY_STYLE.INFO;
              return (
                <li key={item.id}>
                  <button
                    type="button"
                    onClick={() => void open(item)}
                    className={`flex w-full items-start gap-3 rounded-2xl p-3 text-left transition-colors ${
                      item.read
                        ? "bg-surface-container-lowest"
                        : "bg-secondary/5 ring-1 ring-secondary/15"
                    }`}
                  >
                    <span
                      className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${style.className}`}
                      aria-hidden="true"
                    >
                      <FontAwesomeIcon icon={style.icon} />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-2">
                        <span
                          className={`truncate text-sm text-on-surface ${item.read ? "font-medium" : "font-semibold"}`}
                        >
                          {item.title}
                        </span>
                        {!item.read && (
                          <span
                            className="h-2 w-2 shrink-0 rounded-full bg-secondary"
                            aria-label="Chưa đọc"
                          />
                        )}
                      </span>
                      <span className="mt-0.5 block text-sm break-words text-on-surface-variant">
                        {item.body}
                      </span>
                      <span className="mt-1 block text-xs text-on-surface-variant/80">
                        {timeFormat.format(item.createdAt)}
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
        {nextCursor && (
          <div className="pt-4">
            <Button
              variant="ghost"
              className="w-full"
              onClick={loadMore}
              disabled={loadingMore}
            >
              {loadingMore ? "Đang tải..." : "Xem thêm"}
            </Button>
          </div>
        )}
      </main>
    </div>
  );
};

export default NotificationCenterPage;
