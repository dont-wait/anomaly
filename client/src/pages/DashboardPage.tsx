import { AppHeader } from "@/shared/layout/AppHeader";
import { BottomNav } from "@/shared/layout/BottomNav";
import { BalanceCard } from "@/features/dashboard/components/BalanceCard";
import { QuickActions } from "@/features/dashboard/components/QuickActions";
import { PromoBanner } from "@/features/dashboard/components/PromoBanner";
import { TransactionList } from "@/features/dashboard/components/TransactionList";
import { useMemo } from "react";
import { useAuth } from "@/features/auth/useAuth";
import {
  mockPromoBanner,
  mockQuickActions,
} from "@/features/dashboard/mocks/dashboard";
import { Avatar } from "@/shared/ui";

/**
 * Trang chủ (Dashboard). Account data comes from the authenticated profile;
 * static quick actions and promo content remain local presentation data.
 */
const DashboardPage = () => {
  const { status, user } = useAuth();
  const account = useMemo(
    () =>
      user
        ? {
            id: user.id,
            ownerName: user.username,
            accountNumber: user.accountNo,
            balance: user.amount,
            currency: user.currency === "VND" ? "₫" : user.currency,
          }
        : null,
    [user],
  );
  const quickActions = mockQuickActions;
  const promo = mockPromoBanner;

  if (
    status === "idle" ||
    status === "restoring" ||
    (status === "authenticated" && !account)
  ) {
    return <div className="p-6 text-sm text-gray-500">Đang tải tài khoản...</div>;
  }

  if (!account) {
    return <div className="p-6 text-sm text-gray-500">Không tải được tài khoản.</div>;
  }

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

        <TransactionList transactions={[]} />
      </main>

      <BottomNav onQrScan={() => console.log("TODO: mở màn hình quét QR")} />
    </div>
  );
};

export default DashboardPage;