import { navigate, routes, transactionDetailRoute } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import { TransactionHistory, transactionStore } from "@/features/transactions";
import { PageHeader } from "@/shared/layout";

const TransactionHistoryPage = () => {
  const { user } = useAuth();

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-gradient-to-b from-secondary-container/40 to-surface-container-lowest">
      <PageHeader
        title="Lịch sử giao dịch"
        onBack={() => navigate(routes.dashboard)}
        backLabel="Về trang chủ"
      />
      <main className="flex-1 overflow-y-auto px-4 py-5">
        <TransactionHistory
          records={user ? transactionStore.list(user.accountNo) : []}
          onSelect={(record) => navigate(transactionDetailRoute(record.id))}
        />
      </main>
    </div>
  );
};

export default TransactionHistoryPage;
