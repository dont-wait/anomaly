import type { FormEvent, ReactNode } from "react";
import { CheckIcon } from "@/shared/icons";
import { PageHeader } from "@/shared/layout";
import { BottomSheet, Button } from "@/shared/ui";
import { formatVnd, maskAccountNo } from "@/features/transactions/utils/format";
import { NOTE_MAX_LENGTH, type SourceAccount } from "@/features/transfer/model";
import { useBanks } from "@/features/transfer/useBanks";
import { useTransferFlow } from "@/features/transfer/useTransferFlow";
import { BankSelect } from "./components/BankSelect";
import { ConfirmSheetContent } from "./components/ConfirmSheetContent";
import { RecentRecipients } from "./components/RecentRecipients";
import { ResultSheetContent } from "./components/ResultSheetContent";

const QUICK_AMOUNTS = [100_000, 200_000, 500_000, 1_000_000, 2_000_000];
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

/** Màn chuyển tiền một trang; xác thực OTP và kết quả nằm trong sheet trượt lên. */
export function TransferFlow({
  source,
  onExit,
  onViewHistory,
}: TransferFlowProps) {
  const flow = useTransferFlow(source);
  const banks = useBanks();
  const { lookup, amountError } = flow;
  const accountError = lookup.status === "error" ? lookup.message : "";

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    flow.openConfirm();
  };

  const sheetTitle =
    flow.sheet === "result"
      ? flow.result?.ok
        ? "Biên lai giao dịch"
        : "Giao dịch chưa hoàn tất"
      : "Xác thực giao dịch";

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
                  className={`${inputClass(Boolean(accountError))} h-14 pr-12 text-lg font-semibold tracking-wide tabular-nums placeholder:text-base placeholder:font-normal placeholder:tracking-normal`}
                />
                {lookup.status === "loading" && (
                  <span
                    aria-hidden="true"
                    className="absolute top-1/2 right-4 mt-0.5 h-5 w-5 -translate-y-1/2 animate-spin rounded-full border-2 border-secondary/30 border-t-secondary motion-reduce:animate-none"
                  />
                )}
              </div>
              <div id="recipient-status" aria-live="polite" className="min-h-6">
                {lookup.status === "loading" && (
                  <p className="mt-2 text-sm text-on-surface-variant">
                    Đang kiểm tra tài khoản…
                  </p>
                )}
                {lookup.status === "found" && (
                  <p className="mt-2 flex items-center gap-2 rounded-xl bg-success-container px-3 py-2 text-sm font-semibold text-on-success-container">
                    <CheckIcon
                      aria-hidden="true"
                      className="h-4 w-4 shrink-0"
                    />
                    <span className="truncate">{lookup.recipient.name}</span>
                  </p>
                )}
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

            <div
              role="group"
              aria-label="Chọn nhanh số tiền"
              className="flex flex-wrap gap-2"
            >
              {QUICK_AMOUNTS.map((value) => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={flow.amount === value}
                  onClick={() => flow.setAmount(value)}
                  className={`min-h-11 rounded-full px-3.5 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none ${
                    flow.amount === value
                      ? "bg-secondary-strong text-on-secondary"
                      : "bg-surface-container-lowest text-secondary-strong ring-1 ring-secondary/30 hover:bg-secondary/10"
                  }`}
                >
                  {numberFormat.format(value)}
                </button>
              ))}
            </div>

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
        {flow.sheet === "result" && (
          <ResultSheetContent
            {...flow}
            onHome={onExit}
            onViewHistory={onViewHistory}
          />
        )}
      </BottomSheet>
    </>
  );
}
