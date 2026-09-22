import { useMemo, useState } from "react";
import { SearchIcon } from "@/shared/icons";
import { TransactionRow } from "@/features/transactions/components";
import { formatVnd, groupByDay } from "@/features/transactions/utils/format";
import type {
  TransactionFilter,
  TransactionRecord,
} from "@/features/transactions/model";

const FILTERS: { id: TransactionFilter; label: string }[] = [
  { id: "all", label: "Tất cả" },
  { id: "in", label: "Tiền vào" },
  { id: "out", label: "Tiền ra" },
];

const normalize = (text: string) =>
  text.normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace(/đ/gi, "d").toLowerCase();

function matchesQuery(record: TransactionRecord, query: string) {
  if (!query) return true;
  const haystack = normalize(
    [
      record.counterparty.name,
      record.counterparty.accountNo ?? "",
      record.note,
      record.reference,
      String(record.amount),
    ].join(" "),
  );
  return haystack.includes(normalize(query.trim()));
}

interface TransactionHistoryProps {
  records: TransactionRecord[];
  onSelect: (record: TransactionRecord) => void;
  now?: Date;
}

export const TransactionHistory = ({
  records,
  onSelect,
  now: nowProp,
}: TransactionHistoryProps) => {
  const [now] = useState(() => nowProp ?? new Date());
  const [filter, setFilter] = useState<TransactionFilter>("all");
  const [query, setQuery] = useState("");

  const monthSummary = useMemo(() => {
    const inMonth = records.filter(
      (r) =>
        r.status === "success" &&
        r.createdAt.getMonth() === now.getMonth() &&
        r.createdAt.getFullYear() === now.getFullYear(),
    );
    const sum = (direction: TransactionFilter) =>
      inMonth
        .filter((r) => r.direction === direction)
        .reduce((total, r) => total + r.amount, 0);
    return { in: sum("in"), out: sum("out") };
  }, [records, now]);

  const groups = useMemo(
    () =>
      groupByDay(
        records.filter(
          (r) =>
            (filter === "all" || r.direction === filter) &&
            matchesQuery(r, query),
        ),
        now,
      ),
    [records, filter, query, now],
  );

  return (
    <div className="space-y-5">
      <section
        aria-label={`Tổng quan tháng ${now.getMonth() + 1}/${now.getFullYear()}`}
        className="relative overflow-hidden rounded-3xl bg-gradient-to-br from-cta-from to-cta-to p-5 text-on-cta shadow-lg shadow-cta-from/30"
      >
        <div className="pointer-events-none absolute -top-10 -right-8 h-32 w-32 rounded-full bg-on-cta/15 blur-2xl" />
        <p className="relative text-sm text-on-cta">
          Tháng {now.getMonth() + 1}/{now.getFullYear()}
        </p>
        <div className="relative mt-3 grid grid-cols-2 gap-3">
          <div className="rounded-2xl bg-on-cta/15 p-3">
            <p className="text-xs text-on-cta">Tiền vào</p>
            <p className="mt-1 font-bold tabular-nums">
              +{formatVnd(monthSummary.in)}
            </p>
          </div>
          <div className="rounded-2xl bg-on-cta/15 p-3">
            <p className="text-xs text-on-cta">Tiền ra</p>
            <p className="mt-1 font-bold tabular-nums">
              -{formatVnd(monthSummary.out)}
            </p>
          </div>
        </div>
      </section>

      <div className="space-y-3">
        <label className="flex items-center gap-2 rounded-2xl border border-outline-variant bg-surface-container-lowest px-4 focus-within:border-secondary focus-within:ring-2 focus-within:ring-primary/20">
          <SearchIcon className="h-5 w-5 shrink-0 text-on-surface-variant" />
          <span className="sr-only">Tìm kiếm giao dịch</span>
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Tên, nội dung, mã giao dịch..."
            className="h-12 w-full bg-transparent text-base text-on-surface placeholder:text-on-surface-variant/70 focus:outline-none"
          />
        </label>

        <div
          role="group"
          aria-label="Lọc giao dịch"
          className="flex flex-wrap gap-2"
        >
          {FILTERS.map((item) => {
            const active = filter === item.id;
            return (
              <button
                key={item.id}
                type="button"
                aria-pressed={active}
                onClick={() => setFilter(item.id)}
                className={`min-h-11 rounded-full px-4 text-sm font-semibold transition-colors focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none ${
                  active
                    ? "bg-secondary-strong text-on-secondary shadow-md shadow-secondary/30"
                    : "bg-surface-container-lowest text-secondary-strong ring-1 ring-secondary/30 hover:bg-secondary/10"
                }`}
              >
                {item.label}
              </button>
            );
          })}
        </div>
      </div>

      {groups.length === 0 ? (
        <div className="rounded-3xl bg-surface-container-lowest px-6 py-10 text-center">
          <p className="font-semibold text-on-surface">
            Không có giao dịch phù hợp
          </p>
          <p className="mt-1 text-sm text-on-surface-variant">
            Thử đổi từ khoá tìm kiếm hoặc bộ lọc.
          </p>
        </div>
      ) : (
        groups.map((group) => (
          <section key={group.key} aria-label={group.label}>
            <h2 className="px-1 pb-1 text-xs font-semibold tracking-wide text-on-surface-variant uppercase">
              {group.label}
            </h2>
            <div className="rounded-3xl bg-surface-container-lowest p-1">
              {group.items.map((record) => (
                <TransactionRow
                  key={record.id}
                  record={record}
                  onSelect={onSelect}
                />
              ))}
            </div>
          </section>
        ))
      )}
    </div>
  );
};
