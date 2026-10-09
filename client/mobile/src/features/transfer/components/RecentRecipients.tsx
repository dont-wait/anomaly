import { useEffect, useState } from "react";
import { Avatar } from "@/shared/ui";
import { listRecentRecipients } from "@/features/transfer/api/transfer";
import type { Recipient } from "@/features/transfer/model";
import { ApiError } from "@/shared/lib/http";

interface RecentRecipientsProps {
  token: string;
  selected: Recipient | null;
  onSelect: (recipient: Recipient) => void;
  onUnauthorized?: () => void;
}

const isSame = (a: Recipient | null, b: Recipient) =>
  a?.accountNo === b.accountNo && a.bank.code === b.bank.code;

export function RecentRecipients({
  token,
  selected,
  onSelect,
  onUnauthorized,
}: RecentRecipientsProps) {
  const [recipients, setRecipients] = useState<Recipient[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading",
  );
  const [requestId, setRequestId] = useState(0);

  useEffect(() => {
    let active = true;
    void listRecentRecipients(token)
      .then((items) => {
        if (!active) return;
        setRecipients(items);
        setStatus("ready");
      })
      .catch((error: unknown) => {
        if (error instanceof ApiError && error.status === 401) {
          onUnauthorized?.();
          return;
        }
        if (active) setStatus("error");
      });
    return () => {
      active = false;
    };
  }, [token, requestId, onUnauthorized]);

  return (
    <section aria-labelledby="recent-recipients-title">
      <div className="flex items-center justify-between gap-3">
        <h3
          id="recent-recipients-title"
          className="text-xs font-semibold tracking-wide text-on-surface-variant uppercase"
        >
          Gần đây
        </h3>
        {status === "error" && (
          <button
            type="button"
            onClick={() => {
              setStatus("loading");
              setRequestId((value) => value + 1);
            }}
            className="min-h-11 rounded-full px-3 text-xs font-semibold text-secondary-strong"
          >
            Thử tải lại
          </button>
        )}
      </div>

      {status === "loading" && (
        <p role="status" className="mt-2 text-xs text-on-surface-variant">
          Đang tải người nhận gần đây...
        </p>
      )}
      {status === "error" && (
        <p role="alert" className="mt-1 text-xs text-error">
          Không tải được người nhận gần đây.
        </p>
      )}
      {status === "ready" && recipients.length === 0 && (
        <p className="mt-2 text-xs text-on-surface-variant">
          Chưa có người nhận gần đây.
        </p>
      )}
      {recipients.length > 0 && (
        <ul className="-mx-4 mt-2 flex snap-x gap-2 overflow-x-auto px-4 pb-1 [scrollbar-width:none]">
          {recipients.map((recipient) => {
            const active = isSame(selected, recipient);
            const shortName = recipient.name.split(" ").slice(-1)[0];
            return (
              <li key={recipient.accountNo} className="snap-start">
                <button
                  type="button"
                  aria-pressed={active}
                  aria-label={`Chuyển đến ${recipient.name}, AnomalyBank`}
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
                    AnomalyBank
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
