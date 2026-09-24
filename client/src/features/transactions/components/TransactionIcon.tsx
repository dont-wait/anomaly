import {
  ArrowDownLeftIcon,
  BillIcon,
  SavingsIcon,
  ShoppingCartIcon,
  TopUpIcon,
  TransferIcon,
  TrendUpIcon,
} from "@/shared/icons";
import type { TransactionRecord } from "@/features/transactions/model";

type IconComponent = (props: { className?: string }) => React.JSX.Element;

const KIND_ICON: Record<TransactionRecord["kind"], IconComponent> = {
  transfer: TransferIcon,
  payment: ShoppingCartIcon,
  bill: BillIcon,
  topup: TopUpIcon,
  savings: SavingsIcon,
  salary: TrendUpIcon,
};

interface TransactionIconProps {
  record: TransactionRecord;
  size?: "md" | "lg";
}

export const TransactionIcon = ({
  record,
  size = "md",
}: TransactionIconProps) => {
  const Icon =
    record.kind === "transfer" && record.direction === "in"
      ? ArrowDownLeftIcon
      : KIND_ICON[record.kind];
  const tone =
    record.direction === "in"
      ? "bg-success-container text-success"
      : "bg-secondary/15 text-secondary";
  const box = size === "lg" ? "h-16 w-16" : "h-11 w-11";
  const glyph = size === "lg" ? "h-8 w-8" : "h-5 w-5";

  return (
    <span
      aria-hidden="true"
      className={`flex shrink-0 items-center justify-center rounded-full ${box} ${tone}`}
    >
      <Icon className={glyph} />
    </span>
  );
};
