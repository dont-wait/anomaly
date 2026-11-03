import { useEffect, useRef, useState, type FormEvent } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCircleCheck,
  faClipboardCheck,
  faRotateLeft,
} from "@fortawesome/free-solid-svg-icons";

interface Props {
  handled: boolean;
  onConfirm: () => Promise<void>;
  onCancel: () => void;
}

export function AlertHandlingConfirmation({
  handled,
  onConfirm,
  onCancel,
}: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const submitting = useRef(false);
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [completed, setCompleted] = useState(false);
  const action = handled ? "Đánh dấu đã xử lý" : "Hoàn tác xử lý";

  useEffect(() => {
    const element = dialog.current;
    element?.showModal();
    return () => element?.close();
  }, []);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (confirmation !== "OK" || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    try {
      await onConfirm();
      setCompleted(true);
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      aria-labelledby="alert-handling-title"
      aria-describedby="alert-handling-description"
      onCancel={(event) => {
        event.preventDefault();
        if (!submitting.current) onCancel();
      }}
      className="fixed inset-0 m-auto w-[calc(100%-2rem)] max-w-100 rounded-(--admin-radius-lg) border border-(--admin-border) bg-(--admin-surface) p-5 text-(--admin-text) shadow-2xl backdrop:bg-black/60"
    >
      <div className="mx-auto grid h-12 w-12 place-items-center rounded-full bg-(--admin-primary-soft) text-(--admin-primary)">
        <FontAwesomeIcon
          icon={
            completed
              ? faCircleCheck
              : handled
                ? faClipboardCheck
                : faRotateLeft
          }
          className="text-xl"
          aria-hidden="true"
        />
      </div>
      {completed ? (
        <>
          <h2
            id="alert-handling-title"
            className="mt-3 text-center text-[16px] font-semibold"
          >
            {handled ? "Đã đánh dấu cảnh báo đã xử lý" : "Đã hoàn tác cảnh báo"}
          </h2>
          <p
            id="alert-handling-description"
            className="mt-2 text-center text-[12px] leading-5 text-(--admin-text-muted)"
          >
            {handled
              ? "Cảnh báo sẽ không còn xuất hiện trong danh sách chưa xử lý."
              : "Cảnh báo đã xuất hiện lại trong danh sách chưa xử lý."}
          </p>
          <button
            type="button"
            onClick={onCancel}
            className="mt-5 h-10 w-full rounded-sm bg-(--admin-primary) text-[12px] font-semibold text-white hover:bg-(--admin-primary-strong)"
          >
            Đóng
          </button>
        </>
      ) : (
        <>
          <h2
            id="alert-handling-title"
            className="mt-3 text-center text-[16px] font-semibold"
          >
            Xác nhận {action.toLowerCase()}
          </h2>
          <p
            id="alert-handling-description"
            className="mt-2 text-center text-[12px] leading-5 text-(--admin-text-muted)"
          >
            Bạn có chắc muốn {action.toLowerCase()} cảnh báo này không?
          </p>
          <form
            onSubmit={(event) => void submit(event)}
            className="mt-5 grid gap-3"
          >
            <label
              className="grid gap-2 text-[12px] text-(--admin-text-secondary)"
              htmlFor="alert-handling-confirmation"
            >
              <span>
                Nhập chính xác{" "}
                <strong className="text-(--admin-text)">OK</strong> để xác nhận.
              </span>
              <input
                id="alert-handling-confirmation"
                autoComplete="off"
                spellCheck={false}
                placeholder="Nhập OK để xác nhận"
                value={confirmation}
                disabled={busy}
                onChange={(event) => setConfirmation(event.target.value)}
                className="h-10 w-full rounded-sm border border-(--admin-border) bg-(--admin-background) px-3 text-[12px] text-(--admin-text) outline-none focus:border-(--admin-primary) disabled:opacity-50"
              />
            </label>
            <div className="mt-1 grid grid-cols-2 gap-2">
              <button
                type="button"
                disabled={busy}
                onClick={onCancel}
                className="h-10 rounded-sm border border-(--admin-border) text-[12px] font-semibold hover:bg-(--admin-surface-hover) disabled:opacity-50"
              >
                Hủy
              </button>
              <button
                type="submit"
                disabled={confirmation !== "OK" || busy}
                className="h-10 rounded-sm bg-(--admin-primary) px-2 text-[12px] font-semibold text-white disabled:cursor-not-allowed disabled:opacity-40"
              >
                {busy ? "Đang xử lý..." : action}
              </button>
            </div>
          </form>
        </>
      )}
    </dialog>
  );
}
