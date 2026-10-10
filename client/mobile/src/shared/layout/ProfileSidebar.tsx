import { useEffect, useRef, useState } from "react";
import { faRightFromBracket, faXmark } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { Avatar } from "@/shared/ui";

const TRANSITION_MS = 300;

interface ProfileSidebarProps {
  open: boolean;
  name: string;
  onClose: () => void;
  onLogout: () => void;
}

export const ProfileSidebar = ({
  open,
  name,
  onClose,
  onLogout,
}: ProfileSidebarProps) => {
  const [rendered, setRendered] = useState(open);
  const [visible, setVisible] = useState(false);
  const panelRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (open) {
      setRendered(true);
      const frame = requestAnimationFrame(() => setVisible(true));
      return () => cancelAnimationFrame(frame);
    }

    setVisible(false);
    const timer = window.setTimeout(() => setRendered(false), TRANSITION_MS);
    return () => window.clearTimeout(timer);
  }, [open]);

  useEffect(() => {
    if (open && rendered) panelRef.current?.focus();
  }, [open, rendered]);

  useEffect(() => {
    if (!open) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose, open]);

  if (!rendered) return null;

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <button
        type="button"
        aria-label="Đóng hồ sơ"
        onClick={onClose}
        className={`absolute inset-0 bg-black/40 transition-opacity duration-300 motion-reduce:transition-none ${
          visible ? "opacity-100" : "opacity-0"
        }`}
      />
      <aside
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="profile-sidebar-title"
        tabIndex={-1}
        className={`relative flex h-full w-full max-w-sm flex-col bg-white shadow-2xl transition-transform duration-300 ease-out focus:outline-none motion-reduce:transition-none ${
          visible ? "translate-x-0" : "translate-x-full"
        }`}
      >
        <div className="flex items-start justify-between border-b border-gray-100 px-5 py-5">
          <div className="flex items-center gap-3">
            <Avatar name={name} size={48} />
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-gray-500">
                Tài khoản
              </p>
              <h2
                id="profile-sidebar-title"
                className="mt-1 text-base font-bold text-gray-900"
              >
                {name}
              </h2>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Đóng hồ sơ"
            className="flex h-10 w-10 items-center justify-center rounded-full text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900"
          >
            <FontAwesomeIcon icon={faXmark} className="h-5 w-5" />
          </button>
        </div>

        <div className="flex-1" />

        <div className="border-t border-gray-100 p-5">
          <button
            type="button"
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
