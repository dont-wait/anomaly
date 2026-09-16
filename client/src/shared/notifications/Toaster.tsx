import { useSyncExternalStore } from "react";
import { toast } from "./toast";

// Errors remain visible until dismissed so users have time to read them.
export function Toaster() {
  const current = useSyncExternalStore(toast.subscribe, toast.getSnapshot);
  return (
    <div
      className="pointer-events-none fixed inset-x-4 top-4 z-50 mx-auto max-w-md"
      aria-label="Thông báo"
    >
      {current && (
        <div
          key={current.id}
          className="pointer-events-auto flex items-start gap-3 rounded-xl bg-error-container p-4 text-on-error-container shadow-lg"
        >
          <p
            role="alert"
            aria-atomic="true"
            className="min-w-0 flex-1 break-words text-sm"
          >
            {current.message}
          </p>
          <button
            type="button"
            aria-label="Đóng thông báo"
            onClick={toast.dismiss}
            className="shrink-0 rounded px-2 py-1 focus-visible:outline-2"
          >
            ×
          </button>
        </div>
      )}
    </div>
  );
}
