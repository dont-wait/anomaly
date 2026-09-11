import {
  BillIcon,
  MoreIcon,
  SavingsIcon,
  TopUpIcon,
  TransferIcon,
} from "@/shared/icons";
import type { QuickAction } from "@/features/dashboard/model/types";

type IconComponent = (props: { className?: string }) => React.JSX.Element;

const ICONS: Record<QuickAction["icon"], IconComponent> = {
  transfer: TransferIcon,
  topup: TopUpIcon,
  bill: BillIcon,
  savings: SavingsIcon,
  more: MoreIcon,
};

const ICON_COLOR: Record<QuickAction["icon"], string> = {
  transfer: "bg-secondary/10 text-secondary",
  topup: "bg-violet-100 text-violet-600",
  bill: "bg-fuchsia-100 text-fuchsia-600",
  savings: "bg-purple-100 text-purple-700",
  more: "bg-gray-100 text-gray-500",
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
              className={`flex h-14 w-14 items-center justify-center rounded-full ${ICON_COLOR[action.icon]}`}
            >
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
