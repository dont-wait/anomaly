import { useState, type ChangeEvent, type ReactNode } from "react";
import {
  faArrowLeft,
  faCamera,
  faFolderOpen,
  faLock,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { useAuth } from "@/features/auth/useAuth";
import { fetchProfileMedia } from "@/features/auth/api/profileMutations";
import type { ProfileMedia } from "@/features/auth/api/profile";
import type { ProfileUpdateInput } from "@/features/auth/api/profileMutations";
import { passwordRules } from "@/features/registration/model";
import { ApiError } from "@/shared/lib/http";
import {
  AVATAR_MAX_BYTES,
  AVATAR_TYPES,
  DOCUMENT_MAX_BYTES,
  DOCUMENT_TYPES,
  EMAIL_PATTERN,
  PHONE_PATTERN,
  documentLabel,
  formatDate,
  maskTail,
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
}: PanelProps & { onChangeEmail: () => void; onSaved: () => void }) {
  const { user, updateProfile, refreshProfile } = useAuth();
  const { busy, error, run, setError } = useProfileAction();
  const [phone, setPhone] = useState(user?.phone ?? "");
  const [line, setLine] = useState(user?.address?.line ?? "");
  const [province, setProvince] = useState(user?.address?.provinceCode ?? "");
  const [phoneTouched, setPhoneTouched] = useState(false);
  const [serverFields, setServerFields] = useState<Record<string, string>>({});

  // Khách thường gõ "0901 234 567", nên bỏ khoảng trắng và dấu ngăn trước khi kiểm tra.
  const nextPhone = phone.replace(/[\s.-]/g, "");
  const nextLine = line.trim();
  const nextProvince = province.trim();

  // PATCH chỉ gửi field đã đổi; field bỏ trống thì backend giữ nguyên.
  const input: ProfileUpdateInput = {};
  if (nextPhone !== (user?.phone ?? "")) input.phone = nextPhone;
  if (nextLine !== (user?.address?.line ?? "")) {
    input.address = { ...input.address, line: nextLine };
  }
  if (nextProvince !== (user?.address?.provinceCode ?? "")) {
    input.address = { ...input.address, provinceCode: nextProvince };
  }

  const phoneInvalid =
    input.phone !== undefined && !PHONE_PATTERN.test(nextPhone);
  const lineEmpty = input.address?.line === "";
  const provinceEmpty = input.address?.provinceCode === "";
  const hasChanges = input.phone !== undefined || input.address !== undefined;

  const save = async () => {
    setServerFields({});
    let conflict = false;
    const saved = await run(async () => {
      try {
        await updateProfile(input);
      } catch (cause) {
        if (cause instanceof ApiError) {
          setServerFields(
            Object.fromEntries(
              cause.errors.flatMap((item) =>
                item.field ? [[item.field, item.detail]] : [],
              ),
            ),
          );
          conflict = cause.status === 409;
        }
        throw cause;
      }
    });
    if (saved) {
      onSaved();
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
            : serverFields.phone
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

      <h4 className="profile-form-group">Địa chỉ liên hệ</h4>
      <ProfileInput
        label="Số nhà, đường"
        autoComplete="street-address"
        value={line}
        onChange={setLine}
        error={
          lineEmpty
            ? "Không thể để trống địa chỉ."
            : serverFields["address.line"]
        }
      />
      <ProfileInput
        label="Tỉnh/thành"
        autoComplete="address-level1"
        value={province}
        onChange={setProvince}
        error={
          provinceEmpty
            ? "Không thể để trống tỉnh/thành."
            : serverFields["address.provinceCode"]
        }
      />

      {user?.isVerify && (
        <>
          <h4 className="profile-form-group">Đã xác minh</h4>
          <ReadOnlyRow label="Họ và tên" value={user.fullName} />
          <ReadOnlyRow label="Ngày sinh" value={formatDate(user.dateOfBirth)} />
          <ReadOnlyRow
            label="Số CCCD"
            value={maskTail(user.identity?.number)}
          />
          <p className="profile-caption">
            Muốn sửa các thông tin này cần gửi yêu cầu hiệu chỉnh kèm chứng từ.
          </p>
        </>
      )}

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
            busy || !hasChanges || phoneInvalid || lineEmpty || provinceEmpty
          }
          onClick={() => void save()}
        >
          {busy ? "Đang lưu..." : "Lưu thay đổi"}
        </button>
      </div>
    </PanelShell>
  );
}

function ReadOnlyRow({ label, value }: { label: string; value?: string }) {
  if (!value) return null;
  return (
    <div className="profile-readonly">
      <div>
        <span>{label}</span>
        <strong>{value}</strong>
      </div>
      <FontAwesomeIcon icon={faLock} aria-label="Không thể sửa" />
    </div>
  );
}

export function ChangeEmailPanel({ onBack }: PanelProps) {
  const { user, startEmailChange, confirmEmailChange } = useAuth();
  const { busy, message, error, run, setMessage } = useProfileAction();
  const [newEmail, setNewEmail] = useState("");
  const [currentEmailOtp, setCurrentEmailOtp] = useState("");
  const [newEmailOtp, setNewEmailOtp] = useState("");
  const [restarting, setRestarting] = useState(false);

  const nextEmail = newEmail.trim();
  const showOtpStep = Boolean(user?.pendingEmail) && !restarting;

  const start = async () => {
    const started = await run(() => startEmailChange(nextEmail), {
      success: "Đã gửi mã xác minh đến cả email hiện tại và email mới.",
      badRequest: "Email mới chưa hợp lệ.",
    });
    if (started) {
      setRestarting(false);
      setNewEmail("");
      setCurrentEmailOtp("");
      setNewEmailOtp("");
    }
  };

  const confirm = async () => {
    const confirmed = await run(
      () => confirmEmailChange({ currentEmailOtp, newEmailOtp }),
      {
        success: "Đã đổi email.",
        badRequest: "Mã xác minh không đúng hoặc đã hết hạn.",
      },
    );
    setCurrentEmailOtp("");
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
        mã OTP cho đến khi bạn xác minh xong cả hai địa chỉ.
      </p>
      {showOtpStep ? (
        <>
          <p className="profile-caption">
            Email mới đang chờ: {user?.pendingEmail}
          </p>
          <ProfileInput
            label="Mã gửi đến email hiện tại"
            inputMode="numeric"
            autoComplete="one-time-code"
            value={currentEmailOtp}
            onChange={setCurrentEmailOtp}
          />
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
            disabled={
              busy ||
              !OTP_PATTERN.test(currentEmailOtp) ||
              !OTP_PATTERN.test(newEmailOtp)
            }
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
          <button
            type="button"
            className="profile-action"
            disabled={
              busy ||
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

function FileSourcePicker({
  label,
  accept,
  camera,
  disabled,
  onSelect,
}: {
  label: string;
  accept: string[];
  camera: "user" | "environment";
  disabled: boolean;
  onSelect: (file: File) => void;
}) {
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    // Cho phép chọn lại đúng tệp đó sau khi hủy hoặc gặp lỗi.
    event.target.value = "";
    if (file) onSelect(file);
  };

  return (
    <div className="profile-source-picker">
      <p className="profile-source-label">{label}</p>
      <div className="profile-source-actions">
        <label
          className={`profile-source-button${disabled ? " is-disabled" : ""}`}
        >
          <FontAwesomeIcon icon={faCamera} />
          Mở camera
          <input
            className="sr-only"
            type="file"
            accept="image/*"
            // WebView Android của Tauri chỉ mở camera khi accept đúng là image/*;
            // định dạng thực tế được validateUpload kiểm tra sau khi chụp.
            capture={camera}
            disabled={disabled}
            onChange={handleChange}
          />
        </label>
        <label
          className={`profile-source-button${disabled ? " is-disabled" : ""}`}
        >
          <FontAwesomeIcon icon={faFolderOpen} />
          Chọn tệp
          <input
            className="sr-only"
            type="file"
            accept={accept.join(",")}
            disabled={disabled}
            onChange={handleChange}
          />
        </label>
      </div>
    </div>
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
