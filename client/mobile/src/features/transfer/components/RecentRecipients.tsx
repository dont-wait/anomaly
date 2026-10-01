import { Avatar } from "@/shared/ui";
import { recentRecipients } from "@/features/transfer/api/transfer";
import type { Recipient } from "@/features/transfer/model";

interface RecentRecipientsProps {
  selected: Recipient | null;
  onSelect: (recipient: Recipient) => void;
}

const isSame = (a: Recipient | null, b: Recipient) =>
  a?.accountNo === b.accountNo && a.bank.code === b.bank.code;

/** Hàng cuộn ngang các người nhận gần đây — chạm để điền sẵn ngân hàng + STK. */
export function RecentRecipients({
  selected,
  onSelect,
}: RecentRecipientsProps) {
  return (
    <section aria-labelledby="recent-recipients-title">
      <h3
        id="recent-recipients-title"
        className="text-xs font-semibold tracking-wide text-on-surface-variant uppercase"
      >
        Gần đây
      </h3>
      <ul className="-mx-4 mt-2 flex snap-x gap-2 overflow-x-auto px-4 pb-1 [scrollbar-width:none]">
        {recentRecipients.map((recipient) => {
          const active = isSame(selected, recipient);
          const shortName = recipient.name.split(" ").slice(-1)[0];
          return (
            <li
              key={`${recipient.bank.code}-${recipient.accountNo}`}
              className="snap-start"
            >
              <button
                type="button"
                aria-pressed={active}
                aria-label={`Chuyển đến ${recipient.name}, ${recipient.bank.shortName}`}
                onClick={() => onSelect(recipient)}
                className={`flex w-20 flex-col items-center gap-1.5 rounded-2xl px-1 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none ${
                  active ? "bg-secondary/15" : "hover:bg-secondary/10"
                }`}
              >
                <Avatar name={recipient.name} size={44} />
                <span className="w-full truncate text-center text-xs font-semibold text-on-surface">
                  {shortName}
                </span>
                <span className="w-full truncate text-center text-[11px] text-on-surface-variant">
                  {recipient.bank.shortName}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
