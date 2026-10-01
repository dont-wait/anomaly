import { toast } from "@/shared/notifications/toast";
import { toLoginError } from "@/features/auth/api/auth";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import { AppHeader } from "@/shared/layout/AppHeader";
import { BottomNav } from "@/shared/layout/BottomNav";
import { BalanceCard } from "@/features/dashboard/components/BalanceCard";
import { QuickActions } from "@/features/dashboard/components/QuickActions";
import { PromoBanner } from "@/features/dashboard/components/PromoBanner";
import { TransactionList } from "@/features/dashboard/components/TransactionList";
import type { Transaction } from "@/features/dashboard/model/types";
import {
  mockPromoBanner,
  mockQuickActions,
} from "@/features/dashboard/mocks/dashboard";
import { transactionStore } from "@/features/transactions";
import type { TransactionRecord } from "@/features/transactions/model";
import { Avatar } from "@/shared/ui";
import { useAuth } from "@/features/auth/useAuth";
import { navigate, routes } from "@/app/routes";

const dashboardDateFormat = new Intl.DateTimeFormat("vi-VN", {
  day: "2-digit",
  month: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
});

/** Map record dùng chung với history sang shape gọn của dashboard. */
const toDashboardTransaction = (record: TransactionRecord): Transaction => {
  const category =
    record.kind === "transfer"
      ? record.direction === "in"
        ? "transfer-in"
        : "transfer-out"
      : record.kind === "bill"
        ? "bill"
        : record.kind === "savings" || record.kind === "salary"
          ? "savings"
          : /mart|winmart|siêu thị|shopping/i.test(record.counterparty.name)
            ? "shopping"
            : record.kind === "payment"
              ? "food"
              : "other";
  const subtitle = `${dashboardDateFormat.format(record.createdAt)} · ${record.note || record.reference}`;
  return {
    id: record.id,
    amount: record.amount,
    type: record.direction === "in" ? "credit" : "debit",
    description: record.counterparty.name,
    date: record.createdAt,
    category,
    subtitle,
  };
};

const DashboardPage = () => {
  const { status, user, refreshProfile } = useAuth();
  const quickActions = mockQuickActions;
  const promo = mockPromoBanner;

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

  // Cùng nguồn `transactionStore` với trang lịch sử để chuyển tiền xong hiện ngay.
  const transactions = transactionStore
    .list(user.accountNo)
    .slice(0, 4)
    .map(toDashboardTransaction);

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
        <QuickActions
          actions={quickActions}
          onSelect={(action) => {
            if (action.icon === "transfer") navigate(routes.transfer);
          }}
        />
        <PromoBanner promo={promo} />
        <TransactionList
          transactions={transactions}
          onViewAll={() => navigate(routes.transactions)}
        />
      </main>

      <BottomNav onQrScan={() => console.log("TODO: mở màn hình quét QR")} />
    </div>
  );
};

export default DashboardPage;
