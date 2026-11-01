import { useCallback, useEffect, useRef, useState } from "react";
import {
  faCamera,
  faRightFromBracket,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { Avatar } from "@/shared/ui";
import { useAuth } from "@/features/auth/useAuth";
import {
  AvatarPanel,
  ChangeEmailPanel,
  ChangePasswordPanel,
  DocumentsPanel,
  EditProfilePanel,
} from "@/features/profile/ProfilePanels";
import {
  documentLabel,
  formatDate,
  maskTail,
} from "@/features/profile/profileHelpers";
import { provinceName } from "@/features/profile/provinces";
import {
  useProfileDocuments,
  useProfileImage,
} from "@/features/profile/useProfileData";
import "./profile-sidebar.css";

type ProfileSection =
  "overview" | "edit" | "email" | "password" | "avatar" | "documents";

const NOT_SET = "Chưa cập nhật";

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
  const { user } = useAuth();
  const panelRef = useRef<HTMLElement>(null);
  const [section, setSection] = useState<ProfileSection>("overview");
  const documents = useProfileDocuments(open, token);
  const avatarUrl = useProfileImage(token, user?.avatar?.mediaId);
  const [notice, setNotice] = useState("");
  const backToOverview = () => setSection("overview");
  const openSection = (next: ProfileSection) => {
    setNotice("");
    setSection(next);
  };

  // Các panel bị unmount nên mật khẩu, OTP và tệp đã chọn không còn trong bộ nhớ.
  const closeProfile = useCallback(() => {
    setNotice("");
    setSection("overview");
    onClose();
  }, [onClose]);

  useEffect(() => {
    if (!open) return;

    panelRef.current?.focus();
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeProfile();
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [closeProfile, open]);

  return (
    <div
      aria-hidden={!open}
      inert={!open}
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
            <button
              type="button"
              className="profile-avatar-button"
              aria-label="Đổi ảnh đại diện"
              aria-current={section === "avatar" ? "page" : undefined}
              onClick={() => openSection("avatar")}
            >
              <Avatar name={user?.fullName || name} src={avatarUrl} size={52} />
              <span className="profile-avatar-badge" aria-hidden="true">
                <FontAwesomeIcon icon={faCamera} />
              </span>
            </button>
            <div>
              <p className="profile-overline">HỒ SƠ CÁ NHÂN</p>
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
          {section === "overview" && (
            <>
              {notice && (
                <p className="profile-message" role="status">
                  {notice}
                </p>
              )}
              <section className="profile-card profile-account-card">
                <span className="profile-chip">
                  {user?.isVerify ? "Đã xác minh" : "Chưa xác minh"}
                </span>
                <p className="profile-overline mt-3">TÀI KHOẢN THANH TOÁN</p>
                <strong className="profile-account-number">
                  {maskTail(user?.accountNo) || "•••• ----"}
                </strong>
                <p className="profile-caption">{user?.email}</p>
              </section>
              <section className="profile-card">
                <div className="profile-card-heading">
                  <h3>Thông tin cá nhân</h3>
                  <button type="button" onClick={() => openSection("edit")}>
                    Cập nhật thông tin
                  </button>
                </div>
                <ProfileRow label="Họ và tên" value={user?.fullName || name} />
                <ProfileRow
                  label="Ngày sinh"
                  value={formatDate(user?.dateOfBirth) || NOT_SET}
                />
                <ProfileRow label="Điện thoại" value={user?.phone || NOT_SET} />
                <ProfileRow label="Email" value={user?.email || NOT_SET} />
                {user?.pendingEmail && (
                  <ProfileRow
                    label="Email mới · chờ xác minh"
                    value={user.pendingEmail}
                  />
                )}
                <ProfileRow
                  label="Địa chỉ"
                  value={
                    [
                      user?.address?.line,
                      provinceName(user?.address?.provinceCode),
                    ]
                      .filter(Boolean)
                      .join(", ") || NOT_SET
                  }
                />
              </section>
              {user?.identity && (
                <section className="profile-card">
                  <div className="profile-card-heading">
                    <h3>Thông tin định danh</h3>
                  </div>
                  <ProfileRow
                    label="Số CCCD"
                    value={maskTail(user.identity.number) || NOT_SET}
                  />
                  <ProfileRow
                    label="Ngày cấp"
                    value={formatDate(user.identity.issuedDate) || NOT_SET}
                  />
                  <ProfileRow
                    label="Nơi cấp"
                    value={user.identity.issuedPlace || NOT_SET}
                  />
                  <ProfileRow
                    label="Địa chỉ thường trú"
                    value={user.identity.permanentAddress || NOT_SET}
                  />
                </section>
              )}
              <section className="profile-card">
                <div className="profile-card-heading">
                  <h3>Giấy tờ đã lưu</h3>
                  <button
                    type="button"
                    onClick={() => openSection("documents")}
                  >
                    Quản lý
                  </button>
                </div>
                {documents.loading && documents.slots.length === 0 && (
                  <p className="profile-caption">Đang tải chứng từ...</p>
                )}
                {documents.error && (
                  <p className="profile-caption" role="alert">
                    {documents.error}
                  </p>
                )}
                {documents.slots.map((slot) => (
                  <ProfileRow
                    key={slot.type}
                    label={documentLabel(slot.type)}
                    value={
                      slot.document
                        ? `Đã lưu · ${formatDate(slot.document.uploadedAt)}`
                        : slot.required
                          ? "Bắt buộc · chưa có"
                          : "Chưa có"
                    }
                  />
                ))}
              </section>
              <button
                type="button"
                className="profile-action profile-action-secondary"
                onClick={() => openSection("password")}
              >
                Đổi mật khẩu
              </button>
            </>
          )}
          {section === "edit" && (
            <EditProfilePanel
              onBack={backToOverview}
              onChangeEmail={() => openSection("email")}
              onSaved={(submitted) => {
                setNotice(
                  submitted
                    ? "Đã gửi yêu cầu. Thông tin đổi sau khi ngân hàng duyệt."
                    : "Đã cập nhật thông tin.",
                );
                backToOverview();
              }}
            />
          )}
          {section === "email" && <ChangeEmailPanel onBack={backToOverview} />}
          {section === "password" && (
            <ChangePasswordPanel onBack={backToOverview} />
          )}
          {section === "avatar" && <AvatarPanel onBack={backToOverview} />}
          {section === "documents" && (
            <DocumentsPanel onBack={backToOverview} documents={documents} />
          )}
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
  return (
    <div className="profile-row">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
