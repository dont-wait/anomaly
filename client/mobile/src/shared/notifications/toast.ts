export type ToastTone = "error" | "success" | "warning" | "info";

export interface ToastOptions {
  /** Ghi đè tiêu đề mặc định theo tone */
  title?: string;
  /** Bấm vào nội dung toast; toast tự đóng sau khi gọi */
  onClick?: () => void;
}

export interface ToastMessage extends ToastOptions {
  id: number;
  message: string;
  tone: ToastTone;
}

export const TOAST_DURATION_MS = 5000;

let current: ToastMessage | null = null;
let nextId = 0;
let dismissTimer: ReturnType<typeof setTimeout> | undefined;
const listeners = new Set<() => void>();

function dismiss() {
  clearTimeout(dismissTimer);
  dismissTimer = undefined;
  current = null;
  listeners.forEach((notify) => notify());
}

function show(message: string, tone: ToastTone, options: ToastOptions = {}) {
  clearTimeout(dismissTimer);
  current = { id: ++nextId, message, tone, ...options };
  dismissTimer = setTimeout(dismiss, TOAST_DURATION_MS);
  listeners.forEach((notify) => notify());
}

export const toast = {
  error(message: string, options?: ToastOptions) {
    show(message, "error", options);
  },
  success(message: string, options?: ToastOptions) {
    show(message, "success", options);
  },
  warning(message: string, options?: ToastOptions) {
    show(message, "warning", options);
  },
  info(message: string, options?: ToastOptions) {
    show(message, "info", options);
  },
  show,
  dismiss,
  getSnapshot: () => current,
  subscribe(notify: () => void) {
    listeners.add(notify);
    return () => {
      listeners.delete(notify);
    };
  },
};
