import { useState } from "react";
import {
  CardIcon,
  HomeIcon,
  SavingsIcon,
  QrIcon,
  UserIcon,
} from "@/components/icons";

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
}

/**
 * Thanh điều hướng dưới cùng, 5 mục: 4 tab thường + 1 nút "Chuyển tiền"
 * nổi ở giữa. `active`/`onChange` để điều khiển từ bên ngoài (vd router);
 * nếu không truyền thì tự quản lý state nội bộ để demo UI.
 */
export const BottomNav = ({ active, onChange }: BottomNavProps) => {
  const [internalActive, setInternalActive] = useState("home");
  const currentActive = active ?? internalActive;

  const handleSelect = (id: string) => {
    setInternalActive(id);
    onChange?.(id);
  };

  const [left, right] = [NAV_ITEMS.slice(0, 2), NAV_ITEMS.slice(2)];

  return (
    <nav className="flex items-center justify-between border-t border-gray-100 bg-white px-4 pt-2 pb-3">
      {left.map((item) => (
        <NavButton
          key={item.id}
          item={item}
          isActive={currentActive === item.id}
          onClick={() => handleSelect(item.id)}
        />
      ))}

      <button
        aria-label="Chuyển tiền"
        onClick={() => handleSelect("qr")}
        className="-mt-6 flex h-14 w-14 flex-col items-center justify-center rounded-full bg-brand-primary text-white shadow-lg shadow-brand-primary/30 transition-transform hover:scale-105"
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
      className={`flex flex-col items-center gap-1 px-2 text-[11px] font-medium ${
        isActive ? "text-brand-primary" : "text-gray-400"
      }`}
    >
      <Icon className="h-5 w-5" />
      {item.label}
    </button>
  );
};
