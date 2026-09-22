import { useState } from "react";
import { OTP_LENGTH } from "@/features/transfer/model";

interface OtpInputProps {
  id: string;
  value: string;
  onChange: (value: string) => void;
  invalid?: boolean;
  disabled?: boolean;
  describedBy?: string;
}

/**
 * Ô OTP 6 chữ số: một input thật (hỗ trợ dán, tự điền `one-time-code`, trình đọc màn hình)
 * phủ trong suốt lên 6 ô hiển thị.
 */
export function OtpInput({
  id,
  value,
  onChange,
  invalid = false,
  disabled = false,
  describedBy,
}: OtpInputProps) {
  const [focused, setFocused] = useState(false);

  return (
    <div className="relative">
      <input
        id={id}
        autoFocus
        inputMode="numeric"
        autoComplete="one-time-code"
        maxLength={OTP_LENGTH}
        value={value}
        disabled={disabled}
        onChange={(e) =>
          onChange(e.target.value.replace(/\D/g, "").slice(0, OTP_LENGTH))
        }
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        aria-invalid={invalid}
        aria-describedby={describedBy}
        className="absolute inset-0 z-10 h-full w-full cursor-text text-base opacity-0 disabled:cursor-not-allowed"
      />
      <div aria-hidden="true" className="grid grid-cols-6 gap-2">
        {Array.from({ length: OTP_LENGTH }, (_, index) => {
          const digit = value[index];
          const isActive =
            focused &&
            !disabled &&
            index === Math.min(value.length, OTP_LENGTH - 1);
          const ring = invalid
            ? "ring-2 ring-error"
            : isActive
              ? "ring-2 ring-secondary"
              : digit
                ? "ring-1 ring-secondary/40"
                : "ring-1 ring-outline-variant";
          return (
            <span
              key={index}
              className={`flex h-14 items-center justify-center rounded-2xl bg-surface-container-low text-2xl font-bold text-on-surface tabular-nums transition-shadow ${ring} ${disabled ? "opacity-60" : ""}`}
            >
              {digit ??
                (isActive ? (
                  <span className="h-6 w-0.5 animate-pulse rounded-full bg-secondary motion-reduce:animate-none" />
                ) : null)}
            </span>
          );
        })}
      </div>
    </div>
  );
}
