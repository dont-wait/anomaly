import { useState, type FormEvent, type ReactNode } from "react";
import { BookmarkPlusIcon, ContactsIcon } from "@/shared/icons";
import { PageHeader } from "@/shared/layout";
import { BottomSheet, Button } from "@/shared/ui";
import { formatVnd, maskAccountNo } from "@/features/transactions/utils/format";
import {
  NOTE_MAX_LENGTH,
  type Recipient,
  type SourceAccount,
} from "@/features/transfer/model";
import { contactStore } from "@/features/transfer/api/transfer";
import { useBanks } from "@/features/transfer/useBanks";
import { useTransferFlow } from "@/features/transfer/useTransferFlow";
import { suggestAmounts } from "@/features/transfer/utils/amountSuggestions";
import { BankSelect } from "./components/BankSelect";
import { ConfirmSheetContent } from "./components/ConfirmSheetContent";
import { ContactsSheetContent } from "./components/ContactsSheetContent";
import { FailureSheetContent } from "./components/FailureSheetContent";
import { RecentRecipients } from "./components/RecentRecipients";
import { SuccessScreen } from "./components/SuccessScreen";
const MAX_AMOUNT_DIGITS = 12;
const numberFormat = new Intl.NumberFormat("vi-VN");

const Card = ({ title, children }: { title: string; children: ReactNode }) => (
  <section
    aria-label={title}
    className="space-y-4 rounded-3xl bg-surface-container-lowest p-4 shadow-sm shadow-secondary/10"
  >
    <h2 className="text-base font-bold text-on-surface">{title}</h2>
    {children}
  </section>
);

const inputClass = (invalid: boolean) =>
  `mt-1 w-full rounded-2xl border bg-surface-container-lowest px-4 text-on-surface placeholder:text-on-surface-variant/60 focus:ring-2 focus:outline-none ${
    invalid
      ? "border-error focus:ring-error/20"
      : "border-outline-variant focus:border-secondary focus:ring-secondary/20"
  }`;

interface TransferFlowProps {
  source: SourceAccount;
  onExit: () => void;
  onViewHistory: () => void;
}

/** Màn chuyển tiền một trang; xác thực OTP trong sheet trượt lên, thành công hiện toàn màn. */
export function TransferFlow({
  source,
  onExit,
  onViewHistory,
}: TransferFlowProps) {
  const flow = useTransferFlow(source);
  const banks = useBanks();
  const { lookup, amountError } = flow;
  const accountError = lookup.status === "error" ? lookup.message : "";
  const [contactsOpen, setContactsOpen] = useState(false);
  const [contacts, setContacts] = useState(() =>
    contactStore.list(source.accountNo),
  );
  const isSaved =
    flow.recipient !== null &&
    contacts.some(
      (contact) =>
        contact.accountNo === flow.recipient?.accountNo &&
        contact.bank.code === flow.recipient.bank.code,
    );
  const amountSuggestions = suggestAmounts(flow.amount, flow.source.balance);

  const chooseContact = (contact: Recipient) => {
    flow.chooseRecipient(contact);
    setContactsOpen(false);
    setContacts(contactStore.list(source.accountNo));
  };

  const saveContact = () => {
    if (!flow.recipient) return;
    contactStore.add(flow.recipient, source.accountNo);
    setContacts(contactStore.list(source.accountNo));
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    flow.openConfirm();
  };

  const sheetTitle =
    flow.sheet === "result" ? "Giao dịch chưa hoàn tất" : "Xác thực giao dịch";

  if (flow.result?.ok) {
    return (
      <SuccessScreen
        record={flow.result.record}
        recipient={flow.recipient}
        onHome={onExit}
        onNewTransfer={flow.reset}
        onViewHistory={onViewHistory}
      />
    );
  }

  return (
    <>
      <PageHeader
        title="Chuyển tiền"
        onBack={onExit}
        backLabel="Về trang chủ"
      />

      <form
        onSubmit={onSubmit}
        noValidate
        className="flex min-h-0 flex-1 flex-col"
      >
        <main className="flex-1 space-y-4 overflow-y-auto px-4 pt-4 pb-6">
          <section
            aria-label="Tài khoản nguồn"
            className="flex items-center justify-between gap-3 rounded-3xl bg-surface-container-lowest px-4 py-3 shadow-sm shadow-secondary/10"
          >
            <div className="min-w-0">
              <p className="text-xs text-on-surface-variant">Từ tài khoản</p>
              <p className="truncate text-sm font-semibold text-on-surface">
                {maskAccountNo(flow.source.accountNo)}
              </p>
            </div>
            <div className="text-right">
              <p className="text-xs text-on-surface-variant">Số dư khả dụng</p>
              <p className="text-base font-bold text-secondary-strong tabular-nums">
                {formatVnd(flow.source.balance)}
              </p>
            </div>
          </section>

          <Card title="Người nhận">
            <RecentRecipients
              selected={flow.recipient}
              onSelect={flow.chooseRecipient}
            />

            <BankSelect
              banks={banks.banks}
              status={banks.status}
              onRetry={banks.retry}
              value={flow.bank}
              onChange={flow.setBank}
            />

            <div>
              <label
                htmlFor="recipient-account"
                className="text-sm font-semibold text-on-surface-variant"
              >
                Số tài khoản
              </label>
              <div className="relative">
                <input
                  id="recipient-account"
                  inputMode="numeric"
                  autoComplete="off"
                  value={flow.accountNo}
                  onChange={(e) =>
                    flow.setAccountNo(e.target.value.replace(/[^\d\s]/g, ""))
                  }
                  onBlur={flow.commitAccountNo}
                  placeholder="Nhập số tài khoản"
                  aria-invalid={Boolean(accountError)}
                  aria-describedby="recipient-status"
                  className={`${inputClass(Boolean(accountError))} h-14 pr-14 text-lg font-semibold tracking-wide tabular-nums placeholder:text-base placeholder:font-normal placeholder:tracking-normal`}
                />
                <button
                  type="button"
                  onClick={() => setContactsOpen(true)}
                  aria-label="Mở danh bạ người nhận"
                  className="absolute top-1/2 right-1.5 mt-0.5 flex h-11 w-11 -translate-y-1/2 items-center justify-center rounded-xl text-secondary-strong transition-colors hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
                >
                  <ContactsIcon aria-hidden="true" className="h-6 w-6" />
                </button>
              </div>
              <div id="recipient-status" aria-live="polite">
                <span className="sr-only">
                  {lookup.status === "loading"
                    ? "Đang kiểm tra tài khoản"
                    : lookup.status === "found"
                      ? `Người nhận: ${lookup.recipient.name}`
                      : ""}
                </span>
                {accountError && (
                  <p
                    role="alert"
                    className="mt-2 text-sm font-medium text-error"
                  >
                    {accountError}
                  </p>
                )}
              </div>
            </div>

            <div>
              <label
                htmlFor="recipient-name"
                className="text-sm font-semibold text-on-surface-variant"
              >
                Tên người nhận
              </label>
              <div className="relative">
                <input
                  id="recipient-name"
                  readOnly
                  tabIndex={-1}
                  value={lookup.status === "found" ? lookup.recipient.name : ""}
                  className={`${inputClass(false)} h-14 pr-12 text-base font-semibold tracking-wide`}
                />
                {lookup.status === "loading" && (
                  <span
                    aria-hidden="true"
                    className="absolute top-1/2 right-4 mt-0.5 h-5 w-5 -translate-y-1/2 animate-spin rounded-full border-2 border-secondary/30 border-t-secondary motion-reduce:animate-none"
                  />
                )}
              </div>

              {lookup.status === "found" &&
                (isSaved ? (
                  <p className="mt-2 text-xs text-on-surface-variant">
                    Đã có trong danh bạ
                  </p>
                ) : (
                  <button
                    type="button"
                    onClick={saveContact}
                    className="mt-2 -ml-2 inline-flex min-h-11 items-center gap-1.5 rounded-full px-2 text-sm font-semibold text-secondary-strong transition-colors hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
                  >
                    <BookmarkPlusIcon aria-hidden="true" className="h-4 w-4" />
                    Lưu vào danh bạ
                  </button>
                ))}
            </div>
          </Card>

          <Card title="Số tiền">
            <div>
              <label htmlFor="transfer-amount" className="sr-only">
                Số tiền chuyển
              </label>
              <div
                className={`flex items-baseline gap-1 border-b-2 pb-2 transition-colors ${
                  amountError
                    ? "border-error"
                    : "border-outline-variant focus-within:border-secondary"
                }`}
              >
                <input
                  id="transfer-amount"
                  inputMode="numeric"
                  autoComplete="off"
                  value={flow.amount ? numberFormat.format(flow.amount) : ""}
                  onChange={(e) =>
                    flow.setAmount(
                      Number(
                        e.target.value
                          .replace(/\D/g, "")
                          .slice(0, MAX_AMOUNT_DIGITS),
                      ),
                    )
                  }
                  placeholder="0"
                  aria-invalid={Boolean(amountError)}
                  aria-describedby={amountError ? "amount-error" : undefined}
                  className="w-full min-w-0 bg-transparent text-3xl font-bold tracking-tight text-on-surface tabular-nums placeholder:text-on-surface-variant/40 focus:outline-none"
                />
                <span
                  aria-hidden="true"
                  className="text-2xl font-bold text-on-surface-variant"
                >
                  ₫
                </span>
              </div>
              {amountError && (
                <p
                  id="amount-error"
                  role="alert"
                  className="mt-2 text-sm font-medium text-error"
                >
                  {amountError}
                </p>
              )}
            </div>

            {amountSuggestions.length > 0 && (
              <div
                role="group"
                aria-label="Gợi ý số tiền"
                className="flex flex-wrap gap-2"
              >
                {amountSuggestions.map((value) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => flow.setAmount(value)}
                    className="min-h-11 rounded-full bg-surface-container-lowest px-3.5 text-sm font-medium text-secondary-strong tabular-nums ring-1 ring-secondary/30 transition-colors hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
                  >
                    {numberFormat.format(value)}
                  </button>
                ))}
              </div>
            )}

            <div>
              <div className="flex items-baseline justify-between">
                <label
                  htmlFor="transfer-note"
                  className="text-sm font-semibold text-on-surface-variant"
                >
                  Lời nhắn
                </label>
                <span className="text-xs text-on-surface-variant tabular-nums">
                  {flow.note.length}/{NOTE_MAX_LENGTH}
                </span>
              </div>
              <textarea
                id="transfer-note"
                rows={2}
                maxLength={NOTE_MAX_LENGTH}
                value={flow.note}
                onChange={(e) => flow.setNote(e.target.value)}
                className={`${inputClass(false)} resize-none py-3 text-base`}
              />
            </div>
          </Card>
        </main>

        <div className="border-t border-outline-variant/60 bg-surface-container-lowest/95 px-4 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] backdrop-blur">
          <div className="mb-2 flex items-center justify-between text-sm">
            <span className="text-on-surface-variant">
              Tổng tiền · Miễn phí chuyển
            </span>
            <span className="font-bold text-on-surface tabular-nums">
              {formatVnd(flow.amount)}
            </span>
          </div>
          <Button type="submit" className="w-full" disabled={!flow.canSubmit}>
            Chuyển tiền
          </Button>
        </div>
      </form>

      <BottomSheet
        open={flow.sheet !== "closed"}
        title={sheetTitle}
        onClose={flow.closeSheet}
        dismissible={!flow.busy}
      >
        {flow.sheet === "confirm" && <ConfirmSheetContent {...flow} />}
        {flow.sheet === "result" && <FailureSheetContent {...flow} />}
      </BottomSheet>

      <BottomSheet
        open={contactsOpen}
        title="Danh bạ người nhận"
        onClose={() => {
          setContactsOpen(false);
          setContacts(contactStore.list(source.accountNo));
        }}
      >
        <ContactsSheetContent
          ownerAccountNo={source.accountNo}
          onSelect={chooseContact}
        />
      </BottomSheet>
    </>
  );
}
