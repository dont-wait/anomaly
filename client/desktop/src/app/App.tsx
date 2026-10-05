import { useEffect, useState, useSyncExternalStore } from "react";
import { LoginPage } from "@/features/auth/LoginPage";
import { verifyAdminSession } from "@/features/auth/api/adminSession";
import { navigate, routes } from "./routes";

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

function ComingSoonPage() {
  return (
    <main className="admin-login-page">
      <section className="admin-login-card" aria-labelledby="coming-soon-title">
        <div className="admin-brand" aria-label="AnomalyBank Admin Console">
          <span className="admin-brand-mark" aria-hidden="true">
            <span aria-hidden="true">A</span>
          </span>
          <span className="admin-brand-name">AnomalyBank Admin</span>
        </div>
        <h1 className="admin-login-heading" id="coming-soon-title">
          Đăng nhập thành công
        </h1>
        <p className="admin-login-subtitle">
          Dashboard quản trị sẽ được triển khai ở phase tiếp theo.
        </p>
      </section>
    </main>
  );
}

function ProtectedMonitor() {
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
    return <main className="admin-login-page">Đang xác thực...</main>;
  }
  return <ComingSoonPage />;
}

export default function App() {
  const hash = useHash();

  if (hash === routes.monitor) return <ProtectedMonitor />;
  return <LoginPage />;
}
