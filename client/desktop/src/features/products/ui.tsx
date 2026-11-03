import type { ReactNode } from "react";
import { fieldInput, fieldLabel } from "./format";

export function ActionButton({
  children,
  onClick,
  disabled = false,
  variant = "primary",
}: {
  children: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  variant?: "primary" | "secondary" | "danger";
}) {
  const palette =
    variant === "primary"
      ? "bg-(--admin-primary) text-white hover:bg-(--admin-primary-strong)"
      : variant === "danger"
        ? "border border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)"
        : "border border-(--admin-border) bg-(--admin-surface) text-(--admin-text) hover:border-(--admin-primary)";
  return (
    <button
      type="button"
      className={`h-9 rounded-sm px-3 text-[12px] font-semibold disabled:cursor-not-allowed disabled:opacity-50 ${palette}`}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

export function Pager({
  page,
  hasNext,
  disabled = false,
  onPrevious,
  onNext,
}: {
  page: number;
  hasNext: boolean;
  disabled?: boolean;
  onPrevious: () => void;
  onNext: () => void;
}) {
  if (page <= 1 && !hasNext) return null;
  return (
    <nav aria-label="Phân trang" className="flex items-center gap-2">
      <ActionButton
        variant="secondary"
        disabled={disabled || page <= 1}
        onClick={onPrevious}
      >
        Trang trước
      </ActionButton>
      <span aria-current="page" className="text-[12px] text-(--admin-text)">
        Trang {page}
      </span>
      <ActionButton
        variant="secondary"
        disabled={disabled || !hasNext}
        onClick={onNext}
      >
        Trang sau
      </ActionButton>
    </nav>
  );
}

export function Banner({
  message,
  error = false,
}: {
  message: string;
  error?: boolean;
}) {
  return (
    <div
      role={error ? "alert" : "status"}
      className={`rounded-sm border px-3 py-2 text-[11px] ${
        error
          ? "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)"
          : "border-(--admin-border) bg-(--admin-surface-subtle) text-(--admin-text-muted)"
      }`}
    >
      {message}
    </div>
  );
}

export function PageHeader({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <section className="rounded-(--admin-radius-md) bg-(--admin-surface-subtle) p-4">
      <h1 className="text-[18px] font-bold uppercase tracking-[-0.02em] text-(--admin-text)">
        {title}
      </h1>
      <p className="mt-1 text-[12px] leading-5 text-(--admin-text-muted)">
        {description}
      </p>
    </section>
  );
}

export function MoneyField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className={fieldLabel}>
      {label}
      <input
        className={fieldInput}
        inputMode="numeric"
        value={value ? new Intl.NumberFormat("vi-VN").format(value) : ""}
        onChange={(event) =>
          onChange(Number(event.target.value.replace(/\D/g, "")))
        }
      />
    </label>
  );
}
