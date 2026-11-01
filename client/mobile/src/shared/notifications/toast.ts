interface ToastMessage {
  id: number;
  message: string;
  tone: "error" | "success";
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

function show(message: string, tone: ToastMessage["tone"]) {
  clearTimeout(dismissTimer);
  current = { id: ++nextId, message, tone };
  dismissTimer = setTimeout(dismiss, TOAST_DURATION_MS);
  listeners.forEach((notify) => notify());
}

export const toast = {
  error(message: string) {
    show(message, "error");
  },
  success(message: string) {
    show(message, "success");
  },
  dismiss,
  getSnapshot: () => current,
  subscribe(notify: () => void) {
    listeners.add(notify);
    return () => {
      listeners.delete(notify);
    };
  },
};
