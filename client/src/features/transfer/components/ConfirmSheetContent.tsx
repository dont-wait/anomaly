import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { ShieldIcon } from "@/shared/icons";
import { Button } from "@/shared/ui";
import { formatVnd } from "@/features/transactions/utils/format";
import { DEMO_OTP } from "@/features/transfer/api/transfer";
import { OTP_LENGTH } from "@/features/transfer/model";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";
import { BankLogo } from "./BankLogo";
import { OtpInput } from "./OtpInput";

const RESEND_SECONDS = 60;

const countdownLabel = (seconds: number) =>
  `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, "0")}`;

const Row = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex items-start justify-between gap-4 py-2.5">
    <dt className="shrink-0 text-sm text-on-surface-variant">{label}</dt>
    <dd className="min-w-0 text-right text-sm font-medium break-words text-on-surface">
      {children}
    </dd>
  </div>
);

type Props = Pick<
  TransferFlowState,
  "recipient" | "amount" | "note" | "otpError" | "busy" | "submitOtp"
>;

/** Nội dung sheet xác thực: tóm tắt giao dịch ở trên, 6 ô OTP ở dưới. */
export function ConfirmSheetContent({
  recipient,
  amount,
  note,
  otpError,
  busy,
  submitOtp,
}: Props) {
  const [otp, setOtp] = useState("");
  const [secondsLeft, setSecondsLeft] = useState(RESEND_SECONDS);
  const [resent, setResent] = useState(false);

  // Đếm ngược cho nút gửi lại mã.
  useEffect(() => {
    if (secondsLeft <= 0) return;
    const timer = setTimeout(() => setSecondsLeft((s) => s - 1), 1000);
    return () => clearTimeout(timer);
  }, [secondsLeft]);

  if (!recipient) return null;
  // Người dùng đã bắt đầu gõ mã mới thì ẩn lỗi của lần trước.
  const error = otp.length === 0 ? otpError : "";

  const submit = async (value: string) => {
    await submitOtp(value);
    // Sai mã thì xoá để nhập lại từ đầu; đúng mã thì sheet đã chuyển sang kết quả.
    setOtp("");
  };

  const onChange = (value: string) => {
    setOtp(value);
    if (value.length === OTP_LENGTH && !busy) void submit(value);
  };

  // TODO: gọi API gửi lại OTP (2.5 trong specs/transaction-spec.md) khi backend có.
  const resend = () => {
    setSecondsLeft(RESEND_SECONDS);
    setResent(true);
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    void submit(otp);
  };

  return (
    <form onSubmit={onSubmit} noValidate className="space-y-5">
      <section
        aria-label="Thông tin giao dịch"
        className="rounded-3xl bg-surface-container-low p-4"
      >
        <div className="flex items-center gap-3">
          <BankLogo bank={recipient.bank} />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold text-on-surface">
              {recipient.name}
            </p>
            <p className="truncate text-xs text-on-surface-variant">
              {recipient.accountNo} · {recipient.bank.shortName}
            </p>
          </div>
        </div>
        <p className="mt-4 text-center text-3xl font-bold tracking-tight text-on-surface tabular-nums">
          {formatVnd(amount)}
        </p>
        <dl className="mt-2 divide-y divide-outline-variant/50">
          <Row label="Lời nhắn">{note.trim() || "—"}</Row>
          <Row label="Phí giao dịch">
            <span className="text-success">Miễn phí</span>
          </Row>
        </dl>
      </section>

      <div>
        <label
          htmlFor="transfer-otp"
          className="text-sm font-semibold text-on-surface"
        >
          Mã OTP
        </label>
        <p
          id="otp-help"
          className="mt-0.5 mb-3 text-xs text-on-surface-variant"
        >
          Nhập {OTP_LENGTH} chữ số đã gửi đến email đăng ký của bạn.
        </p>
        <OtpInput
          id="transfer-otp"
          value={otp}
          onChange={onChange}
          invalid={Boolean(error)}
          disabled={busy}
          describedBy={error ? "otp-error otp-help" : "otp-help otp-demo"}
        />
        {error && (
          <p
            id="otp-error"
            role="alert"
            className="mt-2 text-sm font-medium text-error"
          >
            {error}
          </p>
        )}
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <p id="otp-demo" className="text-xs text-on-surface-variant">
            Môi trường demo — mã OTP là{" "}
            <span className="font-mono font-semibold text-on-surface">
              {DEMO_OTP}
            </span>
            .
          </p>
          {secondsLeft > 0 ? (
            <p className="text-xs text-on-surface-variant tabular-nums">
              Gửi lại mã sau {countdownLabel(secondsLeft)}
            </p>
          ) : (
            <button
              type="button"
              onClick={resend}
              className="-mr-2 min-h-11 rounded-full px-3 text-sm font-semibold text-secondary-strong transition-colors hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
            >
              Gửi lại mã
            </button>
          )}
        </div>
        <p role="status" className="mt-1 text-xs text-success">
          {resent && "Đã gửi lại mã OTP đến email của bạn."}
        </p>
      </div>

      <p className="flex gap-2 text-xs text-on-surface-variant">
        <ShieldIcon
          aria-hidden="true"
          className="h-4 w-4 shrink-0 text-secondary-strong"
        />
        Giao dịch không thể hoàn tác sau khi xác thực. AnomalyBank không bao giờ
        hỏi mã OTP của bạn.
      </p>

      <Button
        type="submit"
        className="w-full"
        disabled={busy || otp.length !== OTP_LENGTH}
      >
        {busy ? (
          <>
            <span
              aria-hidden="true"
              className="h-4 w-4 animate-spin rounded-full border-2 border-on-cta/40 border-t-on-cta motion-reduce:animate-none"
            />
            Đang xử lý...
          </>
        ) : (
          "Xác nhận chuyển tiền"
        )}
      </Button>
    </form>
  );
}
