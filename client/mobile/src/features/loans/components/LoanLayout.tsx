import { faArrowLeft, faPlus } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { navigate, routes } from "@/app/routes";
import "./loan-layout.css";

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

const LOAN_TABS = [
  { label: "Hồ sơ vay", route: routes.loanApplications },
  { label: "Khoản vay / Trả nợ", route: routes.loanContracts },
];

export function LoanLayout({
  title,
  children,
  overlay = false,
}: {
  title: string;
  children: ReactNode;
  overlay?: boolean;
}) {
  const titleId = useId();
  const currentHash = window.location.hash;
  const surface = useRef<HTMLElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);

  // Runs before the browser drops focus from the list that was just made inert.
  useLayoutEffect(() => {
    const node = surface.current;
    if (!overlay || !node || node.contains(document.activeElement)) return;
    returnFocus.current = document.activeElement as HTMLElement | null;
    node.focus();
  }, [overlay]);

  // A passive cleanup runs after the commit, once the list is no longer inert.
  useEffect(() => {
    if (!overlay) return;
    return () => returnFocus.current?.focus();
  }, [overlay]);

  const onKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === "Escape") {
      event.stopPropagation();
      navigate(routes.loanApplications);
      return;
    }
    if (event.key !== "Tab" || !surface.current) return;
    const items = [...surface.current.querySelectorAll<HTMLElement>(FOCUSABLE)];
    if (items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement;
    if (event.shiftKey && (active === first || active === surface.current)) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  };

  return (
    <div className={overlay ? "loan-overlay" : "loan-shell"}>
      {overlay && (
        <button
          type="button"
          tabIndex={-1}
          aria-label="Đóng chi tiết hồ sơ vay"
          onClick={() => navigate(routes.loanApplications)}
          className="loan-backdrop"
        />
      )}
      <section
        ref={surface}
        role={overlay ? "dialog" : undefined}
        aria-modal={overlay ? true : undefined}
        aria-labelledby={overlay ? titleId : undefined}
        tabIndex={overlay ? -1 : undefined}
        onKeyDown={overlay ? onKeyDown : undefined}
        className={
          overlay ? "loan-surface loan-surface-overlay" : "loan-surface"
        }
      >
        <header className="loan-header">
          <button
            type="button"
            onClick={() => navigate(routes.dashboard)}
            className="loan-home-link"
          >
            <FontAwesomeIcon icon={faArrowLeft} className="mr-1.5" />
            Trang chủ
          </button>
          <h1 id={titleId}>{title}</h1>
        </header>
        <nav className="loan-nav" aria-label="Điều hướng vay vốn">
          <div className="loan-nav-tabs">
            {LOAN_TABS.map((tab) => (
              <button
                key={tab.route}
                type="button"
                aria-current={
                  currentHash.startsWith(tab.route) ? "page" : undefined
                }
                className="loan-nav-button"
                onClick={() => navigate(tab.route)}
              >
                {tab.label}
              </button>
            ))}
          </div>
          <button
            type="button"
            className="loan-nav-create"
            onClick={() => navigate(routes.loanRegister)}
          >
            <FontAwesomeIcon icon={faPlus} className="mr-1.5" />
            Tạo mới hồ sơ vay
          </button>
        </nav>
        <main className="loan-content">{children}</main>
      </section>
    </div>
  );
}
export function Panel({
  title,
  children,
}: {
  title?: string;
  children: ReactNode;
}) {
  return (
    <section className="loan-panel space-y-3">
      {title && <h2>{title}</h2>}
      {children}
    </section>
  );
}
export function Action({
  children,
  onClick,
  disabled = false,
  secondary = false,
}: {
  children: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  secondary?: boolean;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`loan-action ${secondary ? "loan-action-secondary" : ""}`}
    >
      {children}
    </button>
  );
}
export function Notice({
  message,
  error = false,
}: {
  message: string;
  error?: boolean;
}) {
  return (
    <p
      role={error ? "alert" : "status"}
      className={`loan-notice ${error ? "loan-notice-error" : ""}`}
    >
      {message}
    </p>
  );
}
