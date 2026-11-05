import { type CSSProperties, useSyncExternalStore } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCircleCheck,
  faCircleExclamation,
  faCircleInfo,
  faTriangleExclamation,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { toast, TOAST_DURATION_MS, type ToastTone } from "./toast";
import "./toast.css";

const TONE_ICON = {
  error: faCircleExclamation,
  success: faCircleCheck,
  warning: faTriangleExclamation,
  info: faCircleInfo,
} as const;

const TONE_TITLE: Record<ToastTone, string> = {
  error: "Có lỗi xảy ra",
  success: "Thành công",
  warning: "Cần lưu ý",
  info: "Thông báo",
};

export function Toaster() {
  const current = useSyncExternalStore(toast.subscribe, toast.getSnapshot);
  const onClick = current?.onClick;
  const text = current && (
    <>
      <p className="toast-title">{current.title ?? TONE_TITLE[current.tone]}</p>
      <p className="toast-message">{current.message}</p>
    </>
  );
  return (
    <div className="toast-region" aria-label="Thông báo">
      {current && (
        <div
          key={current.id}
          className={`toast-card toast-card--${current.tone}`}
          style={
            { "--toast-duration": `${TOAST_DURATION_MS}ms` } as CSSProperties
          }
        >
          <span className="toast-icon" aria-hidden="true">
            <FontAwesomeIcon icon={TONE_ICON[current.tone]} />
          </span>
          <div role="alert" aria-atomic="true" className="toast-content">
            {onClick ? (
              <button
                type="button"
                className="toast-action"
                onClick={() => {
                  toast.dismiss();
                  onClick();
                }}
              >
                {text}
              </button>
            ) : (
              text
            )}
          </div>
          <button
            type="button"
            aria-label="Đóng thông báo"
            onClick={toast.dismiss}
            className="toast-close"
          >
            <FontAwesomeIcon icon={faXmark} />
          </button>
          <span className="toast-progress" aria-hidden="true" />
        </div>
      )}
    </div>
  );
}
