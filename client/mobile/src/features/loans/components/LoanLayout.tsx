import type { ReactNode } from "react";
import { navigate, routes } from "@/app/routes";

export function LoanLayout({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <div className="mx-auto min-h-screen max-w-md bg-background text-on-surface">
      <header className="sticky top-0 z-10 flex items-center justify-between border-b bg-background px-4 py-4">
        <button
          type="button"
          onClick={() => navigate(routes.dashboard)}
          className="text-sm font-bold text-primary"
        >
          ← Trang chủ
        </button>
        <h1 className="text-base font-bold text-primary">{title}</h1>
      </header>
      <nav className="flex flex-wrap gap-2 border-b px-4 py-3 text-xs font-semibold">
        <button
          className="rounded-full bg-secondary-container px-3 py-2"
          onClick={() => navigate(routes.loanRegister)}
        >
          Đăng ký vay
        </button>
        <button
          className="rounded-full bg-secondary-container px-3 py-2"
          onClick={() => navigate(routes.loanApplications)}
        >
          Hồ sơ vay
        </button>
        <button
          className="rounded-full bg-secondary-container px-3 py-2"
          onClick={() => navigate(routes.loanContracts)}
        >
          Khoản vay / Trả nợ
        </button>
      </nav>
      <main className="space-y-4 px-4 py-5 pb-10">{children}</main>
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
    <section className="space-y-3 rounded-2xl bg-white p-4 shadow-sm">
      {title && <h2 className="text-sm font-bold text-primary">{title}</h2>}
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
      className={`w-full rounded-xl px-3 py-3 text-sm font-semibold disabled:opacity-40 ${secondary ? "border border-primary/20 bg-white text-primary" : "bg-secondary text-white"}`}
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
      className={`rounded-xl p-3 text-xs ${error ? "bg-error-container text-on-error-container" : "bg-surface-container text-primary"}`}
    >
      {message}
    </p>
  );
}
