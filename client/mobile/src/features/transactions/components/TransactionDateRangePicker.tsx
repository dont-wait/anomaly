import { useEffect, useRef, useState } from "react";
import {
  faCalendarDays,
  faChevronLeft,
  faChevronRight,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import type { TransactionDateRange } from "@/features/transactions/model";
import {
  addMonths,
  dateValueFromDate,
  dateValueToDate,
  formatDateValue,
  formatMonthValue,
  isDateInRange,
  startOfMonth,
} from "@/features/transactions/utils/dateRange";

const WEEKDAYS = ["T2", "T3", "T4", "T5", "T6", "T7", "CN"];

const calendarDays = (month: string) => {
  const first = dateValueToDate(startOfMonth(month));
  const leadingDays = (first.getUTCDay() + 6) % 7;
  const daysInMonth = new Date(
    Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0, 12),
  ).getUTCDate();

  return Array.from({ length: leadingDays + daysInMonth }, (_, index) => {
    if (index < leadingDays) return null;
    const day = index - leadingDays + 1;
    return dateValueFromDate(
      new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth(), day, 12)),
    );
  });
};

interface TransactionDateRangePickerProps {
  value: TransactionDateRange;
  minDate: string;
  maxDate: string;
  onConfirm: (range: TransactionDateRange) => void;
}

export const TransactionDateRangePicker = ({
  value,
  minDate,
  maxDate,
  onConfirm,
}: TransactionDateRangePickerProps) => {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const [selectingEnd, setSelectingEnd] = useState(false);
  const [viewMonth, setViewMonth] = useState(startOfMonth(value.to));
  const pickerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;

    const handlePointerDown = (event: PointerEvent) => {
      if (pickerRef.current?.contains(event.target as Node)) return;
      setOpen(false);
      setDraft(value);
      setSelectingEnd(false);
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setOpen(false);
      setDraft(value);
      setSelectingEnd(false);
    };

    document.addEventListener("pointerdown", handlePointerDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("pointerdown", handlePointerDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open, value]);

  const openPicker = () => {
    setDraft(value);
    setSelectingEnd(false);
    setViewMonth(startOfMonth(value.to));
    setOpen(true);
  };

  const selectDay = (day: string) => {
    if (!selectingEnd) {
      setDraft({ from: day, to: "" });
      setSelectingEnd(true);
      return;
    }

    if (day < draft.from) {
      setDraft({ from: day, to: "" });
      return;
    }

    setDraft({ from: draft.from, to: day });
    setSelectingEnd(false);
  };

  const month = startOfMonth(viewMonth);
  const previousMonth = addMonths(month, -1);
  const nextMonth = addMonths(month, 1);
  const canGoPrevious = previousMonth >= startOfMonth(minDate);
  const canGoNext = nextMonth <= startOfMonth(maxDate);
  const days = calendarDays(month);
  const hasCompleteRange = Boolean(draft.from && draft.to);

  return (
    <div ref={pickerRef} className="relative space-y-2">
      <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-2">
        <div className="space-y-1.5">
          <p className="px-1 text-xs font-semibold text-on-surface-variant">
            Từ ngày
          </p>
          <button
            type="button"
            onClick={openPicker}
            aria-label="Ngày bắt đầu"
            className="flex min-h-12 w-full items-center justify-between gap-2 rounded-2xl border border-outline-variant bg-surface-container-lowest px-3 text-left text-sm font-semibold text-on-surface transition-colors hover:border-secondary focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
          >
            <span>{formatDateValue(value.from)}</span>
            <FontAwesomeIcon
              icon={faCalendarDays}
              className="h-4 w-4 shrink-0 text-on-surface-variant"
            />
          </button>
        </div>
        <span
          aria-hidden="true"
          className="mb-3 text-sm font-semibold text-on-surface-variant"
        >
          →
        </span>
        <div className="space-y-1.5">
          <p className="px-1 text-xs font-semibold text-on-surface-variant">
            Đến ngày
          </p>
          <button
            type="button"
            onClick={openPicker}
            aria-label="Ngày kết thúc"
            className="flex min-h-12 w-full items-center justify-between gap-2 rounded-2xl border border-outline-variant bg-surface-container-lowest px-3 text-left text-sm font-semibold text-on-surface transition-colors hover:border-secondary focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
          >
            <span>{formatDateValue(value.to)}</span>
            <FontAwesomeIcon
              icon={faCalendarDays}
              className="h-4 w-4 shrink-0 text-on-surface-variant"
            />
          </button>
        </div>
      </div>

      {open && (
        <div
          role="dialog"
          aria-label="Chọn khoảng thời gian giao dịch"
          className="absolute inset-x-0 z-30 mt-2 rounded-3xl border border-outline-variant bg-surface-container-lowest p-5 shadow-xl shadow-inverse-surface/15"
        >
          <div className="flex items-start justify-between gap-3">
            <div>
              <p className="text-sm font-bold text-on-surface">
                Chọn khoảng thời gian
              </p>
              <p className="mt-1 text-xs text-on-surface-variant">
                {selectingEnd ? "Chọn ngày kết thúc" : "Chọn ngày bắt đầu"}
              </p>
            </div>
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                setDraft(value);
                setSelectingEnd(false);
              }}
              aria-label="Đóng bộ lọc ngày"
              className="flex h-9 w-9 items-center justify-center rounded-full text-on-surface-variant transition-colors hover:bg-secondary/10 hover:text-on-surface focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
            >
              <FontAwesomeIcon icon={faXmark} className="h-4 w-4" />
            </button>
          </div>

          <div className="mt-4 flex items-center justify-between">
            <button
              type="button"
              onClick={() => setViewMonth(previousMonth)}
              disabled={!canGoPrevious}
              aria-label="Tháng trước"
              className="flex h-9 w-9 items-center justify-center rounded-full text-on-surface-variant transition-colors hover:bg-secondary/10 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <FontAwesomeIcon icon={faChevronLeft} className="h-3.5 w-3.5" />
            </button>
            <p className="text-sm font-bold capitalize text-on-surface">
              {formatMonthValue(month)}
            </p>
            <button
              type="button"
              onClick={() => setViewMonth(nextMonth)}
              disabled={!canGoNext}
              aria-label="Tháng sau"
              className="flex h-9 w-9 items-center justify-center rounded-full text-on-surface-variant transition-colors hover:bg-secondary/10 disabled:cursor-not-allowed disabled:opacity-40"
            >
              <FontAwesomeIcon
                icon={faChevronRight}
                className="h-3.5 w-3.5"
              />
            </button>
          </div>

          <div className="mt-3 grid grid-cols-7 text-center text-xs font-semibold text-on-surface-variant">
            {WEEKDAYS.map((weekday) => (
              <span key={weekday} className="py-2">
                {weekday}
              </span>
            ))}
          </div>

          <div className="grid grid-cols-7 gap-y-1 text-center">
            {days.map((day, index) => {
              if (!day) return <span key={`empty-${index}`} className="h-10" />;

              const disabled = day < minDate || day > maxDate;
              const isStart = day === draft.from;
              const isEnd = day === draft.to;
              const inRange =
                hasCompleteRange && isDateInRange(day, draft);
              const today = day === maxDate;

              return (
                <div
                  key={day}
                  className="relative flex h-10 items-center justify-center"
                >
                  {inRange && (
                    <span
                      aria-hidden="true"
                      className={`absolute inset-y-1 inset-x-0 bg-secondary/10 ${
                        isStart ? "rounded-l-full" : ""
                      } ${isEnd ? "rounded-r-full" : ""}`}
                    />
                  )}
                  <button
                    type="button"
                    disabled={disabled}
                    aria-label={formatDateValue(day)}
                    aria-pressed={isStart || isEnd}
                    onClick={() => selectDay(day)}
                    className={`relative flex h-9 w-9 items-center justify-center rounded-full text-sm transition-colors focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none ${
                      isStart || isEnd
                        ? "bg-secondary font-bold text-on-secondary shadow-sm shadow-secondary/30"
                        : inRange
                          ? "text-secondary-strong hover:bg-secondary/15"
                          : "text-on-surface hover:bg-secondary/10"
                    } ${
                      today && !isStart && !isEnd
                        ? "ring-1 ring-secondary"
                        : ""
                    } disabled:cursor-not-allowed disabled:text-on-surface-variant/35`}
                  >
                    {dateValueToDate(day).getUTCDate()}
                  </button>
                </div>
              );
            })}
          </div>

          <div className="mt-4 flex justify-end border-t border-outline-variant pt-3">
            <button
              type="button"
              disabled={!hasCompleteRange}
              onClick={() => {
                if (!hasCompleteRange) return;
                onConfirm(draft);
                setOpen(false);
              }}
              className="rounded-xl bg-secondary px-4 py-2.5 text-sm font-bold text-on-secondary transition-colors hover:bg-secondary-strong disabled:cursor-not-allowed disabled:opacity-40"
            >
              Xác nhận
            </button>
          </div>
        </div>
      )}
    </div>
  );
};
