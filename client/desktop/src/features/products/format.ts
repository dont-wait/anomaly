export const panel =
  "rounded-(--admin-radius-md) border border-(--admin-border) bg-(--admin-surface)";
export const fieldLabel =
  "grid gap-1 text-[11px] font-semibold text-(--admin-text-muted)";
export const fieldInput =
  "h-9 w-full rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none focus:border-(--admin-primary)";
export const tableHead =
  "border-b border-(--admin-border) px-2 py-1.5 text-left text-[10px] font-semibold text-(--admin-text-muted)";
export const tableCell =
  "border-b border-(--admin-border) px-2 py-1.5 text-[11px] text-(--admin-text)";

export const vnd = (amount: number) =>
  `${new Intl.NumberFormat("vi-VN").format(amount)} đ`;
export const percent = (bps: number) =>
  `${new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 2 }).format(bps / 100)}%`;
export const date = (value?: string) => {
  const parsed = new Date(value ?? "");
  return Number.isNaN(parsed.getTime())
    ? "—"
    : parsed.toLocaleDateString("vi-VN");
};
