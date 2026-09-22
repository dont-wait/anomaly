import { useState, type FormEvent, type Ref } from "react";
import { DEMO_OTP } from "@/features/transfer/api/transfer";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";
import { FieldError, StepTitle } from "./primitives";
import { Button } from "@/shared/ui";

type Props = Pick<TransferFlowState, "error" | "busy" | "submitOtp"> & {
  headingRef: Ref<HTMLHeadingElement>;
};

export function OtpStep({ error, busy, submitOtp, headingRef }: Props) {
  const [otp, setOtp] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void submitOtp(otp);
  };

  return (
    <form onSubmit={submit} noValidate className="space-y-6">
      <StepTitle headingRef={headingRef} title="Xác thực giao dịch">
        Nhập mã OTP gồm 6 chữ số đã gửi đến email đăng ký của bạn.
      </StepTitle>

      <div>
        <label
          htmlFor="transfer-otp"
          className="text-sm font-semibold text-on-surface-variant"
        >
          Mã OTP
        </label>
        <input
          id="transfer-otp"
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={6}
          value={otp}
          onChange={(e) =>
            setOtp(e.target.value.replace(/\D/g, "").slice(0, 6))
          }
          placeholder="••••••"
          aria-invalid={Boolean(error)}
          aria-describedby={error ? "otp-error" : "otp-hint"}
          className={`mt-1 h-16 w-full rounded-2xl border bg-surface-container-lowest text-center font-mono text-3xl font-bold tracking-[0.5em] text-on-surface placeholder:text-on-surface-variant/40 focus:ring-2 focus:outline-none ${
            error
              ? "border-error focus:ring-error/20"
              : "border-outline-variant focus:border-secondary focus:ring-secondary/20"
          }`}
        />
        <FieldError id="otp-error" message={error} />
        <p id="otp-hint" className="mt-2 text-xs text-on-surface-variant">
          Môi trường demo — mã OTP là{" "}
          <span className="font-mono font-semibold text-on-surface">
            {DEMO_OTP}
          </span>
          .
        </p>
      </div>

      <Button
        type="submit"
        className="w-full"
        disabled={busy || otp.length !== 6}
      >
        {busy ? "Đang xử lý..." : "Xác thực và chuyển tiền"}
      </Button>
    </form>
  );
}
