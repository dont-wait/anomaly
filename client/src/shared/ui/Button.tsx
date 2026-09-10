import { type ButtonHTMLAttributes } from "react";

type ButtonVariant = "primary" | "secondary" | "ghost";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

const variants: Record<ButtonVariant, string> = {
  primary:
    "bg-primary-container text-on-primary hover:opacity-90 active:scale-[0.98] shadow-lg shadow-primary-container/25",
  secondary:
    "bg-surface-container-high/80 text-primary-container border border-white/60 hover:bg-surface-variant backdrop-blur-md",
  ghost: "bg-transparent text-primary-container hover:bg-secondary/8",
};

export const Button = ({
  variant = "primary",
  className = "",
  ...props
}: ButtonProps) => {
  return (
    <button
      className={`rounded-full px-6 py-3.5 font-semibold text-title-md tracking-wide transition-all cursor-pointer ${variants[variant]} ${className}`}
      {...props}
    />
  );
};
