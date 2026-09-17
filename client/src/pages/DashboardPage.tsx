import { toast } from "@/shared/notifications/toast";
import { toLoginError } from "@/features/auth/api/auth";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import { AppHeader } from "@/shared/layout/AppHeader";
import { BottomNav } from "@/shared/layout/BottomNav";
import { BalanceCard } from "@/features/dashboard/components/BalanceCard";
import { QuickActions } from "@/features/dashboard/components/QuickActions";
import { PromoBanner } from "@/features/dashboard/components/PromoBanner";
import { TransactionList } from "@/features/dashboard/components/TransactionList";
import {
  mockPromoBanner,
  mockQuickActions,
  mockTransactions,
} from "@/features/dashboard/mocks/dashboard";
import { Avatar } from "@/shared/ui";
import { useAuth } from "@/features/auth/useAuth";

const DashboardPage = () => {
  const { status, user, refreshProfile } = useAuth();
  const quickActions = mockQuickActions;
  const promo = mockPromoBanner;
  const transactions = mockTransactions;

  if (status === AUTH_STATUS.RESTORING || status === AUTH_STATUS.IDLE) {
    return (
      <div className="p-6 text-sm text-gray-500">
        Đang tải thông tin tài khoản...
      </div>
    );
  }

  if (status === AUTH_STATUS.AUTHENTICATED && !user) {
    return (
      <div className="p-6 text-sm text-red-600">
        Thông tin tài khoản chưa sẵn sàng.
        <button
          type="button"
          className="mt-3 block rounded-lg bg-primary px-3 py-2 text-white"
          onClick={() =>
            void refreshProfile().catch((error) =>
              toast.error(toLoginError(error)),
            )
          }
        >
          Thử lại
        </button>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="p-6 text-sm text-gray-500">
        Đang kiểm tra phiên đăng nhập...
      </div>
    );
  }

  const account = {
    id: user.id,
    ownerName: user.fullName || user.username,
    accountNumber: user.accountNo,
    balance: user.amount,
    currency: user.currency === "VND" ? "₫" : user.currency,
    cardLabel: "ANOMALYBANK SIGNATURE",
  };

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-gradient-to-b from-violet-100 to-white">
      <AppHeader notificationCount={3} />

      <main className="flex-1 space-y-6 overflow-y-auto px-4 py-5">
        <div className="flex items-center gap-2">
          <Avatar name={account.ownerName} size={40} />
          <p className="text-sm text-gray-500">
            Xin chào,{" "}
            <span className="font-semibold text-gray-900">
              {account.ownerName}
            </span>
          </p>
        </div>

        <BalanceCard account={account} />
        <QuickActions actions={quickActions} />
        <PromoBanner promo={promo} />
        <TransactionList transactions={transactions} />
      </main>

      <BottomNav onQrScan={() => console.log("TODO: mở màn hình quét QR")} />
    </div>
  );
};

export default DashboardPage;
