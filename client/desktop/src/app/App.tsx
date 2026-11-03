import { useEffect, useState, useSyncExternalStore } from "react";
import { LoginPage } from "@/features/auth/LoginPage";
import {
  verifyAdminSession,
  type AdminProfile,
} from "@/features/auth/api/adminSession";
import { AdminShell } from "@/components/layout/AdminShell";
import { AccountTimelinePage } from "@/features/audit/AccountTimelinePage";
import { EventExplorerPage } from "@/features/audit/EventExplorerPage";
import { LoanReviewPage } from "@/features/loans/LoanReviewPage";
import {
  LoanContractsAdminPage,
  SavingsContractsPage,
} from "@/features/products/ContractsPages";
import { PackagesPage } from "@/features/products/PackagesPage";
import { ProfileChangesPage } from "@/features/profileChanges/ProfileChangesPage";
import { MonitorPage } from "@/features/monitor/MonitorPage";
import {
  navigate,
  protectedRouteFromHash,
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
  const [profile, setProfile] = useState<AdminProfile | null>(null);
  const isMonitor = route === routes.monitor;
  const isAccountTimeline = route === routes.auditAccounts;
  const isEventExplorer = route === routes.audit;
  const isLoanReview = route === routes.loanApplications;
  useEffect(() => {
    let active = true;
    let retryTimer: number | undefined;
    const verify = () => {
      void verifyAdminSession()
        .then((verification) => {
          if (!active) return;
          if (verification.status === "invalid") {
            navigate(routes.login);
            return;
          }
          if (verification.status === "valid") {
            setProfile(verification.profile);
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

  if (!profile) {
    return (
      <main className="grid min-h-screen place-items-center bg-(--admin-background) font-(family-name:--admin-font-mono) text-xs text-(--admin-text-muted)">
        Đang xác thực...
      </main>
    );
  }
  return (
    <AdminShell currentRoute={route} user={profile}>
      {isMonitor ? (
        <MonitorPage />
      ) : isAccountTimeline ? (
        <AccountTimelinePage />
      ) : isEventExplorer ? (
        <EventExplorerPage />
      ) : isLoanReview ? (
        <LoanReviewPage />
      ) : route === routes.loanPackages ? (
        <PackagesPage key="loan" kind="loan" />
      ) : route === routes.savingsPackages ? (
        <PackagesPage key="savings" kind="savings" />
      ) : route === routes.loanContracts ? (
        <LoanContractsAdminPage key="contracts" />
      ) : route === routes.overdueLoans ? (
        <LoanContractsAdminPage key="overdue" overdue />
      ) : route === routes.savingsContracts ? (
        <SavingsContractsPage />
      ) : route === routes.profileChanges ? (
        <ProfileChangesPage />
      ) : (
        <PlaceholderPage route={route} />
      )}
    </AdminShell>
  );
}

export default function App() {
  const hash = useHash();
  const protectedRoute = protectedRouteFromHash(hash);

  if (protectedRoute) return <ProtectedAdmin route={protectedRoute} />;
  return <LoginPage />;
}
