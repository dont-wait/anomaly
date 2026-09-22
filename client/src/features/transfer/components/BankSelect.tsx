import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { CheckIcon, ChevronDownIcon, SearchIcon } from "@/shared/icons";
import type { Bank } from "@/features/transfer/model";
import type { BanksStatus } from "@/features/transfer/useBanks";
import { BankLogo } from "./BankLogo";

interface BankSelectProps {
  banks: Bank[];
  status: BanksStatus;
  onRetry: () => void;
  value: Bank;
  onChange: (bank: Bank) => void;
}

/** Dropdown chọn ngân hàng có ô tìm kiếm (combobox + listbox). */
export function BankSelect({
  banks,
  status,
  onRetry,
  value,
  onChange,
}: BankSelectProps) {
  const id = useId();
  const listId = `${id}-list`;
  const labelId = `${id}-label`;
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLUListElement>(null);

  const filtered = useMemo(() => {
    const q = query.trim().toLocaleLowerCase();
    if (!q) return banks;
    return banks.filter((bank) =>
      (`${bank.shortName} ${bank.name} ${bank.code}`).toLowerCase().includes(q),
    );
  }, [banks, query]);

  const close = (restoreFocus = true) => {
    setOpen(false);
    setQuery("");
    if (restoreFocus) trigger.current?.focus();
  };

  const openList = () => {
    setActive(
      Math.max(
        0,
        banks.findIndex((b) => b.code === value.code),
      ),
    );
    setOpen(true);
  };

  const select = (bank: Bank) => {
    onChange(bank);
    close();
  };

  // Chạm ra ngoài thì đóng dropdown.
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) close(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  });

  // Giữ mục đang chọn bằng bàn phím luôn nằm trong vùng nhìn thấy.
  useEffect(() => {
    if (!open) return;
    list.current
      ?.querySelector<HTMLElement>(`[data-index="${active}"]`)
      ?.scrollIntoView?.({ block: "nearest" });
  }, [open, active]);

  const onSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((i) => Math.min(i + 1, filtered.length - 1));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((i) => Math.max(i - 1, 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (filtered[active]) select(filtered[active]);
    } else if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      close();
    }
  };

  return (
    <div ref={root} className="relative">
      <span
        id={labelId}
        className="text-sm font-semibold text-on-surface-variant"
      >
        Ngân hàng
      </span>
      <button
        ref={trigger}
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-labelledby={`${labelId} ${id}-value`}
        onClick={() => (open ? close() : openList())}
        className={`mt-1 flex min-h-14 w-full items-center gap-3 rounded-2xl border bg-surface-container-lowest px-3 py-2 text-left transition-colors focus-visible:ring-2 focus-visible:ring-secondary/40 focus-visible:outline-none ${
          open
            ? "border-secondary"
            : "border-outline-variant hover:border-secondary/60"
        }`}
      >
        <BankLogo bank={value} size="sm" />
        <span id={`${id}-value`} className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold text-on-surface">
            {value.shortName}
          </span>
          <span className="block truncate text-xs text-on-surface-variant">
            {value.name}
          </span>
        </span>
        <ChevronDownIcon
          aria-hidden="true"
          className={`h-5 w-5 shrink-0 text-on-surface-variant transition-transform duration-200 motion-reduce:transition-none ${open ? "rotate-180" : ""}`}
        />
      </button>

      {open && (
        <div className="absolute inset-x-0 top-full z-30 mt-2 overflow-hidden rounded-2xl bg-surface-container-lowest shadow-xl ring-1 shadow-inverse-surface/15 ring-outline-variant/70">
          <div className="border-b border-outline-variant/60 p-2">
            <label className="flex items-center gap-2 rounded-xl bg-surface-container-low px-3">
              <SearchIcon
                aria-hidden="true"
                className="h-4 w-4 shrink-0 text-on-surface-variant"
              />
              <span className="sr-only">Tìm ngân hàng</span>
              <input
                autoFocus
                role="combobox"
                aria-expanded="true"
                aria-controls={listId}
                aria-autocomplete="list"
                aria-activedescendant={
                  filtered[active]
                    ? `${id}-opt-${filtered[active].code}`
                    : undefined
                }
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setActive(0);
                }}
                onKeyDown={onSearchKeyDown}
                placeholder="Tìm theo tên hoặc mã ngân hàng"
                className="h-11 w-full bg-transparent text-base text-on-surface placeholder:text-on-surface-variant/70 focus:outline-none"
              />
            </label>
          </div>

          <ul
            ref={list}
            id={listId}
            role="listbox"
            aria-label="Danh sách ngân hàng"
            className="max-h-72 overflow-y-auto overscroll-contain p-1"
          >
            {filtered.map((bank, index) => {
              const selected = bank.code === value.code;
              return (
                <li
                  key={bank.code}
                  id={`${id}-opt-${bank.code}`}
                  data-index={index}
                  role="option"
                  aria-selected={selected}
                  onPointerEnter={() => setActive(index)}
                  onClick={() => select(bank)}
                  className={`flex min-h-14 cursor-pointer items-center gap-3 rounded-xl px-2 py-1.5 ${
                    index === active ? "bg-secondary/10" : ""
                  }`}
                >
                  <BankLogo bank={bank} size="sm" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-semibold text-on-surface">
                      {bank.shortName}
                    </span>
                    <span className="block truncate text-xs text-on-surface-variant">
                      {bank.name}
                    </span>
                  </span>
                  {selected && (
                    <CheckIcon
                      aria-hidden="true"
                      className="h-5 w-5 shrink-0 text-secondary-strong"
                    />
                  )}
                </li>
              );
            })}
          </ul>

          {filtered.length === 0 && status !== "loading" && (
            <p className="px-4 py-5 text-center text-sm text-on-surface-variant">
              Không tìm thấy ngân hàng phù hợp.
            </p>
          )}
          {status === "loading" && (
            <p
              role="status"
              className="flex items-center justify-center gap-2 px-4 py-4 text-sm text-on-surface-variant"
            >
              <span
                aria-hidden="true"
                className="h-4 w-4 animate-spin rounded-full border-2 border-secondary/30 border-t-secondary motion-reduce:animate-none"
              />
              Đang tải danh sách ngân hàng…
            </p>
          )}
          {status === "error" && (
            <div
              role="alert"
              className="flex items-center justify-between gap-3 border-t border-outline-variant/60 px-4 py-3 text-sm text-on-surface-variant"
            >
              <span>Không tải được các ngân hàng khác.</span>
              <button
                type="button"
                onClick={onRetry}
                className="min-h-11 shrink-0 rounded-full px-3 font-semibold text-secondary-strong hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
              >
                Thử lại
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
