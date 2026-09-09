import { AppHeader } from "@/components/layout/AppHeader";
import { BottomNav } from "@/components/layout/BottomNav";
import { BalanceCard } from "@/components/dashboard/BalanceCard";
import { QuickActions } from "@/components/dashboard/QuickActions";
import { PromoBanner } from "@/components/dashboard/PromoBanner";
import { TransactionList } from "@/components/dashboard/TransactionList";
import {
    mockAccount,
    mockPromoBanner,
    mockQuickActions,
    mockTransactions,
} from "@/mocks/dashboard";
import { Avatar } from "@/components/ui";

/**
 * Trang chủ (Dashboard) — dữ liệu đang lấy từ mock (src/mocks/dashboard.ts).
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

            <BottomNav />
        </div>
    );
};

export default DashboardPage;