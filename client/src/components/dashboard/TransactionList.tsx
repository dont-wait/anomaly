import { TransactionItem } from "@/components/dashboard/TransactionItem";
import type { Transaction } from "@/types";

interface TransactionListProps {
  transactions: Transaction[];
  onViewAll?: () => void;
}

export const TransactionList = ({
  transactions,
  onViewAll,
}: TransactionListProps) => {
  return (
    <section>
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-bold text-gray-900">Giao dịch gần đây</h2>
        {onViewAll && (
          <button
            onClick={onViewAll}
            className="text-xs font-semibold text-brand-primary"
          >
            Xem tất cả
          </button>
        )}
      </div>

      <div className="mt-1 divide-y divide-gray-100">
        {transactions.map((transaction) => (
          <TransactionItem key={transaction.id} transaction={transaction} />
        ))}
      </div>
    </section>
  );
};
