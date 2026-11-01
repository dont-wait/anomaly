import { type CSSProperties, useSyncExternalStore } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCircleCheck,
  faCircleExclamation,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import { toast, TOAST_DURATION_MS } from "./toast";
import "./toast.css";

export function Toaster() {
  const current = useSyncExternalStore(toast.subscribe, toast.getSnapshot);
  return (
    <div className="toast-region" aria-label="Thông báo">
      {current && (
        <div
          key={current.id}
          className="toast-card"
          style={
            { "--toast-duration": `${TOAST_DURATION_MS}ms` } as CSSProperties
          }
        >
          <span className="toast-icon" aria-hidden="true">
            <FontAwesomeIcon
              icon={
                current.tone === "success" ? faCircleCheck : faCircleExclamation
              }
            />
          </span>
          <div role="alert" aria-atomic="true" className="toast-content">
            <p className="toast-title">
              {current.tone === "success" ? "Thành công" : "Có lỗi xảy ra"}
            </p>
            <p className="toast-message">{current.message}</p>
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
