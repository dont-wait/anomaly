import { AppHeader } from "@/shared/layout/AppHeader";
import { BottomNav } from "@/shared/layout/BottomNav";
import { BalanceCard } from "@/features/dashboard/components/BalanceCard";
import { QuickActions } from "@/features/dashboard/components/QuickActions";
import { PromoBanner } from "@/features/dashboard/components/PromoBanner";
import { TransactionList } from "@/features/dashboard/components/TransactionList";
import {
  mockAccount,
  mockPromoBanner,
  mockQuickActions,
  mockTransactions,
} from "@/features/dashboard/mocks/dashboard";
import { Avatar } from "@/shared/ui";

/**
 * Trang chủ (Dashboard) — dữ liệu đang lấy từ mock (src/features/dashboard/mocks/dashboard.ts).
 * Khi có API/backend thật, chỉ cần thay các biến `mock*` bằng dữ liệu
 * lấy từ hook/query thật, phần JSX bên dưới không cần đổi vì các
 * component con nhận props đúng theo type trong `src/types`.
 */
const DashboardPage = () => {
  const account = mockAccount;
  const quickActions = mockQuickActions;
  const promo = mockPromoBanner;
  const transactions = mockTransactions;

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
