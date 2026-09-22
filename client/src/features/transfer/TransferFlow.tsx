import { useEffect, useRef } from "react";
import { PageHeader } from "@/shared/layout";
import type { SourceAccount, TransferStep } from "@/features/transfer/model";
import { useTransferFlow } from "@/features/transfer/useTransferFlow";
import { AmountStep } from "./steps/AmountStep";
import { ConfirmStep } from "./steps/ConfirmStep";
import { OtpStep } from "./steps/OtpStep";
import { RecipientStep } from "./steps/RecipientStep";
import { ResultStep } from "./steps/ResultStep";

const PROGRESS: Partial<Record<TransferStep, number>> = {
  recipient: 1,
  amount: 2,
  confirm: 3,
  otp: 3,
};
const TOTAL_STEPS = 3;

interface TransferFlowProps {
  source: SourceAccount;
  onExit: () => void;
  onViewHistory: () => void;
}

export function TransferFlow({
  source,
  onExit,
  onViewHistory,
}: TransferFlowProps) {
  const flow = useTransferFlow(source);
  const heading = useRef<HTMLHeadingElement>(null);
  const isFirstRender = useRef(true);
  const progress = PROGRESS[flow.step];

  // Chuyển focus về tiêu đề mỗi khi đổi bước để trình đọc màn hình đọc bước mới.
  useEffect(() => {
    if (isFirstRender.current) {
      isFirstRender.current = false;
      return;
    }
    heading.current?.focus();
  }, [flow.step]);

  return (
    <>
      <PageHeader
        title="Chuyển tiền"
        onBack={
          flow.step === "result"
            ? undefined
            : flow.canGoBack
              ? flow.back
              : onExit
        }
        backLabel={flow.canGoBack ? "Quay lại bước trước" : "Về trang chủ"}
      />

      {progress && (
        <div className="bg-surface-container-lowest px-4 pb-3">
          <p className="text-xs font-medium text-on-surface-variant">
            Bước {progress}/{TOTAL_STEPS}
          </p>
          <div
            role="progressbar"
            aria-label="Tiến trình chuyển tiền"
            aria-valuemin={1}
            aria-valuemax={TOTAL_STEPS}
            aria-valuenow={progress}
            className="mt-1.5 flex gap-1.5"
          >
            {Array.from({ length: TOTAL_STEPS }, (_, i) => (
              <span
                key={i}
                className={`h-1 flex-1 rounded-full transition-colors duration-300 ${
                  i < progress
                    ? "bg-gradient-to-r from-cta-from to-cta-to"
                    : "bg-surface-container-highest"
                }`}
              />
            ))}
          </div>
        </div>
      )}

      <main className="flex-1 overflow-y-auto px-4 py-6">
        {flow.step === "recipient" && (
          <RecipientStep {...flow} headingRef={heading} />
        )}
        {flow.step === "amount" && (
          <AmountStep {...flow} headingRef={heading} />
        )}
        {flow.step === "confirm" && (
          <ConfirmStep {...flow} headingRef={heading} />
        )}
        {flow.step === "otp" && <OtpStep {...flow} headingRef={heading} />}
        {flow.step === "result" && (
          <ResultStep
            {...flow}
            headingRef={heading}
            onHome={onExit}
            onViewHistory={onViewHistory}
          />
        )}
      </main>
    </>
  );
}
