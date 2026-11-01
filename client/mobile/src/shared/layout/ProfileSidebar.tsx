import { useEffect, useRef, useState, type ReactNode } from "react";
import { faRightFromBracket, faXmark } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { Avatar } from "@/shared/ui";
import { useAuth } from "@/features/auth/useAuth";
import { API_BASE_URL } from "@/shared/constants";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import "./profile-sidebar.css";

type ProfileSection = "overview" | "edit" | "password" | "avatar" | "documents";

interface ProfileSidebarProps {
  open: boolean;
  name: string;
  onClose: () => void;
  onLogout: () => void;
  token: string | null;
}

export const ProfileSidebar = ({
  open,
  name,
  onClose,
  onLogout,
  token,
}: ProfileSidebarProps) => {
  const { user, updateProfile, changePassword, uploadProfileAvatar, uploadProfileDocument } = useAuth();
  const panelRef = useRef<HTMLElement>(null);
  const [section, setSection] = useState<ProfileSection>("overview");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [phone, setPhone] = useState(user?.phone ?? "");
  const [email, setEmail] = useState(user?.email ?? "");
  const [address, setAddress] = useState(user?.address?.line ?? "");
  const [provinceCode, setProvinceCode] = useState(user?.address?.provinceCode ?? "");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [documentType, setDocumentType] = useState("INCOME_PROOF");
  const [documentFile, setDocumentFile] = useState<File | null>(null);

  const closeProfile = () => {
    setSection("overview");
    setMessage("");
    setError("");
    setAvatarFile(null);
    setDocumentFile(null);
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
    setPhone(user?.phone ?? "");
    setEmail(user?.email ?? "");
    setAddress(user?.address?.line ?? "");
    setProvinceCode(user?.address?.provinceCode ?? "");
    onClose();
  };

  const run = async (action: () => Promise<unknown>, success: string) => {
    setBusy(true); setError(""); setMessage("");
    try { await action(); setMessage(success); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Không thể cập nhật hồ sơ."); }
    finally { setBusy(false); }
  };

  const validateUpload = (file: File | null, allowedTypes: string[], maxBytes: number) => {
    if (!file) return "Chọn một tệp trước khi tải lên.";
    if (file.size <= 0 || file.size > maxBytes) return `Tệp phải nhỏ hơn ${Math.round(maxBytes / (1024 * 1024))} MiB.`;
    if (!allowedTypes.includes(file.type)) return "Định dạng tệp không được hỗ trợ.";
    return "";
  };

  const downloadUrl = (mediaId: string) => `${API_BASE_URL.replace(/\/+$/, "")}${API_ENDPOINTS.AUTH.PROFILE_MEDIA(mediaId)}`;
  const avatarPreviewUrl = avatarFile ? URL.createObjectURL(avatarFile) : "";
  const downloadMedia = async (mediaId: string) => {
    if (!token) throw new Error("Phiên đăng nhập đã hết hạn.");
    const response = await fetch(downloadUrl(mediaId), { headers: { Authorization: `Bearer ${token}` } });
    if (!response.ok) throw new Error(`Không tải được tệp (mã ${response.status}).`);
    const objectUrl = URL.createObjectURL(await response.blob());
    const anchor = document.createElement("a");
    anchor.href = objectUrl;
    anchor.download = mediaId;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1000);
  };

  useEffect(() => {
    if (!open) return;

    panelRef.current?.focus();
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose, open]);

  return (
    <div
      aria-hidden={!open}
      className={`fixed inset-0 z-50 flex justify-end ${
        open ? "pointer-events-auto" : "pointer-events-none"
      }`}
    >
      <button
        type="button"
        aria-label="Đóng hồ sơ"
        tabIndex={open ? 0 : -1}
        onClick={closeProfile}
        className={`absolute inset-0 bg-black/40 transition-opacity duration-300 motion-reduce:transition-none ${
          open ? "opacity-100" : "opacity-0"
        }`}
      />
      <aside
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="profile-sidebar-title"
        tabIndex={-1}
        className={`relative flex h-full w-full max-w-sm flex-col bg-white shadow-2xl transition-transform duration-300 ease-out focus:outline-none motion-reduce:transition-none ${
          open ? "translate-x-0" : "translate-x-full"
        }`}
      >
        <div className="profile-sidebar-header">
          <div className="flex items-center gap-3">
            <Avatar name={user?.fullName || name} src={user?.avatar && token ? downloadUrl(user.avatar.mediaId) : undefined} size={52} />
            <div>
              <p className="profile-overline">
                HỒ SƠ CÁ NHÂN
              </p>
              <h2
                id="profile-sidebar-title"
                className="mt-1 text-base font-bold text-[#00236f]"
              >
                {name}
              </h2>
            </div>
          </div>
          <button
            type="button"
            tabIndex={open ? 0 : -1}
            onClick={closeProfile}
            aria-label="Đóng hồ sơ"
            className="profile-close"
          >
            <FontAwesomeIcon icon={faXmark} className="h-5 w-5" />
          </button>
        </div>

        <div className="profile-sidebar-content">
          {section === "overview" && <>
            <section className="profile-card profile-account-card">
              <span className="profile-chip">{user?.isVerify ? "Đã xác minh" : "Chưa xác minh"}</span>
              <p className="profile-overline mt-3">TÀI KHOẢN THANH TOÁN</p>
              <strong className="profile-account-number">•••• {user?.accountNo.slice(-4) ?? "----"}</strong>
              <p className="profile-caption">{user?.email}</p>
            </section>
            <section className="profile-card">
              <div className="profile-card-heading"><h3>Thông tin cá nhân</h3><button onClick={() => setSection("edit")}>Chỉnh sửa</button></div>
              <ProfileRow label="Họ và tên" value={user?.fullName || name} />
              <ProfileRow label="Ngày sinh" value={user?.dateOfBirth ? new Date(user.dateOfBirth).toLocaleDateString("vi-VN") : "Chưa cập nhật"} />
              <ProfileRow label="Điện thoại" value={user?.phone || "Chưa cập nhật"} />
              <ProfileRow label="Email" value={user?.email || "Chưa cập nhật"} />
              <ProfileRow label="Địa chỉ" value={[user?.address?.line, user?.address?.provinceCode].filter(Boolean).join(", ") || "Chưa cập nhật"} />
            </section>
            <section className="profile-card">
              <div className="profile-card-heading"><h3>Giấy tờ đã lưu</h3><button onClick={() => setSection("documents")}>Quản lý</button></div>
              <ProfileRow label="CCCD mặt trước" value={user?.documents?.some((doc) => doc.type === "IDENTITY_FRONT") ? "Đã lưu · đã xác minh" : "Chưa có"} />
              <ProfileRow label="CCCD mặt sau" value={user?.documents?.some((doc) => doc.type === "IDENTITY_BACK") ? "Đã lưu · đã xác minh" : "Chưa có"} />
              <p className="profile-caption">CCCD đã xác minh chỉ xem/tải xuống tại đây. Chỉnh sửa cần quy trình xác minh lại.</p>
            </section>
            <button className="profile-action" onClick={() => setSection("avatar")}>Đổi ảnh đại diện</button>
            <button className="profile-action profile-action-secondary" onClick={() => setSection("password")}>Đổi mật khẩu</button>
          </>}

          {section === "edit" && <ProfileForm title="Cập nhật thông tin" onBack={() => setSection("overview")} busy={busy}>
            <ProfileInput label="Số điện thoại" value={phone} onChange={setPhone} type="tel" />
            <ProfileInput label="Email" value={email} onChange={setEmail} type="email" />
            <ProfileInput label="Địa chỉ" value={address} onChange={setAddress} />
            <ProfileInput label="Tỉnh/thành" value={provinceCode} onChange={setProvinceCode} />
            {user?.identity?.number && <p className="profile-note">CCCD {user.identity.number} đã xác minh và không thể sửa tại đây. Cần yêu cầu hiệu chỉnh kèm chứng từ.</p>}
            <button className="profile-action" disabled={busy || !email.includes("@") || (phone !== "" && !/^\+?[0-9]{8,16}$/.test(phone))} onClick={() => void run(() => updateProfile({ phone, email, address: { line: address, provinceCode } }), "Đã cập nhật thông tin.")}>{busy ? "Đang lưu..." : "Lưu thay đổi"}</button>
          </ProfileForm>}

          {section === "password" && <ProfileForm title="Đổi mật khẩu" onBack={() => setSection("overview")} busy={busy}>
            <ProfileInput label="Mật khẩu hiện tại" type="password" autoComplete="current-password" value={currentPassword} onChange={setCurrentPassword} />
            <ProfileInput label="Mật khẩu mới" type="password" autoComplete="new-password" value={newPassword} onChange={setNewPassword} />
            <ProfileInput label="Xác nhận mật khẩu mới" type="password" autoComplete="new-password" value={confirmPassword} onChange={setConfirmPassword} />
            <p className="profile-note">Mật khẩu mới cần ít nhất 8 ký tự. Phiên đăng nhập khác sẽ hết hạn theo thời hạn token hiện tại.</p>
            <button className="profile-action" disabled={busy || !currentPassword || newPassword.length < 8 || newPassword !== confirmPassword} onClick={() => void run(async () => { await changePassword({ currentPassword, newPassword }); setCurrentPassword(""); setNewPassword(""); setConfirmPassword(""); }, "Đã đổi mật khẩu.")}>{busy ? "Đang cập nhật..." : "Cập nhật mật khẩu"}</button>
          </ProfileForm>}

          {section === "avatar" && <ProfileForm title="Đổi ảnh đại diện" onBack={() => setSection("overview")} busy={busy}>
            <p className="profile-note">Chọn ảnh JPEG, PNG hoặc WebP, tối đa 5 MiB. Ảnh sẽ hiển thị dạng tròn.</p>
            {avatarPreviewUrl && <img className="profile-image-preview" src={avatarPreviewUrl} alt="Xem trước ảnh đại diện" />}
            <label className="profile-source-label">Chụp ảnh hoặc chọn ảnh
              <input className="profile-file" type="file" accept="image/jpeg,image/png,image/webp" capture="user" onChange={(event) => setAvatarFile(event.target.files?.[0] ?? null)} />
            </label>
            <button className="profile-action" disabled={busy || Boolean(validateUpload(avatarFile, ["image/jpeg", "image/png", "image/webp"], 5 * 1024 * 1024))} onClick={() => avatarFile && void run(() => uploadProfileAvatar(avatarFile), "Đã cập nhật ảnh đại diện.")}>{busy ? "Đang tải ảnh..." : "Lưu ảnh"}</button>
          </ProfileForm>}

          {section === "documents" && <ProfileForm title="Chứng từ cá nhân" onBack={() => setSection("overview")} busy={busy}>
            <p className="profile-note">Chứng từ bổ sung tùy theo dịch vụ yêu cầu. PDF/JPEG/PNG, tối đa 10 MiB mỗi tệp.</p>
            {(user?.documents ?? []).map((document) => <div className="profile-document" key={document.mediaId}><div><strong>{documentLabel(document.type)}</strong><p>{document.status} · {document.fileName}</p></div><button onClick={() => void run(() => downloadMedia(document.mediaId), "Đã tải chứng từ.")}>Tải xuống</button></div>)}
            {user?.idCardFrontUrl && <div className="profile-document"><strong>CCCD mặt trước · đã xác minh</strong><button onClick={() => void run(() => downloadMedia("kyc-identity-front"), "Đã tải CCCD.")}>Tải xuống</button></div>}
            {user?.idCardBackUrl && <div className="profile-document"><strong>CCCD mặt sau · đã xác minh</strong><button onClick={() => void run(() => downloadMedia("kyc-identity-back"), "Đã tải CCCD.")}>Tải xuống</button></div>}
            <select className="profile-input" value={documentType} onChange={(event) => setDocumentType(event.target.value)}><option value="INCOME_PROOF">Chứng từ thu nhập</option><option value="ADDRESS_PROOF">Chứng từ địa chỉ</option></select>
            <label className="profile-source-label">Chụp chứng từ hoặc chọn tệp
              <input className="profile-file" type="file" accept="application/pdf,image/jpeg,image/png" capture="environment" onChange={(event) => setDocumentFile(event.target.files?.[0] ?? null)} />
            </label>
            <button className="profile-action" disabled={busy || Boolean(validateUpload(documentFile, ["application/pdf", "image/jpeg", "image/png"], 10 * 1024 * 1024))} onClick={() => documentFile && void run(() => uploadProfileDocument(documentType, documentFile), "Đã tải chứng từ lên hồ sơ.")}>{busy ? "Đang tải chứng từ..." : "Tải chứng từ lên"}</button>
          </ProfileForm>}

          {message && <p className="profile-message" role="status">{message}</p>}
          {error && <p className="profile-error" role="alert">{error}</p>}
        </div>

        <div className="profile-sidebar-footer">
          <button
            type="button"
            tabIndex={open ? 0 : -1}
            onClick={onLogout}
            className="flex w-full items-center justify-center gap-3 rounded-2xl bg-red-50 px-5 py-3.5 text-base font-semibold text-red-600 transition-colors hover:bg-red-100 focus-visible:ring-2 focus-visible:ring-red-400 focus-visible:outline-none"
          >
            <FontAwesomeIcon icon={faRightFromBracket} className="h-4 w-4" />
            Đăng xuất
          </button>
        </div>
      </aside>
    </div>
  );
};

function ProfileRow({ label, value }: { label: string; value: string }) {
  return <div className="profile-row"><span>{label}</span><strong>{value}</strong></div>;
}

function ProfileForm({ title, onBack, busy, children }: { title: string; onBack: () => void; busy: boolean; children: ReactNode }) {
  return <section className="profile-card profile-form">
    <button className="profile-back" disabled={busy} onClick={onBack}>← Hồ sơ của tôi</button>
    <h3>{title}</h3>
    {children}
  </section>;
}

function ProfileInput({ label, value, onChange, type = "text", autoComplete }: { label: string; value: string; onChange: (value: string) => void; type?: string; autoComplete?: string }) {
  return <label className="profile-field">{label}<input className="profile-input" type={type} autoComplete={autoComplete} value={value} onChange={(event) => onChange(event.target.value)} /></label>;
}

function documentLabel(type: string) {
  switch (type) {
    case "IDENTITY_FRONT": return "CCCD mặt trước";
    case "IDENTITY_BACK": return "CCCD mặt sau";
    case "INCOME_PROOF": return "Chứng từ thu nhập";
    case "ADDRESS_PROOF": return "Chứng từ địa chỉ";
    case "LIVENESS_VIDEO": return "Video xác minh";
    default: return type;
  }
}
