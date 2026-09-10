import {
  BillIcon,
  MoreIcon,
  SavingsIcon,
  TopUpIcon,
  TransferIcon,
} from "@/components/icons";
import type { QuickAction } from "@/types";

type IconComponent = (props: { className?: string }) => React.JSX.Element;

const ICONS: Record<QuickAction["icon"], IconComponent> = {
  transfer: TransferIcon,
  topup: TopUpIcon,
  bill: BillIcon,
  savings: SavingsIcon,
  more: MoreIcon,
};

const ICON_COLOR: Record<QuickAction["icon"], string> = {
    transfer: "bg-blue-100 text-blue-600",
    topup: "bg-purple-100 text-purple-600",
    bill: "bg-red-100 text-red-600",
    savings: "bg-green-100 text-green-600",
    more: "bg-gray-100 text-gray-600",
};

interface QuickActionsProps {
  actions: QuickAction[];
  onSelect?: (action: QuickAction) => void;
}

/** Hàng icon chức năng nhanh: Chuyển tiền, Nạp tiền ĐT, Hóa đơn, Tiết kiệm, Xem thêm. */
export const QuickActions = ({ actions, onSelect }: QuickActionsProps) => {
  return (
    <div className="grid grid-cols-5 gap-2">
      {actions.map((action) => {
        const Icon = ICONS[action.icon];
        return (
          <button
            key={action.id}
            onClick={() => onSelect?.(action)}
            className="flex flex-col items-center gap-2"
          >
            <span
                className={`flex h-14 w-14 items-center justify-center rounded-full ${ICON_COLOR[action.icon]}`}>
                <Icon className="h-7 w-7" />
            </span>
            <span className="text-center text-[11px] leading-tight text-gray-600">
              {action.label}
            </span>
          </button>
        );
      })}
    </div>
  );
};
