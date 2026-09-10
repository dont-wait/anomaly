import { useState, useRef, useEffect, type FormEvent } from "react";
import { stages, passwordRules, type Screen } from "./model";
import { useAuth } from "@/features/auth/useAuth";
import { defaultAuthTokenStore } from "@/features/auth/lib/token-store";
import { verifyFace } from "@/features/auth/api/kyc";
import {
  registerAccount,
  uploadMedia,
  verifyAccount,
  registrationError,
  type VerificationMedia,
} from "@/features/auth/api/registration";
import type { AuthUser } from "@/features/auth/api/auth";

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
      ? 3
      : screen === "success"
        ? 4
        : stages.indexOf(screen);
  useEffect(() => () => operation.current?.abort(), []);
  useEffect(() => {
    heading.current?.focus();
  }, [screen]);
  function go(next: Screen) {
    if (operation.current || createdAccount) return;
    setError("");
    if (next !== "password") verifiedVideo.current = null;
    setScreen(next);
  }
  async function verifyVideo(video: File) {
    if (operation.current || remaining === 0 || !documents.front) return;
    const controller = new AbortController();
    operation.current = controller;
    setBusy(true);
    setError("");
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
      } else {
        if (result.decision === "FAILED_FINAL") setRemaining(0);
        if (result.decision === "RETRY_ALLOWED")
          setRemaining((r) => Math.max(0, r - 1));
        setError(
          result.reason_message ||
            (result.decision === "SYSTEM_ERROR"
              ? "Dịch vụ xác thực gặp lỗi. Lượt thử của bạn được giữ nguyên."
              : "Xác thực chưa đạt. Vui lòng kiểm tra ảnh CCCD và quay lại video."),
        );
        setScreen("error");
      }
    } catch (cause) {
      if (!controller.signal.aborted) {
        setError(registrationError(cause));
        setScreen("error");
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
      if (!controller.signal.aborted) setError(registrationError(cause));
    } finally {
      if (!controller.signal.aborted) {
        setBusy(false);
        setProgress("");
      }
      if (operation.current === controller) operation.current = null;
    }
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    if (operation.current) return;
    if (screen === "email" && consent && email.trim()) go("document");
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
