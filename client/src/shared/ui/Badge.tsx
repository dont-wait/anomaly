import { type HTMLAttributes } from "react";

type BadgeVariant = "light" | "outline";

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
}

const variants: Record<BadgeVariant, string> = {
  light: "bg-white/20 text-white",
  outline: "border border-current text-secondary",
};

/** Nhãn nhỏ dạng viên thuốc, dùng cho tag "SIGNATURE", trạng thái, v.v. */
export const Badge = ({
  variant = "light",
  className = "",
  ...props
}: BadgeProps) => {
  return (
    <span
      className={`rounded-full px-2 py-0.5 text-[10px] font-semibold tracking-wide ${variants[variant]} ${className}`}
      {...props}
    />
  );
};
