import {
  ArrowDownLeftIcon,
  BillIcon,
  CoffeeIcon,
  SavingsIcon,
  ShoppingCartIcon,
  TransferIcon,
  TrendUpIcon,
  TrendDownIcon,
} from "@/shared/icons";
import type {
  Transaction,
  TransactionCategory,
} from "@/features/dashboard/model/types";

type IconComponent = (props: { className?: string }) => React.JSX.Element;

const CATEGORY_ICON: Record<TransactionCategory, IconComponent> = {
  food: CoffeeIcon,
  shopping: ShoppingCartIcon,
  "transfer-in": ArrowDownLeftIcon,
  "transfer-out": TransferIcon,
  bill: BillIcon,
  savings: SavingsIcon,
  other: BillIcon,
};

const CATEGORY_COLOR: Record<TransactionCategory, string> = {
  food: "bg-secondary/15 text-secondary",
  shopping: "bg-secondary/15 text-secondary",
  "transfer-in": "bg-secondary/15 text-secondary",
  "transfer-out": "bg-secondary/15 text-secondary",
  bill: "bg-secondary/15 text-secondary",
  savings: "bg-secondary/15 text-secondary",
  other: "bg-gray-100 text-gray-600",
};

const formatSignedCurrency = (amount: number, type: Transaction["type"]) => {
  const sign = type === "credit" ? "+" : "-";
  return `${sign}${new Intl.NumberFormat("vi-VN").format(amount)} đ`;
};

interface TransactionItemProps {
  transaction: Transaction;
}

export const TransactionItem = ({ transaction }: TransactionItemProps) => {
  const Icon = CATEGORY_ICON[transaction.category ?? "other"];
  const isCredit = transaction.type === "credit";

  return (
    <div className="flex items-center gap-3 py-3">
      <span
        className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full ${CATEGORY_COLOR[transaction.category ?? "other"]}`}
      >
        <Icon className="h-5 w-5" />
      </span>

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-gray-900">
          {transaction.description}
        </p>
        {transaction.subtitle && (
          <p className="truncate text-xs text-gray-500">
            {transaction.subtitle}
          </p>
        )}
      </div>

      <span className="flex flex-shrink-0 items-center gap-1 text-sm font-semibold text-pink-600">
        {isCredit ? (
          <TrendUpIcon className="h-3.5 w-3.5 text-green-500" />
        ) : (
          <TrendDownIcon className="h-3.5 w-3.5 text-red-500" />
        )}
        {formatSignedCurrency(transaction.amount, transaction.type)}
      </span>
    </div>
  );
};
