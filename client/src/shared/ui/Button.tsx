import { type ButtonHTMLAttributes } from "react";

type ButtonVariant = "primary" | "secondary" | "ghost";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

const variants: Record<ButtonVariant, string> = {
  // Gradient 135° + bóng mauve của nút Đăng nhập, lấy màu từ token `cta`
  primary:
    "bg-linear-135/srgb from-cta-from to-cta-to text-on-cta shadow-[0_4px_18px_-2px_color-mix(in_srgb,var(--color-cta-from)_45%,transparent)]",
  secondary: "bg-secondary/10 text-secondary-strong hover:bg-secondary/15",
  ghost: "bg-transparent text-secondary-strong hover:bg-secondary/10",
};

/** Nút dùng chung toàn app — kích thước, bo góc, chữ khớp nút Đăng nhập. */
export const Button = ({
  variant = "primary",
  type = "button",
  className = "",
  ...props
}: ButtonProps) => {
  return (
    <button
      type={type}
      className={`flex cursor-pointer items-center justify-center gap-3 rounded-2xl px-5 py-3.5 text-center text-base leading-6 font-semibold tracking-wide transition-all focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:ring-offset-2 focus-visible:outline-none active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-[0.38] disabled:active:scale-100 sm:py-4 ${variants[variant]} ${className}`}
      {...props}
    />
  );
};
