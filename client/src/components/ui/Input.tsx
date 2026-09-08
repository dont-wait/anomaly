import { forwardRef, type InputHTMLAttributes } from "react";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
    label?: string;
    rightElement?: React.ReactNode;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
    ({ label, rightElement, ...props }, ref) => {
        return (
            <div className="flex flex-col gap-1">
                {label && (
                    <label className="text-label-md font-semibold text-on-surface-variant">
                        {label}
                    </label>
                )}
                <div className="flex items-center bg-surface-container-lowest/90 px-4 py-3 shadow-xs rounded-2xl">
                    <input
                        ref={ref}
                        className="w-full bg-transparent text-title-lg font-bold tracking-tight text-on-surface focus:outline-none placeholder:text-on-surface-variant/50"
                        {...props}
                    />
                    {rightElement}
                </div>
            </div>
        );
    },
);

Input.displayName = "Input";
