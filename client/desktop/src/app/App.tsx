import { useEffect, useState, useSyncExternalStore } from "react";
import { LoginPage } from "@/features/auth/LoginPage";
import { verifyAdminSession } from "@/features/auth/api/adminSession";
import { AdminShell } from "@/components/layout/AdminShell";
import {
  isProtectedRoute,
  navigate,
  routes,
  type ProtectedRoute,
} from "./routes";

function subscribe(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}

function useHash() {
  return useSyncExternalStore(
    subscribe,
    () => window.location.hash || routes.login,
    () => routes.login,
  );
}

function PlaceholderPage({ route }: { route: ProtectedRoute }) {
  const isMonitor = route === routes.monitor;

  return (
    <section
      className="grid min-h-90 place-items-center rounded-(--admin-radius-lg) border border-(--admin-border) bg-(--admin-surface)"
      aria-label="Nội dung trang"
    >
      <div className="grid max-w-90 gap-2 p-6 text-center">
        <strong>
          {isMonitor
            ? "Monitoring workspace sẵn sàng"
            : "Màn hình đang được triển khai"}
        </strong>
        <span className="font-(family-name:--admin-font-mono) text-[10px] leading-4 text-(--admin-text-muted)">
          {isMonitor
            ? "Admin shell đã được kết nối. Dashboard risk sẽ được đưa vào phase tiếp theo."
            : "Route đã được đăng ký và sẽ dùng chung layout admin này."}
        </span>
      </div>
    </section>
  );
}

function ProtectedAdmin({ route }: { route: ProtectedRoute }) {
  const [isAuthorized, setIsAuthorized] = useState(false);
  useEffect(() => {
    let active = true;
    let retryTimer: number | undefined;
    const verify = () => {
      void verifyAdminSession()
        .then((verification) => {
          if (!active) return;
          if (verification === "invalid") {
            navigate(routes.login);
            return;
          }
          if (verification === "valid") {
            setIsAuthorized(true);
            return;
          }
          retryTimer = window.setTimeout(verify, 3000);
        })
        .catch(() => {
          if (active) retryTimer = window.setTimeout(verify, 3000);
        });
    };
    verify();
    return () => {
      active = false;
      if (retryTimer !== undefined) window.clearTimeout(retryTimer);
    };
  }, []);

  if (!isAuthorized) {
    return (
      <main className="grid min-h-screen place-items-center bg-(--admin-background) font-(family-name:--admin-font-mono) text-xs text-(--admin-text-muted)">
        Đang xác thực...
      </main>
    );
  }
  return (
    <AdminShell currentRoute={route}>
      <PlaceholderPage route={route} />
    </AdminShell>
  );
}

export default function App() {
  const hash = useHash();

  if (isProtectedRoute(hash)) return <ProtectedAdmin route={hash} />;
  return <LoginPage />;
}
