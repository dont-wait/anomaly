import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type FocusEvent,
} from "react";

import { moneyInWords } from "@/features/loans/format";
import { suggestAmounts } from "@/features/transfer/utils/amountSuggestions";

const MAX_DIGITS = 15;
const DEFAULT_SUGGEST_MIN = 1_000;
const DEFAULT_SUGGEST_MAX = 10_000_000_000;

const formatMoney = (value: number) =>
  new Intl.NumberFormat("vi-VN").format(value);

interface MoneyInputProps {
  label: string;
  value: number;
  onChange: (value: number) => void;
  error?: string;
  hint?: string;
  size?: "md" | "lg";
  disabled?: boolean;
  /** Khoảng gợi ý khi đang nhập: gõ 3 → 3.000, 30.000, 300.000... */
  suggestMin?: number;
  suggestMax?: number;
}

/** Ô nhập tiền VND: bàn phím số, tự thêm dấu ngăn cách và giữ vị trí con trỏ. */
export function MoneyInput({
  label,
  value,
  onChange,
  error,
  hint,
  size = "md",
  disabled = false,
  suggestMin = DEFAULT_SUGGEST_MIN,
  suggestMax = DEFAULT_SUGGEST_MAX,
}: MoneyInputProps) {
  const id = useId();
  // Chỉ gợi ý khi khách đang gõ, không gợi ý theo giá trị mặc định hay dữ liệu đã lưu.
  const [typing, setTyping] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const digitsAfterCaret = useRef<number | null>(null);
  const display = value > 0 ? formatMoney(value) : "";

  // Định dạng lại làm con trỏ nhảy về cuối: đặt lại theo số chữ số bên phải nó.
  useLayoutEffect(() => {
    const node = input.current;
    const after = digitsAfterCaret.current;
    if (!node || after === null) return;
    digitsAfterCaret.current = null;
    let position = node.value.length;
    for (let seen = 0; position > 0 && seen < after; position -= 1) {
      if (/\d/.test(node.value[position - 1])) seen += 1;
    }
    node.setSelectionRange(position, position);
  }, [display]);

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { value: text, selectionStart } = event.target;
    digitsAfterCaret.current = text
      .slice(selectionStart ?? text.length)
      .replace(/\D/g, "").length;
    const digits = text.replace(/\D/g, "").slice(0, MAX_DIGITS);
    setTyping(true);
    onChange(digits === "" ? 0 : Number(digits));
  };
  const selectAll = (event: FocusEvent<HTMLInputElement>) =>
    event.target.select();

  const words = moneyInWords(value);
  const suggestions = typing
    ? suggestAmounts(value, suggestMax, suggestMin)
    : [];
  const large = size === "lg";
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-xs font-semibold text-primary">
        {label}
      </label>
      <div
        className={`flex items-center gap-2 rounded-2xl border bg-surface-container-low px-4 focus-within:ring-2 focus-within:ring-secondary/30 ${error ? "border-error" : "border-transparent"} ${large ? "py-3" : "py-2.5"}`}
      >
        <input
          ref={input}
          id={id}
          type="text"
          inputMode="numeric"
          autoComplete="off"
          placeholder="0"
          disabled={disabled}
          value={display}
          aria-invalid={Boolean(error)}
          onChange={handleChange}
          onFocus={selectAll}
          className={`w-full min-w-0 bg-transparent font-bold tracking-tight text-primary outline-none ${large ? "text-3xl" : "text-base"}`}
        />
        <span
          aria-hidden="true"
          className={`font-bold text-secondary ${large ? "text-xl" : "text-sm"}`}
        >
          đ
        </span>
      </div>
      {suggestions.length > 0 && (
        <div
          role="group"
          aria-label={`Gợi ý ${label.toLowerCase()}`}
          className="flex flex-wrap gap-2"
        >
          {suggestions.map((suggestion) => (
            <button
              key={suggestion}
              type="button"
              onClick={() => {
                setTyping(false);
                onChange(suggestion);
              }}
              className="min-h-11 rounded-full bg-white px-3.5 text-sm font-medium tabular-nums text-secondary ring-1 ring-secondary/30 active:bg-secondary/10"
            >
              {formatMoney(suggestion)}
            </button>
          ))}
        </div>
      )}
      {error ? (
        <p role="alert" className="text-xs text-error">
          {error}
        </p>
      ) : (
        (words || hint) && (
          <p className="text-xs text-on-surface-variant">
            {[words, hint].filter(Boolean).join(" · ")}
          </p>
        )
      )}
    </div>
  );
}
