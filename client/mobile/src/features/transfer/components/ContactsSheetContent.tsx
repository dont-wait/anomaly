import { useMemo, useState } from "react";
import { ChevronRightIcon, SearchIcon, TrashIcon } from "@/shared/icons";
import { contactStore } from "@/features/transfer/api/transfer";
import type { Recipient } from "@/features/transfer/model";
import { BankLogo } from "./BankLogo";

const contactKey = (contact: Recipient) =>
  `${contact.bank.code}-${contact.accountNo}`;

interface ContactsSheetContentProps {
  onSelect: (recipient: Recipient) => void;
  /** STK chủ sở hữu để cô lập danh bạ theo account. */
  ownerAccountNo?: string;
}

/** Danh bạ người nhận đã lưu: tìm kiếm, chọn để chuyển, xoá có bước xác nhận. */
export function ContactsSheetContent({
  onSelect,
  ownerAccountNo,
}: ContactsSheetContentProps) {
  const [query, setQuery] = useState("");
  const [contacts, setContacts] = useState(() =>
    contactStore.list(ownerAccountNo),
  );
  const [confirmingKey, setConfirmingKey] = useState<string | null>(null);
  const [removedName, setRemovedName] = useState("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return contacts;
    return contacts.filter((contact) =>
      `${contact.name} ${contact.accountNo} ${contact.bank.shortName}`
        .toLowerCase()
        .includes(q),
    );
  }, [contacts, query]);

  const remove = (contact: Recipient) => {
    contactStore.remove(contact, ownerAccountNo);
    setContacts(contactStore.list(ownerAccountNo));
    setConfirmingKey(null);
    setRemovedName(contact.name);
  };

  return (
    <div className="space-y-3">
      <label className="flex items-center gap-2 rounded-2xl bg-surface-container-low px-4 focus-within:ring-2 focus-within:ring-secondary/30">
        <SearchIcon
          aria-hidden="true"
          className="h-5 w-5 shrink-0 text-on-surface-variant"
        />
        <span className="sr-only">Tìm trong danh bạ</span>
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Tên, số tài khoản, ngân hàng"
          className="h-12 w-full bg-transparent text-base text-on-surface placeholder:text-on-surface-variant/70 focus:outline-none"
        />
      </label>

      <p role="status" className="sr-only">
        {removedName && `Đã xoá ${removedName} khỏi danh bạ`}
      </p>

      {filtered.length === 0 ? (
        <p className="py-8 text-center text-sm text-on-surface-variant">
          {contacts.length === 0
            ? "Danh bạ trống. Lưu người nhận sau khi nhập số tài khoản."
            : "Không có người nhận phù hợp."}
        </p>
      ) : (
        <ul aria-label="Danh bạ" className="-mx-2">
          {filtered.map((contact) => {
            const key = contactKey(contact);
            if (confirmingKey === key) {
              return (
                <li
                  key={key}
                  className="rounded-2xl bg-error-container/50 px-3 py-3"
                >
                  <p className="text-sm font-semibold text-on-error-container">
                    Xoá {contact.name} khỏi danh bạ?
                  </p>
                  <p className="mt-0.5 text-xs text-on-surface-variant">
                    {contact.accountNo} · {contact.bank.shortName}
                  </p>
                  <div className="mt-3 flex gap-2">
                    <button
                      type="button"
                      onClick={() => remove(contact)}
                      className="min-h-11 flex-1 rounded-full bg-error px-4 text-sm font-semibold text-on-error transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-error/50 focus-visible:outline-none"
                    >
                      Xoá
                    </button>
                    <button
                      type="button"
                      autoFocus
                      onClick={() => setConfirmingKey(null)}
                      className="min-h-11 flex-1 rounded-full bg-surface-container-lowest px-4 text-sm font-semibold text-on-surface ring-1 ring-outline-variant transition-colors hover:bg-surface-container focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
                    >
                      Huỷ
                    </button>
                  </div>
                </li>
              );
            }

            return (
              <li key={key} className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={() => onSelect(contact)}
                  aria-label={`${contact.name}, ${contact.accountNo}, ${contact.bank.shortName}`}
                  className="flex min-h-16 flex-1 items-center gap-3 rounded-2xl px-2 py-2 text-left transition-colors hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
                >
                  <BankLogo bank={contact.bank} size="sm" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-semibold text-on-surface">
                      {contact.name}
                    </span>
                    <span className="block truncate text-xs text-on-surface-variant">
                      {contact.accountNo} · {contact.bank.shortName}
                    </span>
                  </span>
                  <ChevronRightIcon
                    aria-hidden="true"
                    className="h-5 w-5 shrink-0 text-on-surface-variant"
                  />
                </button>
                <button
                  type="button"
                  onClick={() => setConfirmingKey(key)}
                  aria-label={`Xoá ${contact.name} khỏi danh bạ`}
                  className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-on-surface-variant transition-colors hover:bg-error-container hover:text-on-error-container focus-visible:ring-2 focus-visible:ring-error/40 focus-visible:outline-none"
                >
                  <TrashIcon aria-hidden="true" className="h-5 w-5" />
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
