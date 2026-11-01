import { useState } from "react";
import { faCheck, faChevronDown } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { BottomSheet } from "./BottomSheet";

export interface SelectSheetOption {
  value: string;
  label: string;
  description?: string;
  disabled?: boolean;
}

interface SelectSheetProps {
  id?: string;
  label: string;
  value: string;
  options: SelectSheetOption[];
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
}

/** Mobile friendly select rendered as a labeled trigger and a selectable bottom sheet. */
export function SelectSheet({
  id,
  label,
  value,
  options,
  onChange,
  disabled = false,
  placeholder = "Chọn một mục",
}: SelectSheetProps) {
  const [open, setOpen] = useState(false);
  const selected = options.find((option) => option.value === value);

  return (
    <>
      <button
        id={id}
        type="button"
        disabled={disabled}
        aria-haspopup="dialog"
        aria-label={`${label}: ${selected?.label ?? placeholder}`}
        onClick={() => setOpen(true)}
        className="flex min-h-12 w-full items-center justify-between gap-3 rounded-2xl border border-[#e2e7ff] bg-white px-4 py-3 text-left shadow-sm transition-colors hover:bg-[#faf8ff] disabled:cursor-not-allowed disabled:opacity-50"
      >
        <span className="min-w-0">
          <span className="block truncate text-sm font-semibold text-[#00236f]">
            {selected?.label ?? placeholder}
          </span>
          {selected?.description && (
            <span className="mt-0.5 block text-xs text-[#646b80]">
              {selected.description}
            </span>
          )}
        </span>
        <FontAwesomeIcon
          icon={faChevronDown}
          className="h-5 w-5 shrink-0 text-[#6b38d4]"
        />
      </button>
      <BottomSheet open={open} title={label} onClose={() => setOpen(false)}>
        <div className="flex flex-col gap-2">
          {options.map((option) => {
            const active = option.value === value;
            return (
              <button
                key={option.value}
                type="button"
                disabled={option.disabled}
                aria-pressed={active}
                onClick={() => {
                  onChange(option.value);
                  setOpen(false);
                }}
                className={`flex min-h-14 items-center justify-between gap-3 rounded-2xl border px-4 py-3 text-left transition-colors disabled:opacity-50 ${
                  active
                    ? "border-[#b582b5] bg-[#f3eafa]"
                    : "border-[#e2e7ff] bg-white hover:bg-[#faf8ff]"
                }`}
              >
                <span>
                  <span className="block text-sm font-semibold text-[#00236f]">
                    {option.label}
                  </span>
                  {option.description && (
                    <span className="mt-1 block text-xs text-[#646b80]">
                      {option.description}
                    </span>
                  )}
                </span>
                {active && (
                  <FontAwesomeIcon
                    icon={faCheck}
                    className="h-5 w-5 shrink-0 text-[#6b38d4]"
                  />
                )}
              </button>
            );
          })}
        </div>
      </BottomSheet>
    </>
  );
}
