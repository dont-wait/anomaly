import type { FormEvent, Ref } from "react";
import { formatVnd } from "@/features/transactions/utils/format";
import { NOTE_MAX_LENGTH } from "@/features/transfer/model";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";
import { FieldError, RecipientCard, StepTitle } from "./primitives";
import { Button } from "@/shared/ui";

const QUICK_AMOUNTS = [100_000, 200_000, 500_000, 1_000_000, 2_000_000];
const MAX_DIGITS = 12;
const numberFormat = new Intl.NumberFormat("vi-VN");

type Props = Pick<
  TransferFlowState,
  | "source"
  | "recipient"
  | "amount"
  | "setAmount"
  | "note"
  | "setNote"
  | "error"
  | "amountError"
  | "submitAmount"
  | "back"
> & { headingRef: Ref<HTMLHeadingElement> };

export function AmountStep({
  source,
  recipient,
  amount,
  setAmount,
  note,
  setNote,
  error,
  amountError,
  submitAmount,
  back,
  headingRef,
}: Props) {
  if (!recipient) return null;
  const message = amountError || error;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    submitAmount();
  };

  return (
    <form onSubmit={submit} noValidate className="space-y-6">
      <StepTitle headingRef={headingRef} title="Nhập số tiền" />

      <RecipientCard
        recipient={recipient}
        action={
          <button
            type="button"
            onClick={back}
            className="min-h-10 rounded-full px-3 text-sm font-semibold text-secondary-strong hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
          >
            Đổi
          </button>
        }
      />

      <div className="rounded-3xl bg-surface-container-lowest p-4">
        <label
          htmlFor="transfer-amount"
          className="text-sm font-semibold text-on-surface-variant"
        >
          Số tiền chuyển
        </label>
        <div
          className={`mt-2 flex items-baseline gap-1 border-b-2 pb-2 ${message ? "border-error" : "border-outline-variant focus-within:border-secondary"}`}
        >
          <input
            id="transfer-amount"
            inputMode="numeric"
            autoComplete="off"
            value={amount ? numberFormat.format(amount) : ""}
            onChange={(e) =>
              setAmount(
                Number(e.target.value.replace(/\D/g, "").slice(0, MAX_DIGITS)),
              )
            }
            placeholder="0"
            aria-invalid={Boolean(message)}
            aria-describedby={`transfer-balance${message ? " amount-error" : ""}`}
            className="w-full min-w-0 bg-transparent text-3xl font-bold tracking-tight text-on-surface tabular-nums placeholder:text-on-surface-variant/40 focus:outline-none"
          />
          <span
            aria-hidden="true"
            className="text-2xl font-bold text-on-surface-variant"
          >
            ₫
          </span>
        </div>
        <p
          id="transfer-balance"
          className="mt-2 text-xs text-on-surface-variant"
        >
          Số dư khả dụng:{" "}
          <span className="font-semibold text-on-surface">
            {formatVnd(source.balance)}
          </span>
        </p>
        <FieldError id="amount-error" message={message} />

        <div
          role="group"
          aria-label="Chọn nhanh số tiền"
          className="mt-4 flex flex-wrap gap-2"
        >
          {QUICK_AMOUNTS.map((value) => (
            <button
              key={value}
              type="button"
              aria-pressed={amount === value}
              onClick={() => setAmount(value)}
              className={`min-h-11 rounded-full px-3.5 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none ${
                amount === value
                  ? "bg-secondary-strong text-on-secondary shadow-md shadow-secondary/30"
                  : "bg-surface-container-lowest text-secondary-strong ring-1 ring-secondary/30 hover:bg-secondary/10"
              }`}
            >
              {numberFormat.format(value)}
            </button>
          ))}
        </div>
      </div>

      <div>
        <div className="flex items-baseline justify-between">
          <label
            htmlFor="transfer-note"
            className="text-sm font-semibold text-on-surface-variant"
          >
            Nội dung chuyển tiền
          </label>
          <span className="text-xs text-on-surface-variant tabular-nums">
            {note.length}/{NOTE_MAX_LENGTH}
          </span>
        </div>
        <textarea
          id="transfer-note"
          rows={2}
          maxLength={NOTE_MAX_LENGTH}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          className="mt-1 w-full resize-none rounded-2xl border border-outline-variant bg-surface-container-lowest px-4 py-3 text-base text-on-surface focus:border-primary focus:ring-2 focus:ring-primary/20 focus:outline-none"
        />
      </div>

      <Button
        type="submit"
        className="w-full"
        disabled={!amount || Boolean(amountError)}
      >
        Tiếp tục
      </Button>
    </form>
  );
}
