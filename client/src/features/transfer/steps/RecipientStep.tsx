import { useState, type FormEvent, type Ref } from "react";
import { ChevronRightIcon } from "@/shared/icons";
import { recentRecipients } from "@/features/transfer/api/transfer";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";
import { FieldError, RecipientCard, StepTitle } from "./primitives";
import { Button } from "@/shared/ui";

type Props = Pick<
  TransferFlowState,
  "error" | "busy" | "findRecipient" | "chooseRecipient"
> & {
  headingRef: Ref<HTMLHeadingElement>;
};

export function RecipientStep({
  error,
  busy,
  findRecipient,
  chooseRecipient,
  headingRef,
}: Props) {
  const [accountNo, setAccountNo] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void findRecipient(accountNo);
  };

  return (
    <div className="space-y-6">
      <StepTitle headingRef={headingRef} title="Chuyển đến ai?">
        Chuyển tiền miễn phí đến tài khoản AnomalyBank khác.
      </StepTitle>

      <form onSubmit={submit} noValidate className="space-y-4">
        <div>
          <label
            htmlFor="recipient-account"
            className="text-sm font-semibold text-on-surface-variant"
          >
            Số tài khoản người nhận
          </label>
          <input
            id="recipient-account"
            inputMode="numeric"
            autoComplete="off"
            value={accountNo}
            onChange={(e) =>
              setAccountNo(e.target.value.replace(/[^\d\s]/g, ""))
            }
            placeholder="Ví dụ: 99999180147"
            aria-invalid={Boolean(error)}
            aria-describedby={error ? "recipient-error" : undefined}
            className={`mt-1 h-13 w-full rounded-2xl border bg-surface-container-lowest px-4 text-lg font-semibold tracking-wide text-on-surface tabular-nums placeholder:font-normal placeholder:text-on-surface-variant/60 focus:ring-2 focus:outline-none ${
              error
                ? "border-error focus:ring-error/20"
                : "border-outline-variant focus:border-secondary focus:ring-secondary/20"
            }`}
          />
          <FieldError id="recipient-error" message={error} />
        </div>
        <Button
          type="submit"
          className="w-full"
          disabled={busy || !accountNo.trim()}
        >
          {busy ? "Đang kiểm tra..." : "Tiếp tục"}
        </Button>
      </form>

      <section aria-labelledby="recent-recipients">
        <h3
          id="recent-recipients"
          className="px-1 pb-2 text-xs font-semibold tracking-wide text-on-surface-variant uppercase"
        >
          Người nhận gần đây
        </h3>
        <ul className="space-y-2">
          {recentRecipients.map((recipient) => (
            <li key={recipient.accountNo}>
              <button
                type="button"
                onClick={() => chooseRecipient(recipient)}
                aria-label={`Chuyển đến ${recipient.name}`}
                className="block w-full rounded-2xl text-left transition-shadow hover:shadow-md hover:shadow-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
              >
                <RecipientCard
                  recipient={recipient}
                  action={
                    <ChevronRightIcon className="h-5 w-5 text-on-surface-variant" />
                  }
                />
              </button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
