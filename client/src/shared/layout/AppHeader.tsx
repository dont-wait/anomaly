import { Logo } from "@/shared/layout/Logo";
import { IconButton } from "@/shared/ui";
import { BellIcon, UserIcon } from "@/shared/icons";

interface AppHeaderProps {
  /** Tên trang hiện tại — không truyền thì chỉ hiện logo */
  title?: string;
  notificationCount?: number;
  onNotificationClick?: () => void;
  onProfileClick?: () => void;
}

export const AppHeader = ({
  title,
  notificationCount,
  onNotificationClick,
  onProfileClick,
}: AppHeaderProps) => {
  return (
    <header className="flex items-center justify-between border-b border-gray-100 bg-white px-4 py-3">
      <div className="flex items-center gap-3">
        <Logo />
        {title && (
          <>
            <span className="h-6 w-px bg-gray-200" />
            <span className="text-sm font-semibold text-gray-700">{title}</span>
          </>
        )}
      </div>

      <div className="flex items-center gap-2">
        <IconButton
          icon={<BellIcon className="h-5 w-5" />}
          badgeCount={notificationCount}
          aria-label="Thông báo"
          onClick={onNotificationClick}
          tone="default"
        />
        <IconButton
          icon={<UserIcon className="h-4.5 w-4.5" />}
          aria-label="Hồ sơ"
          onClick={onProfileClick}
          tone="brand"
        />
      </div>
    </header>
  );
};
