interface ToastMessage {
  id: number;
  message: string;
}

let current: ToastMessage | null = null;
let nextId = 0;
const listeners = new Set<() => void>();

export const toast = {
  error(message: string) {
    current = { id: ++nextId, message };
    listeners.forEach((notify) => notify());
  },
  dismiss() {
    current = null;
    listeners.forEach((notify) => notify());
  },
  getSnapshot: () => current,
  subscribe(notify: () => void) {
    listeners.add(notify);
    return () => {
      listeners.delete(notify);
    };
  },
};
