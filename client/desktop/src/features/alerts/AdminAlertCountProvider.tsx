import { useEffect, useState, type ReactNode } from "react";
import { clearAdminSession } from "@/features/auth/api/adminSession";
import { navigate, routes } from "@/app/routes";
import {
  consumeRiskStream,
  getMonitorDashboard,
  shouldAdvanceMonitorCursor,
} from "@/features/monitor/api/monitorApi";
import { AdminAlertCountContext } from "./adminAlertCountContext";

const RETRY_MIN_MS = 1000;
const RETRY_MAX_MS = 15000;

function normalizeCount(value: number) {
  return Math.max(0, Math.floor(value));
}

export function AdminAlertCountProvider({ children }: { children: ReactNode }) {
  const [count, setCount] = useState(0);

  useEffect(() => {
    let cancelled = false;
    let retryTimer: number | undefined;
    let controller: AbortController | undefined;
    let cursor = "";
    let delay = RETRY_MIN_MS;
    let connected = false;

    const refresh = async () => {
      try {
        const response = await getMonitorDashboard("24h", undefined, "all");
        if (cancelled) return;
        setCount(normalizeCount(response.snapshot.unhandledAlertCount));
        if (shouldAdvanceMonitorCursor(cursor, response.cursor)) {
          cursor = response.cursor;
        }
      } catch (error) {
        if (
          error instanceof Error &&
          "status" in error &&
          [401, 403].includes((error as { status?: number }).status ?? 0)
        ) {
          clearAdminSession();
          navigate(routes.login);
        }
      }
    };

    const run = async () => {
      if (cancelled) return;
      controller = new AbortController();
      try {
        await consumeRiskStream(
          cursor,
          (event, nextCursor) => {
            if (nextCursor) cursor = nextCursor;
            if (event.kind === "alert_count") {
              setCount(normalizeCount(event.count));
            }
          },
          controller.signal,
          () => {
            if (connected) void refresh();
            connected = true;
            delay = RETRY_MIN_MS;
          },
        );
      } catch (error) {
        if (cancelled) return;
        if (
          error instanceof Error &&
          "status" in error &&
          [401, 403].includes((error as { status?: number }).status ?? 0)
        ) {
          clearAdminSession();
          navigate(routes.login);
          return;
        }
      }
      if (cancelled) return;
      retryTimer = window.setTimeout(() => void run(), delay);
      delay = Math.min(delay * 2, RETRY_MAX_MS);
    };

    void refresh().finally(() => {
      if (!cancelled) void run();
    });

    return () => {
      cancelled = true;
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
      controller?.abort();
    };
  }, []);

  return (
    <AdminAlertCountContext.Provider value={count}>
      {children}
    </AdminAlertCountContext.Provider>
  );
}
