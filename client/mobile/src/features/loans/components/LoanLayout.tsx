import type { ReactNode } from "react";
import { navigate, routes } from "@/app/routes";
import "./loan-layout.css";

export function LoanLayout({
  title,
  children,
  overlay = false,
}: {
  title: string;
  children: ReactNode;
  overlay?: boolean;
}) {
  return (
    <div className={overlay ? "loan-overlay" : "loan-shell"}>
      {overlay && <button type="button" aria-label="Đóng chi tiết hồ sơ vay" onClick={() => navigate(routes.loanApplications)} className="loan-backdrop" />}
      <section role={overlay ? "dialog" : undefined} aria-modal={overlay ? true : undefined} className={overlay ? "loan-surface loan-surface-overlay" : "loan-surface"}>
      <header className="loan-header">
        <button
          type="button"
          onClick={() => navigate(routes.dashboard)}
          className="loan-home-link"
        >
          ← Trang chủ
        </button>
        <h1>{title}</h1>
      </header>
      <nav className="loan-nav">
        <button
          className="loan-nav-button"
          onClick={() => navigate(routes.loanRegister)}
        >
          Đăng ký vay
        </button>
        <button
          className="loan-nav-button"
          onClick={() => navigate(routes.loanApplications)}
        >
          Hồ sơ vay
        </button>
        <button
          className="loan-nav-button"
          onClick={() => navigate(routes.loanContracts)}
        >
          Khoản vay / Trả nợ
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
