import { useState, type ReactNode } from "react";
import type { IconDefinition } from "@fortawesome/fontawesome-svg-core";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowRightFromBracket,
  faBell,
  faBolt,
  faChartLine,
  faChevronLeft,
  faChevronRight,
  faClockRotateLeft,
  faCode,
  faFileInvoiceDollar,
  faGaugeHigh,
  faIdCard,
  faMagnifyingGlass,
  faMoneyBillTransfer,
  faPlay,
  faTerminal,
  faTriangleExclamation,
  faUserCircle,
} from "@fortawesome/free-solid-svg-icons";
import { clearAdminSession } from "@/features/auth/api/adminSession";
import { navigate, routes, type ProtectedRoute } from "@/app/routes";

interface NavigationItem {
  label: string;
  route: ProtectedRoute;
  icon: IconDefinition;
  badge?: string;
}

interface NavigationGroup {
  label: string;
  items: NavigationItem[];
}

interface AdminShellProps {
  currentRoute: ProtectedRoute;
  children: ReactNode;
  onLogout?: () => void;
}

const navigationGroups: NavigationGroup[] = [
  {
    label: "TỔNG QUAN",
    items: [
      { label: "Dashboard", route: routes.monitor, icon: faGaugeHigh },
      {
        label: "Cảnh báo",
        route: routes.alerts,
        icon: faTriangleExclamation,
        badge: "38",
      },
      {
        label: "Tài khoản rủi ro",
        route: routes.riskAccounts,
        icon: faChartLine,
      },
    ],
  },
  {
    label: "ĐIỀU TRA",
    items: [
      { label: "Event Explorer", route: routes.audit, icon: faCode },
      {
        label: "Timeline",
        route: routes.auditAccounts,
        icon: faClockRotateLeft,
      },
      { label: "Replay Console", route: routes.auditReplay, icon: faPlay },
    ],
  },
  {
    label: "KHOẢN VAY",
    items: [
      {
        label: "Thẩm định hồ sơ",
        route: routes.loanApplications,
        icon: faFileInvoiceDollar,
        badge: "5",
      },
      {
        label: "Gói vay",
        route: routes.loanPackages,
        icon: faMoneyBillTransfer,
      },
    ],
  },
  {
    label: "ĐỊNH DANH",
    items: [
      { label: "Duyệt KYC", route: routes.kyc, icon: faIdCard, badge: "8" },
    ],
  },
  {
    label: "BÁO CÁO",
    items: [
      {
        label: "Giao dịch & Rủi ro",
        route: routes.reportsTransactions,
        icon: faChartLine,
      },
    ],
  },
];

export function AdminShell({
  currentRoute,
  children,
  onLogout = () => {
    clearAdminSession();
    navigate(routes.login);
  },
}: AdminShellProps) {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const shellColumns = isCollapsed
    ? "grid-cols-[64px_minmax(0,1fr)]"
    : "grid-cols-[308px_minmax(0,1fr)]";

  return (
    <div
      className={`grid min-h-screen ${shellColumns} bg-(--admin-background) text-(--admin-text) transition-[grid-template-columns] duration-200 ease-in-out motion-reduce:transition-none max-[900px]:grid-cols-[64px_minmax(0,1fr)]`}
    >
      <aside
        className="flex min-h-screen flex-col overflow-hidden border-r border-(--admin-border) bg-(--admin-surface)"
        aria-label="Điều hướng admin"
      >
        <div
          className={`flex h-17.5 shrink-0 items-center border-b border-(--admin-border) px-4 max-[900px]:justify-center max-[900px]:px-2 ${isCollapsed ? "justify-center px-2" : "justify-start"}`}
        >
          <div
            className={`flex min-w-0 items-center max-[900px]:gap-0 ${isCollapsed ? "gap-0" : "gap-3"}`}
          >
            <div className="grid h-8 w-8 shrink-0 place-items-center rounded-(--admin-radius-md) border border-(--admin-primary) bg-(--admin-primary) text-white">
              <FontAwesomeIcon
                icon={faTerminal}
                className="text-[13px]"
                aria-hidden="true"
              />
            </div>
            <div
              className={`grid min-w-0 overflow-hidden transition-[max-width,opacity,transform] duration-200 ease-in-out motion-reduce:transition-none max-[900px]:hidden ${isCollapsed ? "max-w-0 -translate-x-2 opacity-0" : "max-w-45 translate-x-0 opacity-100"}`}
            >
              <span className="font-(family-name:--admin-font-sans) text-[15px] leading-5 font-semibold tracking-[0.04em] text-(--admin-text)">
                ANOMALY
              </span>
              <span className="font-(family-name:--admin-font-mono) text-[10px] leading-3.5 tracking-[0.02em] text-(--admin-text-muted)">
                CONSOLE V0.4
              </span>
            </div>
          </div>
        </div>

        <nav
          className={`w-full flex-1 overflow-y-auto py-4 ${isCollapsed ? "px-2" : "px-3"} max-[900px]:px-2`}
          aria-label="Menu admin"
        >
          {navigationGroups.map((group) => (
            <div
              className={`transition-[margin] duration-200 ease-in-out motion-reduce:transition-none max-[900px]:mb-2 ${isCollapsed ? "mb-2" : "mb-5"}`}
              key={group.label}
            >
              <div
                className={`flex items-center overflow-hidden font-(family-name:--admin-font-sans) text-[10px] leading-4 font-semibold tracking-[0.09em] text-(--admin-text-muted) transition-[max-height,opacity,padding] duration-200 ease-in-out motion-reduce:transition-none max-[900px]:mx-auto max-[900px]:mb-2 max-[900px]:h-px max-[900px]:w-5 max-[900px]:bg-(--admin-border) max-[900px]:p-0 max-[900px]:text-[0px] ${isCollapsed ? "mx-auto mb-2 h-px max-h-px w-5 bg-(--admin-border) p-0 text-[0px]" : "max-h-8 px-2 pb-2"}`}
              >
                {group.label}
              </div>
              <div className="grid gap-1">
                {group.items.map((item) => {
                  const active = currentRoute === item.route;
                  return (
                    <button
                      className={`relative flex h-10 w-full items-center gap-3 rounded-(--admin-radius-md) border px-3 text-left text-[13px] font-medium transition-colors focus-visible:outline-1 focus-visible:outline-offset-1 focus-visible:outline-(--admin-primary) max-[900px]:justify-center max-[900px]:px-0 ${isCollapsed ? "justify-center px-0" : ""} ${
                        active
                          ? "border-(--admin-border) bg-(--admin-surface-hover) text-(--admin-text) before:absolute before:bottom-1.5 before:left-0 before:top-1.5 before:w-0.5 before:bg-(--admin-primary)"
                          : "border-transparent text-(--admin-text-secondary) hover:bg-(--admin-surface-subtle) hover:text-(--admin-text)"
                      }`}
                      key={item.route}
                      title={item.label}
                      type="button"
                      aria-current={active ? "page" : undefined}
                      onClick={() => navigate(item.route)}
                    >
                      <FontAwesomeIcon
                        className="w-4 shrink-0 text-[14px]"
                        icon={item.icon}
                        aria-hidden="true"
                      />
                      <span
                        className={`min-w-0 flex-1 truncate max-[900px]:hidden ${isCollapsed ? "hidden" : ""}`}
                      >
                        {item.label}
                      </span>
                      {item.badge && (
                        <span
                          className={`border border-[#7f1d1d] bg-[#c1121f] font-(family-name:--admin-font-mono) text-white max-[900px]:absolute max-[900px]:top-1.5 max-[900px]:right-1.5 max-[900px]:h-1.5 max-[900px]:min-w-0 max-[900px]:w-1.5 max-[900px]:rounded-full max-[900px]:p-0 max-[900px]:text-[0px] ${isCollapsed ? "absolute top-1.5 right-1.5 h-1.5 w-1.5 rounded-full p-0 text-[0px]" : "min-w-5 rounded-[3px] px-1.5 py-0.5 text-center text-[10px] leading-4"}`}
                          aria-label={`${item.badge} mục cần xử lý`}
                        >
                          {item.badge}
                        </span>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>

        <div className="w-full shrink-0 border-t border-(--admin-border)">
          <div
            className={`flex min-h-16 items-center gap-3 px-3 py-2.5 max-[900px]:justify-center max-[900px]:px-2 ${isCollapsed ? "justify-center px-2" : "justify-between"}`}
          >
            <div
              className="hidden h-8 w-8 shrink-0 place-items-center rounded-full bg-[#b4c5ff] text-[#002a78] max-[900px]:grid"
              aria-hidden="true"
            >
              <FontAwesomeIcon icon={faUserCircle} />
            </div>
            <div
              className={`min-w-0 flex-1 max-[900px]:hidden ${isCollapsed ? "hidden" : "grid"}`}
            >
              <strong className="truncate text-[13px] leading-5 font-semibold text-(--admin-text)">
                Lê Văn C.
              </strong>
              <span className="truncate text-[11px] leading-4 text-(--admin-text-secondary)">
                Tier-1 Risk Officer
              </span>
            </div>
            <button
              className="grid h-8 w-8 shrink-0 place-items-center rounded-(--admin-radius-md) border border-(--admin-border) text-[11px] text-(--admin-text-muted) hover:bg-(--admin-surface-hover) hover:text-(--admin-text) focus-visible:outline focus-visible:outline-offset-1 focus-visible:outline-(--admin-primary) max-[900px]:hidden"
              type="button"
              aria-label={isCollapsed ? "Mở rộng sidebar" : "Thu gọn sidebar"}
              aria-pressed={isCollapsed}
              title={isCollapsed ? "Mở rộng sidebar" : "Thu gọn sidebar"}
              onClick={() => setIsCollapsed((collapsed) => !collapsed)}
            >
              <FontAwesomeIcon
                icon={isCollapsed ? faChevronRight : faChevronLeft}
                aria-hidden="true"
              />
            </button>
          </div>
        </div>
      </aside>

      <div className="min-w-0 bg-(--admin-background)">
        <header className="flex h-17.5 items-center justify-between gap-4 border-b border-(--admin-border) bg-(--admin-surface) px-5 max-[900px]:px-3">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex shrink-0 items-center gap-1 font-(family-name:--admin-font-mono) text-[12px] tracking-[0.02em] text-(--admin-text-secondary) max-[700px]:hidden">
              <span>ADMIN</span>
              <span className="text-(--admin-text-muted)">/</span>
              <span>CONSOLE</span>
              <span className="text-(--admin-primary)">/ WORKSPACE</span>
            </div>
            <label className="flex h-9 w-82.5 items-center gap-2.5 rounded-(--admin-radius-md) border border-(--admin-border) bg-(--admin-background) px-3 text-(--admin-text-muted) focus-within:border-(--admin-primary) focus-within:shadow-(--admin-focus-ring) max-[1100px]:hidden">
              <FontAwesomeIcon
                icon={faMagnifyingGlass}
                className="w-3.5 text-[12px]"
                aria-hidden="true"
              />
              <input
                className="min-w-0 flex-1 border-0 bg-transparent font-(family-name:--admin-font-sans) text-xs text-(--admin-text) outline-none placeholder:text-(--admin-text-muted)"
                aria-label="Lọc stream hoặc entity ID"
                placeholder="Filter stream or entity ID..."
              />
              <kbd className="font-(family-name:--admin-font-mono) text-[10px] text-(--admin-text-muted)">
                /
              </kbd>
            </label>
          </div>
          <div className="flex items-center gap-2">
            <span className="rounded-(--admin-radius-md) border border-(--admin-border) px-2.5 py-1.5 font-(family-name:--admin-font-mono) text-[10px] tracking-[0.03em] text-(--admin-text-secondary) max-[1200px]:hidden">
              PROD · V0.4
            </span>
            <span className="rounded-(--admin-radius-md) border border-(--admin-border) px-2.5 py-1.5 font-(family-name:--admin-font-mono) text-[10px] tracking-[0.03em] text-(--admin-text-secondary) max-[600px]:hidden">
              UTC+7
            </span>
            <button
              className="flex h-9 items-center gap-2 rounded-(--admin-radius-md) border border-[#1d4ed8] bg-[#2563eb] px-3 font-(family-name:--admin-font-sans) text-[13px] font-semibold text-white hover:bg-[#1d4ed8] max-[1100px]:hidden"
              type="button"
            >
              <FontAwesomeIcon icon={faBolt} aria-hidden="true" />
              Nhận việc tiếp theo
            </button>
            <button
              className="relative grid h-8 w-8 place-items-center text-(--admin-text-secondary) hover:text-(--admin-text)"
              type="button"
              aria-label="Thông báo"
              title="Thông báo"
            >
              <FontAwesomeIcon icon={faBell} aria-hidden="true" />
              <span
                className="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-[#c1121f]"
                aria-hidden="true"
              />
            </button>
            <button
              className="grid h-9 w-9 place-items-center rounded-full bg-[#b4c5ff] text-[#002a78]"
              type="button"
              aria-label="Tài khoản operator"
              title="Tài khoản operator"
            >
              <FontAwesomeIcon icon={faUserCircle} aria-hidden="true" />
            </button>
            <button className="sr-only" type="button" onClick={onLogout}>
              <FontAwesomeIcon
                icon={faArrowRightFromBracket}
                aria-hidden="true"
              />
              Đăng xuất
            </button>
          </div>
        </header>

        <main className="w-full p-4 max-[900px]:p-3">{children}</main>
      </div>
    </div>
  );
}
