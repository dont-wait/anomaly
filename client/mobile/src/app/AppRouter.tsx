import { AUTH_STATUS } from "@/features/auth/authStatus";
import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { useAuth } from "@/features/auth/useAuth";
import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import DashboardPage from "@/pages/DashboardPage";
import TransferPage from "@/pages/TransferPage";
import TransactionHistoryPage from "@/pages/TransactionHistoryPage";
import TransactionDetailPage from "@/pages/TransactionDetailPage";
import LoanRegisterPage from "@/pages/LoanRegisterPage";
import SavingsLivePage from "@/features/savings/SavingsLivePage";
import LoanApplicationsPage from "@/pages/LoanApplicationsPage";
import LoanContractsPage from "@/pages/LoanContractsPage";
import type { LoanApplication } from "@/features/loans/api";
import {
  isProtectedRoute,
  parseTransactionId,
  parseLoanApplicationId,
  parseLoanContractId,
  routes,
} from "./routes";

function subscribe(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}

// The list stays mounted while a detail overlay is open so its pagination and
// scroll survive; changes seen in the detail are applied to the list on top.
function LoanApplicationsRoute({ applicationId }: { applicationId?: string }) {
  const [updates, setUpdates] = useState<Record<string, LoanApplication>>({});
  const onApplicationChange = useCallback((application: LoanApplication) => {
    setUpdates((current) => ({ ...current, [application.id]: application }));
  }, []);

  return (
    <>
      <div inert={Boolean(applicationId)}>
        <LoanApplicationsPage updatedApplications={updates} />
      </div>
      {applicationId && (
        <LoanApplicationsPage
          key={applicationId}
          applicationId={applicationId}
          onApplicationChange={onApplicationChange}
        />
      )}
    </>
  );
}

// Hash routes work in both Vite and the packaged Tauri WebView.
export function AppRouter() {
  const { status } = useAuth();
  const hash = useSyncExternalStore(
    subscribe,
    () => window.location.hash,
    () => routes.login,
  );
  const isProtected = isProtectedRoute(hash);
  // A fresh launch has no fragment and renders the login page.
  const isLoginRoute = hash === routes.login || hash === "";

  useEffect(() => {
    if (isProtected && status === AUTH_STATUS.UNAUTHENTICATED) {
      window.location.hash = routes.login;
    }
  }, [isProtected, status]);

  useEffect(() => {
    if (isLoginRoute && status === AUTH_STATUS.AUTHENTICATED) {
      window.location.hash = routes.dashboard;
    }
  }, [isLoginRoute, status]);

  if (
    isLoginRoute &&
    (status === AUTH_STATUS.IDLE || status === AUTH_STATUS.RESTORING)
  ) {
    return (
      <div className="p-6 text-sm text-gray-500" role="status">
        Đang khôi phục phiên đăng nhập...
      </div>
    );
  }

  if (isProtected && status !== AUTH_STATUS.AUTHENTICATED) {
    return (
      <div className="p-6 text-sm text-gray-500">
        Đang kiểm tra phiên đăng nhập...
      </div>
    );
  }

  const transactionId = parseTransactionId(hash);
  if (transactionId) {
    return <TransactionDetailPage transactionId={transactionId} />;
  }

  const applicationId = parseLoanApplicationId(hash);
  if (applicationId)
    return <LoanApplicationsRoute applicationId={applicationId} />;
  const loanId = parseLoanContractId(hash);
  if (loanId) return <LoanContractsPage loanId={loanId} />;

  switch (hash) {
    case routes.dashboard:
      return <DashboardPage />;
    case routes.transfer:
      return <TransferPage />;
    case routes.transactions:
      return <TransactionHistoryPage />;
    case routes.loanRegister:
      return <LoanRegisterPage />;
    case routes.savings:
      return <SavingsLivePage />;
    case routes.loanApplications:
      return <LoanApplicationsRoute />;
    case routes.loanContracts:
      return <LoanContractsPage />;
    case routes.register:
      return <RegisterPage />;
    default:
      return <LoginPage />;
  }
}
