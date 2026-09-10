import {
    ArrowDownLeftIcon,
    BillIcon,
    CoffeeIcon,
    SavingsIcon,
    ShoppingCartIcon,
    TransferIcon,
} from "@/components/icons";
import type { Transaction, TransactionCategory } from "@/types";

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
    food: "bg-orange-100 text-orange-600",
    shopping: "bg-purple-100 text-purple-600",
    "transfer-in": "bg-green-100 text-green-600",
    "transfer-out": "bg-blue-100 text-blue-600",
    bill: "bg-red-100 text-red-600",
    savings: "bg-yellow-100 text-yellow-700",
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
                className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full ${CATEGORY_COLOR[transaction.category ?? "other"]}`}>
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

            <span
                className={`flex-shrink-0 text-sm font-semibold ${isCredit ? "text-success" : "text-error"
                    }`}
            >
                {formatSignedCurrency(transaction.amount, transaction.type)}
            </span>
        </div>
    );
};