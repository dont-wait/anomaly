interface ToastMessage {
  id: number;
  message: string;
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

export const toast = {
  error(message: string) {
    clearTimeout(dismissTimer);
    current = { id: ++nextId, message };
    dismissTimer = setTimeout(dismiss, TOAST_DURATION_MS);
    listeners.forEach((notify) => notify());
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
