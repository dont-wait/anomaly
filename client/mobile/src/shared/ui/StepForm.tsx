import { useEffect, useRef, type FormEvent, type ReactNode } from "react";
import { faArrowLeft, faCheck } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

interface StepFormProps {
  /** Tên cả luồng, ví dụ "Đăng ký vay vốn". */
  title: string;
  stepId: string;
  stepNumber: number;
  stepCount: number;
  heading: string;
  hint?: string;
  canContinue: boolean;
  continueLabel?: string;
  busy?: boolean;
  onBack: () => void;
  onNext: () => void;
  children: ReactNode;
}

/** Khung form từng bước: mỗi màn hình một ô, nút tiếp tục luôn ở cuối màn hình. */
export function StepForm({
  title,
  stepId,
  stepNumber,
  stepCount,
  heading,
  hint,
  canContinue,
  continueLabel = "Tiếp tục",
  busy = false,
  onBack,
  onNext,
  children,
}: StepFormProps) {
  const body = useRef<HTMLElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);

  // Mỗi bước mới bắt đầu từ đầu màn hình, focus vào ô nhập nếu bước đó có ô nhập.
  useEffect(() => {
    if (body.current) body.current.scrollTop = 0;
    const field = body.current?.querySelector<HTMLElement>(
      "input:not([type=file]):not([type=checkbox]), textarea",
    );
    (field ?? headingRef.current)?.focus();
  }, [stepId]);

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (canContinue && !busy) onNext();
  };

  return (
    <form
      noValidate
      onSubmit={submit}
      className="mx-auto flex h-dvh max-w-md flex-col bg-background text-on-surface"
    >
      <header className="shrink-0 border-b border-primary/5 px-4 pt-3 pb-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            aria-label="Quay lại bước trước"
            onClick={onBack}
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-primary active:bg-surface-container"
          >
            <FontAwesomeIcon icon={faArrowLeft} />
          </button>
          <h1 className="min-w-0 flex-1 truncate text-sm font-bold text-primary">
            {title}
          </h1>
          <span className="shrink-0 text-xs font-semibold text-on-surface-variant">
            Bước {stepNumber}/{stepCount}
          </span>
        </div>
        <div
          role="progressbar"
          aria-label={heading}
          aria-valuemin={1}
          aria-valuemax={stepCount}
          aria-valuenow={stepNumber}
          className="mt-2 h-1.5 overflow-hidden rounded-full bg-surface-container"
        >
          <span
            className="block h-full rounded-full bg-secondary transition-[width] motion-reduce:transition-none"
            style={{ width: `${(stepNumber / stepCount) * 100}%` }}
          />
        </div>
      </header>

      <main ref={body} className="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <div>
          <h2
            ref={headingRef}
            tabIndex={-1}
            className="text-xl font-bold text-primary outline-none"
          >
            {heading}
          </h2>
          {hint && (
            <p className="mt-1 text-sm text-on-surface-variant">{hint}</p>
          )}
        </div>
        {children}
      </main>

      <footer className="grid shrink-0 grid-cols-[1fr_2fr] gap-2 border-t border-primary/5 bg-background p-4">
        <button
          type="button"
          disabled={busy}
          onClick={onBack}
          className="min-h-12 rounded-full bg-surface-container px-4 text-base font-bold text-primary disabled:opacity-40"
        >
          Quay lại
        </button>
        <button
          type="submit"
          disabled={!canContinue || busy}
          className="flex min-h-12 items-center justify-center rounded-full bg-primary px-4 text-base font-bold text-white disabled:opacity-40"
        >
          {busy ? "Đang xử lý..." : continueLabel}
        </button>
      </footer>
    </form>
  );
}

export interface ChoiceOption {
  value: string;
  label: string;
  description?: string;
  disabled?: boolean;
}

const DOUBLE_TAP_MS = 400;

/** Chạm một lần để chọn; chạm hai lần liên tiếp vào cùng mục để đi tiếp. */
export function ChoiceList({
  label,
  value,
  options,
  onChoose,
  onConfirm,
}: {
  label: string;
  value: string;
  options: ChoiceOption[];
  onChoose: (value: string) => void;
  onConfirm: () => void;
}) {
  // Tự đo thời gian vì dblclick không ổn định khi chạm trong WebView Android.
  const lastTap = useRef<{ value: string; at: number } | null>(null);
  const tap = (next: string, at: number) => {
    const previous = lastTap.current;
    if (previous?.value === next && at - previous.at <= DOUBLE_TAP_MS) {
      lastTap.current = null;
      onConfirm();
      return;
    }
    lastTap.current = { value: next, at };
    onChoose(next);
  };

  return (
    <div className="flex flex-col gap-2">
      <div role="radiogroup" aria-label={label} className="flex flex-col gap-2">
        {options.map((option) => {
          const active = option.value === value;
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={active}
              disabled={option.disabled}
              onClick={(event) => tap(option.value, event.timeStamp)}
              className={`flex min-h-14 w-full items-center justify-between gap-3 rounded-2xl border px-4 py-3 text-left disabled:opacity-40 ${
                active
                  ? "border-secondary bg-secondary-container"
                  : "border-outline-variant/40 bg-white"
              }`}
            >
              <span className="min-w-0">
                <span className="block text-sm font-semibold text-primary">
                  {option.label}
                </span>
                {option.description && (
                  <span className="mt-0.5 block text-xs text-on-surface-variant">
                    {option.description}
                  </span>
                )}
              </span>
              {active && (
                <FontAwesomeIcon
                  icon={faCheck}
                  className="shrink-0 text-secondary"
                />
              )}
            </button>
          );
        })}
      </div>
      <p className="text-center text-xs text-on-surface-variant/70">
        Bấm Tiếp tục hoặc chạm 2 lần vào lựa chọn để đi tiếp
      </p>
    </div>
  );
}
