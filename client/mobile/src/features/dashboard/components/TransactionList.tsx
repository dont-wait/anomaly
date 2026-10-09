import { TransactionItem } from "@/features/dashboard/components/TransactionItem";
import type { Transaction } from "@/features/dashboard/model/types";

interface TransactionListProps {
  transactions: Transaction[];
  onViewAll?: () => void;
  loading?: boolean;
  error?: string | null;
  onRetry?: () => void;
}

export const TransactionList = ({
  transactions,
  onViewAll,
  loading = false,
  error,
  onRetry,
}: TransactionListProps) => {
  return (
    <section>
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-bold text-gray-900">Giao dịch gần đây</h2>
        {onViewAll && (
          <button
            onClick={onViewAll}
            className="text-xs font-semibold text-primary"
          >
            Xem tất cả
          </button>
        )}
      </div>

      {loading ? (
        <p className="mt-3 text-sm text-gray-500" role="status">
          Đang tải giao dịch...
        </p>
      ) : error ? (
        <div
          className="mt-3 rounded-2xl bg-red-50 p-4 text-sm text-red-700"
          role="alert"
        >
          <p>{error}</p>
          {onRetry && (
            <button
              type="button"
              className="mt-2 font-semibold underline"
              onClick={onRetry}
            >
              Thử lại
            </button>
          )}
        </div>
      ) : transactions.length === 0 ? (
        <p className="mt-3 text-sm text-gray-500">Chưa có giao dịch nào.</p>
      ) : (
        <div className="mt-1 divide-y divide-gray-100">
          {transactions.map((transaction) => (
            <TransactionItem key={transaction.id} transaction={transaction} />
          ))}
        </div>
      )}
    </section>
  );
};
