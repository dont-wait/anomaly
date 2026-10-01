import { AUTH_STATUS } from "@/features/auth/authStatus";
import { useEffect, useSyncExternalStore } from "react";
import { useAuth } from "@/features/auth/useAuth";
import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import DashboardPage from "@/pages/DashboardPage";
import TransferPage from "@/pages/TransferPage";
import TransactionHistoryPage from "@/pages/TransactionHistoryPage";
import TransactionDetailPage from "@/pages/TransactionDetailPage";
import { isProtectedRoute, parseTransactionId, routes } from "./routes";

function subscribe(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
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

  useEffect(() => {
    if (isProtected && status === AUTH_STATUS.UNAUTHENTICATED) {
      window.location.hash = routes.login;
    }
  }, [isProtected, status]);

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

  switch (hash) {
    case routes.dashboard:
      return <DashboardPage />;
    case routes.transfer:
      return <TransferPage />;
    case routes.transactions:
      return <TransactionHistoryPage />;
    case routes.register:
      return <RegisterPage />;
    default:
      return <LoginPage />;
  }
}
