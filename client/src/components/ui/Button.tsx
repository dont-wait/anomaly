import { type ButtonHTMLAttributes } from "react";

type ButtonVariant = "primary" | "secondary" | "ghost";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
    variant?: ButtonVariant;
}

const variants: Record<ButtonVariant, string> = {
    primary: "bg-blue-600 text-white hover:bg-blue-700",
    secondary: "bg-gray-200 text-gray-800 hover:bg-gray-300",
    ghost: "bg-transparent text-gray-600 hover:bg-gray-100",
};

export const Button = ({
    variant = "primary",
    className = "",
    ...props
}: ButtonProps) => {
    return (
        <button
            className={`rounded-lg px-4 py-2 font-medium transition-colors ${variants[variant]} ${className}`}
            {...props}
        />
    );
};
