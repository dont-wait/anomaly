import { useEffect, useState, type FormEvent } from "react";
import { navigate, routes } from "@/app/routes";
import {
  confirmPasswordReset,
  isValidCccd,
  normalizeCccd,
  requestPasswordReset,
} from "@/features/auth/api/auth";
import {
  isAcceptedNewPassword,
  OTP_LENGTH,
} from "@/features/registration/model";
import { ApiError } from "@/shared/lib/http";
import { toast } from "@/shared/notifications/toast";
import { Button, Input } from "@/shared/ui";

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const OTP_PATTERN = new RegExp(`^\\d{${OTP_LENGTH}}$`);
const RESEND_COOLDOWN_SECONDS = 30;

function toResetError(cause: unknown) {
  if (!(cause instanceof ApiError)) {
    return "Không thể đặt lại mật khẩu. Vui lòng thử lại.";
  }
  switch (cause.status) {
    case 0:
      return cause.message;
    case 400:
      return "Mã xác thực không đúng hoặc đã hết hạn. Hãy kiểm tra lại hoặc gửi lại mã.";
    case 409:
      return "Tài khoản vừa có thay đổi. Vui lòng gửi lại mã và thử lại.";
    case 429:
      return "Bạn đã thử quá nhiều lần. Vui lòng đợi rồi thử lại.";
    default:
      return "Hệ thống đang gặp sự cố. Vui lòng thử lại sau.";
  }
}

export function ForgotPasswordView() {
  const [step, setStep] = useState<"request" | "confirm">("request");
  const [cccd, setCccd] = useState("");
  const [email, setEmail] = useState("");
  const [otp, setOtp] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [cooldown, setCooldown] = useState(0);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = window.setTimeout(() => setCooldown(cooldown - 1), 1000);
    return () => window.clearTimeout(timer);
  }, [cooldown]);

  const sendCode = async () => {
    setBusy(true);
    setError("");
    try {
      await requestPasswordReset({
        cccdNumber: normalizeCccd(cccd),
        email: email.trim(),
      });
      setStep("confirm");
      setCooldown(RESEND_COOLDOWN_SECONDS);
    } catch (cause) {
      setError(toResetError(cause));
    } finally {
      setBusy(false);
    }
  };

  const submitRequest = (event: FormEvent) => {
    event.preventDefault();
    void sendCode();
  };

  const submitConfirm = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await confirmPasswordReset({
        cccdNumber: normalizeCccd(cccd),
        otp,
        newPassword,
      });
      toast.success("Đã đặt lại mật khẩu. Vui lòng đăng nhập.");
      navigate(routes.login);
    } catch (cause) {
      setError(toResetError(cause));
      setOtp("");
      setNewPassword("");
      setConfirmPassword("");
    } finally {
      setBusy(false);
    }
  };

  const canRequest = isValidCccd(cccd) && EMAIL_PATTERN.test(email.trim());
  const canConfirm =
    OTP_PATTERN.test(otp) &&
    isAcceptedNewPassword(newPassword) &&
    newPassword === confirmPassword;
  const mismatch = confirmPassword !== "" && confirmPassword !== newPassword;

  return (
    <div className="min-h-screen w-full bg-background">
      <main className="mx-auto flex min-h-screen w-full max-w-sm flex-col justify-center gap-4 px-4 py-6 sm:max-w-md">
        <h1 className="text-xl font-bold text-on-surface">Quên mật khẩu</h1>
        {step === "request" ? (
          <form className="flex flex-col gap-4" onSubmit={submitRequest}>
            <p className="text-sm text-on-surface-variant">
              Nhập số CCCD và email đã đăng ký để nhận mã xác thực.
            </p>
            <Input
              label="Số Căn cước Công dân"
              inputMode="numeric"
              autoComplete="username"
              maxLength={14}
              value={cccd}
              disabled={busy}
              onChange={(event) => setCccd(event.target.value)}
            />
            <Input
              label="Email đã đăng ký"
              type="email"
              autoComplete="email"
              value={email}
              disabled={busy}
              onChange={(event) => setEmail(event.target.value)}
            />
            {error && <ResetError message={error} />}
            <Button type="submit" disabled={busy || !canRequest}>
              {busy ? "Đang gửi..." : "Gửi mã xác thực"}
            </Button>
          </form>
        ) : (
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => void submitConfirm(event)}
          >
            <p className="text-sm text-on-surface-variant" role="status">
              Nếu thông tin khớp với một tài khoản, mã xác thực đã được gửi tới
              email của bạn và có hiệu lực trong 5 phút.
            </p>
            <Input
              label="Mã xác thực"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={OTP_LENGTH}
              value={otp}
              disabled={busy}
              onChange={(event) => setOtp(event.target.value)}
            />
            <Input
              label="Mật khẩu mới"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              disabled={busy}
              onChange={(event) => setNewPassword(event.target.value)}
            />
            <Input
              label="Xác nhận mật khẩu mới"
              type="password"
              autoComplete="new-password"
              value={confirmPassword}
              disabled={busy}
              onChange={(event) => setConfirmPassword(event.target.value)}
            />
            {mismatch && <ResetError message="Mật khẩu xác nhận không khớp." />}
            <p className="text-xs text-on-surface-variant">
              Mật khẩu mới cần 8–72 ký tự, có chữ hoa, chữ thường, số và ký tự
              đặc biệt. Sau khi đặt lại, mọi phiên đăng nhập sẽ hết hiệu lực.
            </p>
            {error && !mismatch && <ResetError message={error} />}
            <Button type="submit" disabled={busy || !canConfirm}>
              {busy ? "Đang cập nhật..." : "Đặt lại mật khẩu"}
            </Button>
            <button
              type="button"
              className="text-sm font-semibold text-secondary disabled:opacity-50"
              disabled={busy || cooldown > 0}
              onClick={() => void sendCode()}
            >
              {cooldown > 0 ? `Gửi lại mã sau ${cooldown}s` : "Gửi lại mã"}
            </button>
          </form>
        )}
        <a
          className="text-center text-sm text-on-surface-variant hover:underline"
          href={routes.login}
        >
          Quay lại đăng nhập
        </a>
      </main>
    </div>
  );
}

function ResetError({ message }: { message: string }) {
  return (
    <p
      role="alert"
      className="rounded-xl bg-error-container px-3 py-2 text-xs text-on-error-container"
    >
      {message}
    </p>
  );
}
