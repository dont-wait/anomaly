import { useState, type ReactNode } from "react";
import { faArrowLeft } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { useAuth } from "@/features/auth/useAuth";
import {
  fetchProfileMedia,
  requestProfileChange,
} from "@/features/auth/api/profileMutations";
import type { ProfileMedia } from "@/features/auth/api/profile";
import { passwordRules } from "@/features/registration/model";
import { ApiError } from "@/shared/lib/http";
import { FileSourcePicker, SelectSheet } from "@/shared/ui";
import { PROVINCES } from "./provinces";
import type { ProfileChangeInput } from "@/features/auth/api/profile";
import {
  AVATAR_MAX_BYTES,
  AVATAR_TYPES,
  DOCUMENT_MAX_BYTES,
  DOCUMENT_TYPES,
  EMAIL_PATTERN,
  PHONE_PATTERN,
  documentLabel,
  formatDate,
  toProfileError,
  validateUpload,
} from "./profileHelpers";
import { useFilePreview, type ProfileDocumentsState } from "./useProfileData";

const OTP_PATTERN = /^\d{6}$/;

interface PanelProps {
  onBack: () => void;
}

interface ActionOptions {
  success?: string;
  badRequest?: string;
  /** Mã 401 ở màn đổi mật khẩu không được tự đăng xuất người dùng. */
  logoutOnUnauthorized?: boolean;
}

function useProfileAction() {
  const { logout } = useAuth();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const run = async (
    action: () => Promise<unknown>,
    options: ActionOptions = {},
  ) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
      if (options.success) setMessage(options.success);
      return true;
    } catch (cause) {
      if (
        cause instanceof ApiError &&
        cause.status === 401 &&
        options.logoutOnUnauthorized !== false
      ) {
        logout();
        return false;
      }
      setError(toProfileError(cause, options.badRequest));
      return false;
    } finally {
      setBusy(false);
    }
  };

  return { busy, message, error, run, setError, setMessage };
}

function PanelShell({
  title,
  onBack,
  busy,
  message,
  error,
  children,
}: PanelProps & {
  title: string;
  busy: boolean;
  message?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <section className="profile-card profile-form">
      <button
        type="button"
        className="profile-back"
        disabled={busy}
        onClick={onBack}
      >
        <FontAwesomeIcon icon={faArrowLeft} className="mr-1.5" />
        Hồ sơ của tôi
      </button>
      <h3>{title}</h3>
      {children}
      {message && (
        <p className="profile-message" role="status">
          {message}
        </p>
      )}
      {error && (
        <p className="profile-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

function ProfileInput({
  label,
  value,
  onChange,
  type = "text",
  autoComplete,
  inputMode,
  onBlur,
  error,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: string;
  autoComplete?: string;
  inputMode?: "numeric" | "tel" | "email";
  onBlur?: () => void;
  error?: string;
}) {
  return (
    <label className="profile-field">
      {label}
      <input
        className="profile-input"
        type={type}
        autoComplete={autoComplete}
        inputMode={inputMode}
        value={value}
        aria-invalid={Boolean(error)}
        onBlur={onBlur}
        onChange={(event) => onChange(event.target.value)}
      />
      {error && <span className="profile-field-error">{error}</span>}
    </label>
  );
}

export function EditProfilePanel({
  onBack,
  onChangeEmail,
  onSaved,
}: PanelProps & {
  onChangeEmail: () => void;
  onSaved: (submitted: boolean) => void;
}) {
  const { user, token, updateProfile, refreshProfile } = useAuth();
  const { busy, error, run, setError } = useProfileAction();
  const pending =
    user?.profileChange?.status === "PENDING" ? user.profileChange : undefined;
  const rejected =
    user?.profileChange?.status === "REJECTED" ? user.profileChange : undefined;
  const [phone, setPhone] = useState(user?.phone ?? "");
  const [phoneTouched, setPhoneTouched] = useState(false);
  const [serverPhoneError, setServerPhoneError] = useState<string>();
  const [fullName, setFullName] = useState(
    pending?.fullName ?? user?.fullName ?? "",
  );
  const [addressLine, setAddressLine] = useState(
    pending?.addressLine ?? user?.address?.line ?? "",
  );
  const [provinceCode, setProvinceCode] = useState(
    pending?.provinceCode ?? user?.address?.provinceCode ?? "",
  );
  const [issuedPlace, setIssuedPlace] = useState(
    pending?.issuedPlace ?? user?.identity?.issuedPlace ?? "",
  );
  const [permanentAddress, setPermanentAddress] = useState(
    pending?.permanentAddress ?? user?.identity?.permanentAddress ?? "",
  );

  // Khách thường gõ "0901 234 567", nên bỏ khoảng trắng và dấu ngăn trước khi kiểm tra.
  const nextPhone = phone.replace(/[\s.-]/g, "");
  const phoneChanged = nextPhone !== (user?.phone ?? "");
  const phoneInvalid = phoneChanged && !PHONE_PATTERN.test(nextPhone);

  // Chỉ gửi trường khác với hồ sơ hiện tại; admin duyệt rồi mới có hiệu lực.
  const change: ProfileChangeInput = {};
  const reviewed = [
    ["fullName", fullName, user?.fullName],
    ["addressLine", addressLine, user?.address?.line],
    ["provinceCode", provinceCode, user?.address?.provinceCode],
    ["issuedPlace", issuedPlace, user?.identity?.issuedPlace],
    ["permanentAddress", permanentAddress, user?.identity?.permanentAddress],
  ] as const;
  for (const [key, value, current] of reviewed) {
    if (value.trim() !== (current ?? "")) change[key] = value.trim();
  }
  const emptyField = Object.values(change).some((value) => value === "");
  const needsReview = Object.keys(change).length > 0;
  const samePending =
    !!pending &&
    reviewed.every(([key]) => (pending[key] ?? "") === (change[key] ?? ""));
  const submitsChange = needsReview && !samePending;

  const save = async () => {
    setServerPhoneError(undefined);
    let conflict = false;
    const saved = await run(async () => {
      try {
        if (phoneChanged) await updateProfile({ phone: nextPhone });
        if (submitsChange) {
          if (!token) throw new Error("Phiên đăng nhập đã hết hạn.");
          await requestProfileChange(token, change);
          await refreshProfile().catch(() => null);
        }
      } catch (cause) {
        if (cause instanceof ApiError) {
          setServerPhoneError(
            cause.errors.find((item) => item.field === "phone")?.detail,
          );
          conflict = cause.status === 409;
        }
        throw cause;
      }
    });
    if (saved) {
      onSaved(submitsChange);
    } else if (conflict) {
      // Tải lại hồ sơ mới nhất nhưng giữ nguyên nội dung khách đang nhập.
      await refreshProfile().catch(() => null);
      setError(
        "Hồ sơ vừa được thay đổi ở nơi khác. Hãy kiểm tra lại nội dung rồi lưu lại.",
      );
    }
  };

  return (
    <PanelShell
      title="Cập nhật thông tin"
      onBack={onBack}
      busy={busy}
      error={error}
    >
      {pending && (
        <p className="profile-note" role="status">
          Thay đổi của bạn đang chờ ngân hàng duyệt. Gửi lại sẽ thay thế yêu cầu
          đang chờ.
        </p>
      )}
      {rejected && (
        <p className="profile-error" role="alert">
          Yêu cầu trước không được duyệt: {rejected.rejectionReason}
        </p>
      )}

      <h4 className="profile-form-group">Liên hệ</h4>
      <ProfileInput
        label="Số điện thoại"
        type="tel"
        inputMode="tel"
        autoComplete="tel"
        value={phone}
        onChange={setPhone}
        onBlur={() => setPhoneTouched(true)}
        error={
          phoneTouched && phoneInvalid
            ? "Số điện thoại gồm 8-16 chữ số, có thể bắt đầu bằng +."
            : serverPhoneError
        }
      />
      <div className="profile-readonly">
        <div>
          <span>Email</span>
          <strong>{user?.email}</strong>
          {user?.pendingEmail && (
            <small>Đang chờ xác minh: {user.pendingEmail}</small>
          )}
        </div>
        <button type="button" disabled={busy} onClick={onChangeEmail}>
          Đổi email
        </button>
      </div>

      <h4 className="profile-form-group">Cần ngân hàng duyệt</h4>
      <ProfileInput
        label="Họ và tên"
        autoComplete="name"
        value={fullName}
        onChange={setFullName}
      />
      <ProfileInput
        label="Số nhà, đường"
        autoComplete="street-address"
        value={addressLine}
        onChange={setAddressLine}
      />
      <div className="profile-field">
        Tỉnh/thành
        <SelectSheet
          label="Tỉnh/thành"
          searchable
          value={provinceCode}
          placeholder="Chọn tỉnh/thành"
          onChange={setProvinceCode}
          options={PROVINCES.map((item) => ({
            value: item.code,
            label: item.name,
          }))}
        />
      </div>
      <ProfileInput
        label="Nơi cấp CCCD"
        value={issuedPlace}
        onChange={setIssuedPlace}
      />
      <ProfileInput
        label="Địa chỉ thường trú"
        value={permanentAddress}
        onChange={setPermanentAddress}
      />
      {emptyField && (
        <p className="profile-field-error">
          Không thể để trống thông tin đã có.
        </p>
      )}
      <p className="profile-caption">
        Số CCCD, ngày sinh và ngày cấp không thể đổi tại đây.
      </p>
      <div className="profile-form-actions">
        <button
          type="button"
          className="profile-action profile-action-secondary"
          disabled={busy}
          onClick={onBack}
        >
          Hủy
        </button>
        <button
          type="button"
          className="profile-action"
          disabled={
            busy ||
            phoneInvalid ||
            emptyField ||
            (!phoneChanged && !submitsChange)
          }
          onClick={() => void save()}
        >
          {busy
            ? "Đang gửi..."
            : submitsChange
              ? "Gửi yêu cầu duyệt"
              : "Lưu thay đổi"}
        </button>
      </div>
    </PanelShell>
  );
}

export function ChangeEmailPanel({ onBack }: PanelProps) {
  const { user, startEmailChange, confirmEmailChange } = useAuth();
  const { busy, message, error, run, setMessage } = useProfileAction();
  const [newEmail, setNewEmail] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newEmailOtp, setNewEmailOtp] = useState("");
  const [restarting, setRestarting] = useState(false);

  const nextEmail = newEmail.trim();
  const showOtpStep = Boolean(user?.pendingEmail) && !restarting;

  const start = async () => {
    const started = await run(
      () => startEmailChange({ newEmail: nextEmail, currentPassword }),
      {
        success: "Đã gửi mã xác minh đến email mới.",
        badRequest: "Mật khẩu hiện tại không đúng hoặc email chưa hợp lệ.",
        logoutOnUnauthorized: false,
      },
    );
    setCurrentPassword("");
    if (started) {
      setRestarting(false);
      setNewEmail("");
      setNewEmailOtp("");
    }
  };

  const confirm = async () => {
    const confirmed = await run(() => confirmEmailChange({ newEmailOtp }), {
      success: "Đã đổi email.",
      badRequest: "Mã xác minh không đúng hoặc đã hết hạn.",
    });
    setNewEmailOtp("");
    if (!confirmed) setMessage("");
  };

  return (
    <PanelShell
      title="Đổi email"
      onBack={onBack}
      busy={busy}
      message={message}
      error={error}
    >
      <p className="profile-note">
        Email hiện tại ({user?.email}) vẫn được dùng cho đăng nhập, thông báo và
        mã OTP cho đến khi bạn xác minh xong email mới.
      </p>
      {showOtpStep ? (
        <>
          <p className="profile-caption">
            Email mới đang chờ: {user?.pendingEmail}
          </p>
          <ProfileInput
            label="Mã gửi đến email mới"
            inputMode="numeric"
            autoComplete="one-time-code"
            value={newEmailOtp}
            onChange={setNewEmailOtp}
          />
          <button
            type="button"
            className="profile-action"
            disabled={busy || !OTP_PATTERN.test(newEmailOtp)}
            onClick={() => void confirm()}
          >
            {busy ? "Đang xác minh..." : "Xác minh email"}
          </button>
          <button
            type="button"
            className="profile-action profile-action-secondary"
            disabled={busy}
            onClick={() => setRestarting(true)}
          >
            Dùng email khác
          </button>
        </>
      ) : (
        <>
          <ProfileInput
            label="Email mới"
            type="email"
            inputMode="email"
            autoComplete="email"
            value={newEmail}
            onChange={setNewEmail}
          />
          <ProfileInput
            label="Mật khẩu hiện tại"
            type="password"
            autoComplete="current-password"
            value={currentPassword}
            onChange={setCurrentPassword}
          />
          <button
            type="button"
            className="profile-action"
            disabled={
              busy ||
              !currentPassword ||
              !EMAIL_PATTERN.test(nextEmail) ||
              nextEmail === user?.email
            }
            onClick={() => void start()}
          >
            {busy ? "Đang gửi mã..." : "Gửi mã xác minh"}
          </button>
        </>
      )}
    </PanelShell>
  );
}

export function ChangePasswordPanel({ onBack }: PanelProps) {
  const { changePassword } = useAuth();
  const { busy, error, run } = useProfileAction();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const strong = passwordRules(newPassword).every(Boolean);
  const mismatch = confirmPassword !== "" && confirmPassword !== newPassword;

  const submit = async () => {
    await run(() => changePassword({ currentPassword, newPassword }), {
      badRequest:
        "Mật khẩu hiện tại không đúng hoặc mật khẩu mới chưa đạt yêu cầu.",
      logoutOnUnauthorized: false,
    });
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
  };

  return (
    <PanelShell title="Đổi mật khẩu" onBack={onBack} busy={busy} error={error}>
      <ProfileInput
        label="Mật khẩu hiện tại"
        type="password"
        autoComplete="current-password"
        value={currentPassword}
        onChange={setCurrentPassword}
      />
      <ProfileInput
        label="Mật khẩu mới"
        type="password"
        autoComplete="new-password"
        value={newPassword}
        onChange={setNewPassword}
      />
      <ProfileInput
        label="Xác nhận mật khẩu mới"
        type="password"
        autoComplete="new-password"
        value={confirmPassword}
        onChange={setConfirmPassword}
        error={mismatch ? "Mật khẩu xác nhận không khớp." : undefined}
      />
      <p className="profile-note">
        Mật khẩu mới cần ít nhất 8 ký tự, có chữ hoa, chữ thường, số và ký tự
        đặc biệt. Sau khi đổi, mọi phiên đăng nhập sẽ hết hiệu lực và bạn cần
        đăng nhập lại.
      </p>
      <button
        type="button"
        className="profile-action"
        disabled={
          busy || !currentPassword || !strong || newPassword !== confirmPassword
        }
        onClick={() => void submit()}
      >
        {busy ? "Đang cập nhật..." : "Cập nhật mật khẩu"}
      </button>
    </PanelShell>
  );
}

function UploadProgressBar({ percent }: { percent: number }) {
  return (
    <label className="profile-field">
      Đang tải lên {percent}%
      <progress className="profile-progress" max={100} value={percent} />
    </label>
  );
}

export function AvatarPanel({ onBack }: PanelProps) {
  const { uploadProfileAvatar } = useAuth();
  const { busy, message, error, run, setError, setMessage } =
    useProfileAction();
  const [file, setFile] = useState<File | null>(null);
  const [percent, setPercent] = useState(0);
  const preview = useFilePreview(file);

  const choose = (next: File | null) => {
    setMessage("");
    const invalid = next
      ? validateUpload(next, AVATAR_TYPES, AVATAR_MAX_BYTES)
      : "";
    setError(invalid);
    setFile(invalid ? null : next);
  };

  const save = async () => {
    if (!file) return;
    setPercent(0);
    const saved = await run(() => uploadProfileAvatar(file, setPercent), {
      success: "Đã cập nhật ảnh đại diện.",
    });
    if (saved) setFile(null);
  };

  return (
    <PanelShell
      title="Đổi ảnh đại diện"
      onBack={onBack}
      busy={busy}
      message={message}
      error={error}
    >
      <p className="profile-note">
        Chọn ảnh JPEG, PNG hoặc WebP, tối đa 5 MiB. Ảnh hiển thị dạng tròn, căn
        giữa.
      </p>
      {preview && (
        <img
          className="profile-image-preview"
          src={preview}
          alt="Xem trước ảnh đại diện"
        />
      )}
      <FileSourcePicker
        label="Mở camera hoặc chọn ảnh"
        accept={AVATAR_TYPES}
        camera="user"
        disabled={busy}
        onSelect={choose}
      />
      {busy && <UploadProgressBar percent={percent} />}
      {file && (
        <>
          <button
            type="button"
            className="profile-action"
            disabled={busy}
            onClick={() => void save()}
          >
            {busy ? "Đang tải ảnh..." : error ? "Thử lại" : "Lưu ảnh"}
          </button>
          <button
            type="button"
            className="profile-action profile-action-secondary"
            disabled={busy}
            onClick={() => choose(null)}
          >
            Hủy
          </button>
        </>
      )}
    </PanelShell>
  );
}

function saveBlob(blob: Blob, fileName: string) {
  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = fileName;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
}

export function DocumentsPanel({
  onBack,
  documents,
}: PanelProps & { documents: ProfileDocumentsState }) {
  const { token, uploadProfileDocument } = useAuth();
  const { busy, message, error, run, setError, setMessage } =
    useProfileAction();
  const [uploadType, setUploadType] = useState<string | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [percent, setPercent] = useState(0);
  const [uploading, setUploading] = useState(false);
  const [imagePreview, setImagePreview] = useState<Blob | null>(null);
  const localPreview = useFilePreview(
    file?.type.startsWith("image/") ? file : null,
  );
  const remotePreview = useFilePreview(imagePreview);

  const choose = (next: File | null) => {
    setMessage("");
    const invalid = next
      ? validateUpload(next, DOCUMENT_TYPES, DOCUMENT_MAX_BYTES)
      : "";
    setError(invalid);
    setFile(invalid ? null : next);
  };

  const download = (document: ProfileMedia) =>
    run(async () => {
      if (!token) throw new Error("Phiên đăng nhập đã hết hạn.");
      saveBlob(
        await fetchProfileMedia(token, document.mediaId),
        document.fileName,
      );
    });

  const preview = (document: ProfileMedia) =>
    run(async () => {
      if (!token) throw new Error("Phiên đăng nhập đã hết hạn.");
      setImagePreview(await fetchProfileMedia(token, document.mediaId));
    });

  const upload = async () => {
    if (!file || !uploadType) return;
    setPercent(0);
    setUploading(true);
    const saved = await run(
      async () => {
        await uploadProfileDocument(uploadType, file, setPercent);
        await documents.reload();
      },
      { success: "Đã tải chứng từ lên hồ sơ." },
    );
    setUploading(false);
    if (saved) {
      setFile(null);
      setUploadType(null);
    }
  };

  return (
    <PanelShell
      title="Chứng từ cá nhân"
      onBack={onBack}
      busy={busy}
      message={message}
      error={error}
    >
      <p className="profile-note">
        Chấp nhận PDF, JPEG hoặc PNG, tối đa 10 MiB mỗi tệp. Tải lên loại đã có
        sẽ thay thế tệp cũ. CCCD đã xác minh chỉ có thể xem tại đây.
      </p>
      {documents.loading && documents.slots.length === 0 && (
        <p className="profile-caption" role="status">
          Đang tải chứng từ...
        </p>
      )}
      {documents.error && (
        <div className="profile-error" role="alert">
          {documents.error}{" "}
          <button
            type="button"
            className="profile-link"
            onClick={() => void documents.reload()}
          >
            Thử lại
          </button>
        </div>
      )}
      {!documents.loading &&
        !documents.error &&
        documents.slots.length === 0 && (
          <p className="profile-caption">
            Hiện chưa có chứng từ nào được hỗ trợ.
          </p>
        )}
      {documents.slots.map((slot) => (
        <div className="profile-document" key={slot.type}>
          <div>
            <strong>
              {documentLabel(slot.type)}
              {slot.required ? " · Bắt buộc" : ""}
            </strong>
            <p>
              {slot.document
                ? `${slot.document.fileName} · ${formatDate(slot.document.uploadedAt)}`
                : "Chưa tải lên"}
            </p>
          </div>
          <div className="profile-document-actions">
            {slot.document?.contentType.startsWith("image/") && (
              <button
                type="button"
                disabled={busy}
                onClick={() => void preview(slot.document as ProfileMedia)}
              >
                Xem
              </button>
            )}
            {slot.document && (
              <button
                type="button"
                disabled={busy}
                onClick={() => void download(slot.document as ProfileMedia)}
              >
                Tải xuống
              </button>
            )}
            {slot.replaceable && (
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  setUploadType(slot.type);
                  choose(null);
                }}
              >
                {slot.document ? "Thay thế" : "Tải lên"}
              </button>
            )}
          </div>
        </div>
      ))}
      {remotePreview && (
        <img
          className="profile-document-preview"
          src={remotePreview}
          alt="Xem trước chứng từ"
        />
      )}
      {uploadType && (
        <>
          <FileSourcePicker
            label={`${documentLabel(uploadType)}: mở camera hoặc chọn tệp`}
            accept={DOCUMENT_TYPES}
            camera="environment"
            disabled={busy}
            onSelect={choose}
          />
          {localPreview && (
            <img
              className="profile-document-preview"
              src={localPreview}
              alt="Xem trước tệp đã chọn"
            />
          )}
          {file && !localPreview && (
            <p className="profile-caption">{file.name}</p>
          )}
          {uploading && <UploadProgressBar percent={percent} />}
          {file && (
            <button
              type="button"
              className="profile-action"
              disabled={busy}
              onClick={() => void upload()}
            >
              {uploading
                ? "Đang tải chứng từ..."
                : error
                  ? "Thử lại"
                  : "Tải chứng từ lên"}
            </button>
          )}
          <button
            type="button"
            className="profile-action profile-action-secondary"
            disabled={busy}
            onClick={() => {
              setUploadType(null);
              choose(null);
            }}
          >
            Hủy
          </button>
        </>
      )}
    </PanelShell>
  );
}
