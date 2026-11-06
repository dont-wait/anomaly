import { useEffect, useState, type ReactNode } from "react";
import { clearAdminSession } from "@/features/auth/api/adminSession";
import { navigate, routes } from "@/app/routes";
import {
  consumeUnhandledAlertCountStream,
  getUnhandledAlertCount,
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
    let delay = RETRY_MIN_MS;

    const refresh = async () => {
      try {
        const nextCount = await getUnhandledAlertCount();
        if (cancelled) return;
        setCount(normalizeCount(nextCount));
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
        await consumeUnhandledAlertCountStream(
          (nextCount) => setCount(normalizeCount(nextCount)),
          controller.signal,
          () => {
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
