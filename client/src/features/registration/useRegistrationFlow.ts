import { toast } from "@/shared/notifications/toast";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { ApiError } from "@/shared/lib/http";
import { useState, useRef, useEffect, type FormEvent } from "react";
import { stages, passwordRules, OTP_LENGTH, type Screen } from "./model";
import { useAuth } from "@/features/auth/useAuth";
import { defaultAuthTokenStore } from "@/features/auth/lib/token-store";
import {
  verifyFace,
  registerAccount,
  uploadMedia,
  verifyAccount,
  registrationError,
  requestOtp,
  verifyOtp,
  otpError,
  type VerificationMedia,
} from "./api";
import type { AuthUser } from "@/features/auth/api";

const OTP_RESEND_SECONDS = 30;

export function useRegistrationFlow() {
  const auth = useAuth();
  const [screen, setScreen] = useState<Screen>("email");
  const [email, setEmail] = useState("");
  const [consent, setConsent] = useState(false);
  const [error, setError] = useState("");
  const [documents, setDocuments] = useState<{
    front: File | null;
    back: File | null;
  }>({ front: null, back: null });
  const [profile, setProfile] = useState({
    name: "",
    id: "",
    dob: "",
    issuedDate: "",
  });
  const [otpCode, setOtpCode] = useState("");
  // otpBusy = đang xác thực (khoá ô nhập); otpSending = đang gửi mã (vẫn cho
  // nhập); otpSent = đã gửi xong ít nhất một lần kể từ khi vào màn OTP.
  const [otpBusy, setOtpBusy] = useState(false);
  const [otpSending, setOtpSending] = useState(false);
  const [otpSent, setOtpSent] = useState(false);
  const [otpErrorMsg, setOtpErrorMsg] = useState("");
  const [otpErrorTick, setOtpErrorTick] = useState(0);
  const [resendIn, setResendIn] = useState(0);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [remaining, setRemaining] = useState(3);
  const [dialog, setDialog] = useState("");
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState("");
  const [createdAccount, setCreatedAccount] = useState<AuthUser | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const operation = useRef<AbortController | null>(null);
  const verifiedVideo = useRef<File | null>(null);
  const media = useRef<Partial<VerificationMedia>>({});
  const uploadPrefix = useRef("");
  const registrationKey = useRef("");
  const step =
    screen === "processing" || screen === "error"
      ? stages.indexOf("face")
      : screen === "success"
        ? stages.indexOf("password")
        : stages.indexOf(screen);
  useEffect(() => () => operation.current?.abort(), []);
  useEffect(() => {
    heading.current?.focus();
  }, [screen]);
  useEffect(() => {
    if (screen !== "otp") return;
    const timer = setInterval(
      () => setResendIn((left) => (left > 0 ? left - 1 : 0)),
      1000,
    );
    return () => clearInterval(timer);
  }, [screen]);
  // Counter tăng mỗi lần báo lỗi để màn OTP biết có lỗi mới kể cả khi nội
  // dung message không đổi.
  function showOtpError(message: string) {
    setOtpErrorMsg(message);
    setOtpErrorTick((tick) => tick + 1);
  }
  function go(next: Screen) {
    if (operation.current || createdAccount) return;
    setError("");
    if (next !== "password") verifiedVideo.current = null;
    setScreen(next);
  }
  // Trả về message lỗi, hoặc null khi gửi mã thành công.
  async function sendOtp(): Promise<string | null> {
    const controller = new AbortController();
    operation.current = controller;
    setOtpSending(true);
    // Server bật cooldown ngay khi nhận request nên đếm ngược từ lúc bắt đầu
    // gửi, không phải lúc nhận được phản hồi.
    setResendIn(OTP_RESEND_SECONDS);
    try {
      await requestOtp(email, controller.signal);
      if (controller.signal.aborted) return null;
      setOtpSent(true);
      return null;
    } catch (cause) {
      if (controller.signal.aborted) return null;
      // Gửi hỏng thì không khoá user 30 giây.
      setResendIn(0);
      return otpError(cause);
    } finally {
      if (!controller.signal.aborted) setOtpSending(false);
      if (operation.current === controller) operation.current = null;
    }
  }
  function startOtp() {
    if (operation.current) return;
    setOtpCode("");
    setOtpErrorMsg("");
    setOtpSent(false);
    // Sang màn OTP ngay, mã gửi chạy nền; lỗi báo tại chỗ chứ không kéo
    // user ngược về bước email. sendOtp tự bắt lỗi nên .then là đủ.
    go("otp");
    void sendOtp()
      .then((failure) => {
        if (failure) showOtpError(failure);
      })
      .catch(() => {});
  }
  async function resendOtp() {
    if (operation.current || resendIn > 0) return;
    setOtpErrorMsg("");
    const failure = await sendOtp();
    if (failure) showOtpError(failure);
  }
  async function confirmOtp() {
    if (operation.current || otpCode.length !== OTP_LENGTH) return;
    const controller = new AbortController();
    operation.current = controller;
    setOtpBusy(true);
    setOtpErrorMsg("");
    setProgress("Đang xác thực mã…");
    let verified = false;
    try {
      await verifyOtp(email, otpCode, controller.signal);
      verified = !controller.signal.aborted;
    } catch (cause) {
      if (!controller.signal.aborted) {
        showOtpError(otpError(cause));
        // Mã đã hết hạn hoặc hết lượt thử: xoá các ô để user nhập mã mới.
        if (cause instanceof ApiError && cause.status === HTTP_STATUS.GONE)
          setOtpCode("");
      }
    } finally {
      if (!controller.signal.aborted) {
        setOtpBusy(false);
        setProgress("");
      }
      if (operation.current === controller) operation.current = null;
    }
    if (verified) {
      // Mã đã bị consume: xoá để quay lại màn OTP không tự xác thực lại.
      setOtpCode("");
      go("document");
    }
  }
  async function verifyVideo(video: File) {
    if (operation.current || remaining === 0 || !documents.front) return;
    const controller = new AbortController();
    operation.current = controller;
    setBusy(true);
    setError("");
    toast.dismiss();
    setScreen("processing");
    setProgress("Đang kiểm tra ảnh CCCD, liveness và đối chiếu khuôn mặt…");
    try {
      const result = await verifyFace(
        documents.front,
        video,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      if (result.decision === "VERIFIED") {
        verifiedVideo.current = video;
        setScreen("password");
      } else if (result.decision === "SYSTEM_ERROR") {
        toast.error(
          result.reason_message ||
            "Dịch vụ xác thực gặp lỗi. Lượt thử của bạn được giữ nguyên.",
        );
        setScreen("face");
      } else {
        if (result.decision === "FAILED_FINAL") setRemaining(0);
        if (result.decision === "RETRY_ALLOWED")
          setRemaining((r) => Math.max(0, r - 1));
        setError(
          result.reason_message ||
            "Xác thực chưa đạt. Vui lòng kiểm tra ảnh CCCD và quay lại video.",
        );
        setScreen("error");
      }
    } catch (cause) {
      if (!controller.signal.aborted) {
        toast.error(registrationError(cause));
        setScreen("face");
      }
    } finally {
      if (!controller.signal.aborted) setBusy(false);
      if (operation.current === controller) operation.current = null;
    }
  }
  async function finish() {
    if (
      operation.current ||
      !verifiedVideo.current ||
      !documents.front ||
      !documents.back
    )
      return;
    if (!passwordRules(password).every(Boolean) || password !== confirm) {
      setError("Vui lòng kiểm tra lại mật khẩu.");
      return;
    }
    const controller = new AbortController();
    operation.current = controller;
    setBusy(true);
    setError("");
    toast.dismiss();
    try {
      let account = createdAccount;
      if (!account) {
        setProgress("Đang tạo tài khoản…");
        if (!registrationKey.current)
          registrationKey.current = crypto.randomUUID();
        account = await registerAccount(
          {
            username: profile.name.trim(),
            cccdNumber: profile.id,
            dob: `${profile.dob}T00:00:00Z`,
            cccdIssuedDate: `${profile.issuedDate}T00:00:00Z`,
            email: email.trim(),
            password,
          },
          controller.signal,
          registrationKey.current,
        );
        if (controller.signal.aborted) return;
        setCreatedAccount(account);
      }
      setProgress("Đang đăng nhập…");
      const loggedInUser = await auth.login(
        { cccdNumber: profile.id, password },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) return;
      if (loggedInUser.id !== account.id)
        throw new Error("Tài khoản đăng nhập không khớp hồ sơ vừa tạo.");
      const token = defaultAuthTokenStore.getToken();
      if (!token)
        throw new Error(
          "Không lưu được phiên đăng nhập. Vui lòng cho phép lưu trữ và thử lại.",
        );
      setProgress("Đang tải ảnh CCCD và video…");
      if (!uploadPrefix.current)
        uploadPrefix.current = `kyc/${account.id}/${crypto.randomUUID()}`;
      const files = {
        idCardFrontUrl: documents.front,
        idCardBackUrl: documents.back,
        liveVideoUrl: verifiedVideo.current,
      };
      for (const key of Object.keys(files) as (keyof VerificationMedia)[]) {
        if (!media.current[key])
          media.current[key] = await uploadMedia(
            files[key],
            `${uploadPrefix.current}/${key}`,
            token,
            controller.signal,
          );
        if (controller.signal.aborted) return;
      }
      setProgress("Đang hoàn tất xác thực tài khoản…");
      await verifyAccount(
        account.id,
        media.current as VerificationMedia,
        token,
        controller.signal,
      );
      if (controller.signal.aborted) return;
      // The existing provider owns the token and current profile.
      await auth.refreshProfile({ signal: controller.signal });
      if (controller.signal.aborted) return;
      setPassword("");
      setConfirm("");
      verifiedVideo.current = null;
      setDocuments({ front: null, back: null });
      setScreen("success");
    } catch (cause) {
      if (!controller.signal.aborted) toast.error(registrationError(cause));
    } finally {
      if (!controller.signal.aborted) {
        setBusy(false);
        setProgress("");
      }
      if (operation.current === controller) operation.current = null;
    }
  }
  function submit(event?: FormEvent) {
    event?.preventDefault();
    if (operation.current) return;
    if (screen === "email" && consent && email.trim()) startOtp();
    if (screen === "otp") void confirmOtp();
    if (screen === "document" && documents.front && documents.back)
      go("profile");
    if (screen === "profile") {
      if (profile.issuedDate < profile.dob) {
        setError("Ngày cấp CCCD phải từ ngày sinh trở đi.");
        return;
      }
      go("face");
    }
    if (screen === "password") void finish();
  }
  function retry() {
    if (remaining > 0) go("face");
  }
  return {
    screen,
    email,
    setEmail,
    consent,
    setConsent,
    error,
    setError,
    documents,
    setDocuments,
    profile,
    setProfile,
    otpCode,
    setOtpCode,
    otpBusy,
    otpSending,
    otpSent,
    otpErrorMsg,
    otpErrorTick,
    resendIn,
    resendOtp,
    password,
    setPassword,
    confirm,
    setConfirm,
    remaining,
    dialog,
    setDialog,
    heading,
    step,
    go,
    submit,
    retry,
    verifyVideo,
    busy,
    progress,
    createdAccount,
  };
}
export type RegistrationFlowState = ReturnType<typeof useRegistrationFlow>;
