import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { CloseIcon } from "@/shared/icons";

const EXIT_MS = 200;
const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

interface BottomSheetProps {
  open: boolean;
  title: string;
  onClose: () => void;
  /** `false` khi đang xử lý — ẩn nút đóng, chặn Esc và chạm nền */
  dismissible?: boolean;
  children: ReactNode;
}

/**
 * Cửa sổ trượt từ dưới lên (mobile-first). Giữ focus bên trong khi mở,
 * trả focus về phần tử đã mở nó khi đóng.
 */
export const BottomSheet = ({
  open,
  title,
  onClose,
  dismissible = true,
  children,
}: BottomSheetProps) => {
  const titleId = useId();
  const panel = useRef<HTMLDivElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  const [rendered, setRendered] = useState(open);
  const [entered, setEntered] = useState(false);

  // Mount ngay khi mở; unmount sau khi animation đóng chạy xong.
  if (open && !rendered) setRendered(true);
  const visible = open && entered;

  useEffect(() => {
    if (open) {
      returnFocus.current = document.activeElement as HTMLElement | null;
      // Chờ 2 frame để trình duyệt vẽ trạng thái ban đầu rồi mới chạy transition.
      let second = 0;
      const first = requestAnimationFrame(() => {
        second = requestAnimationFrame(() => setEntered(true));
      });
      return () => {
        cancelAnimationFrame(first);
        cancelAnimationFrame(second);
      };
    }
    const timer = setTimeout(() => {
      setRendered(false);
      setEntered(false);
      returnFocus.current?.focus();
    }, EXIT_MS);
    return () => clearTimeout(timer);
  }, [open]);

  // Đưa focus vào sheet nếu nội dung chưa tự focus (vd ô OTP có autoFocus).
  useEffect(() => {
    if (!open || !rendered) return;
    const node = panel.current;
    if (node && !node.contains(document.activeElement)) {
      (node.querySelector<HTMLElement>(FOCUSABLE) ?? node).focus();
    }
  }, [open, rendered]);

  if (!rendered) return null;

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape" && dismissible) {
      event.stopPropagation();
      onClose();
      return;
    }
    if (event.key !== "Tab" || !panel.current) return;
    const items = [...panel.current.querySelectorAll<HTMLElement>(FOCUSABLE)];
    if (items.length === 0) return;
    const first = items[0];
    const last = items[items.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center">
      <div
        aria-hidden="true"
        onClick={dismissible ? onClose : undefined}
        className={`absolute inset-0 bg-inverse-surface/55 transition-opacity duration-300 motion-reduce:transition-none ${
          visible ? "opacity-100" : "opacity-0"
        }`}
      />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        onKeyDown={onKeyDown}
        className={`relative flex max-h-[92dvh] w-full max-w-md flex-col rounded-t-3xl bg-surface-container-lowest shadow-2xl shadow-inverse-surface/30 transition-transform focus:outline-none motion-reduce:transition-none ${
          visible
            ? "translate-y-0 duration-300 ease-out"
            : "translate-y-full duration-200 ease-in"
        }`}
      >
        <div aria-hidden="true" className="flex justify-center pt-3">
          <span className="h-1.5 w-10 rounded-full bg-outline-variant" />
        </div>
        <div className="flex items-center gap-2 px-5 pt-2 pb-1">
          <h2 id={titleId} className="flex-1 text-lg font-bold text-on-surface">
            {title}
          </h2>
          {dismissible && (
            <button
              type="button"
              onClick={onClose}
              aria-label="Đóng"
              className="-mr-2 flex h-11 w-11 items-center justify-center rounded-full text-on-surface-variant transition-colors hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
            >
              <CloseIcon className="h-5 w-5" />
            </button>
          )}
        </div>
        <div className="overflow-y-auto overscroll-contain px-5 pt-2 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
          {children}
        </div>
      </div>
    </div>
  );
};
