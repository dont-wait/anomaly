import { useState } from "react";
import {
  CardIcon,
  HomeIcon,
  SavingsIcon,
  QrIcon,
  UserIcon,
} from "@/shared/icons";

type IconComponent = (props: { className?: string }) => React.JSX.Element;

interface NavItem {
  id: string;
  label: string;
  icon: IconComponent;
}

const NAV_ITEMS: NavItem[] = [
  { id: "home", label: "Trang chủ", icon: HomeIcon },
  { id: "card", label: "Thẻ", icon: CardIcon },
  { id: "savings", label: "Tiết kiệm", icon: SavingsIcon },
  { id: "profile", label: "Cá nhân", icon: UserIcon },
];

interface BottomNavProps {
  active?: string;
  onChange?: (id: string) => void;
  /** Gọi khi bấm nút QR nổi giữa — nối logic mở camera/scan thật vào đây sau */
  onQrScan?: () => void;
}

/**
 * Thanh điều hướng dưới cùng, 5 mục: 4 tab thường + 1 nút QR
 * nổi ở giữa. `active`/`onChange` để điều khiển từ bên ngoài (vd router);
 * nếu không truyền thì tự quản lý state nội bộ để demo UI.
 */
export const BottomNav = ({ active, onChange, onQrScan }: BottomNavProps) => {
  const [internalActive, setInternalActive] = useState("home");
  const currentActive = active ?? internalActive;

  const handleSelect = (id: string) => {
    setInternalActive(id);
    onChange?.(id);
  };

  const [left, right] = [NAV_ITEMS.slice(0, 2), NAV_ITEMS.slice(2)];

  return (
    <nav className="relative z-20 -mt-[72px] flex items-center justify-between rounded-t-3xl border-t border-white/60 bg-white/95 px-4 pt-2 pb-3 shadow-[0_-8px_24px_rgb(0_35_111/10%)] backdrop-blur-xl backdrop-saturate-150 supports-[backdrop-filter]:bg-white/80">
      {left.map((item) => (
        <NavButton
          key={item.id}
          item={item}
          isActive={currentActive === item.id}
          onClick={() => handleSelect(item.id)}
        />
      ))}

      <button
        aria-label="Quét mã QR"
        onClick={onQrScan}
        className="-mt-7 flex h-16 w-16 flex-col items-center justify-center rounded-full bg-gradient-to-br from-secondary to-[#8e5d8e] text-white ring-4 ring-white/70 shadow-lg shadow-secondary/40 ring-4 ring-white transition-transform hover:scale-105 active:scale-95"
      >
        <QrIcon className="h-6 w-6" />
      </button>

      {right.map((item) => (
        <NavButton
          key={item.id}
          item={item}
          isActive={currentActive === item.id}
          onClick={() => handleSelect(item.id)}
        />
      ))}
    </nav>
  );
};

const NavButton = ({
  item,
  isActive,
  onClick,
}: {
  item: NavItem;
  isActive: boolean;
  onClick: () => void;
}) => {
  const Icon = item.icon;
  return (
    <button
      onClick={onClick}
      aria-current={isActive ? "page" : undefined}
      className="flex flex-col items-center gap-1 px-2 text-[11px] font-medium transition-colors"
    >
      <span
        className={`flex h-8 w-8 items-center justify-center rounded-full transition-colors ${
          isActive ? "bg-secondary/15 text-secondary" : "text-gray-600"
        }`}
      >
        <Icon className="h-5 w-5" />
      </span>
      <span className={isActive ? "text-secondary" : "text-gray-600"}>
        {item.label}
      </span>
    </button>
  );
};
