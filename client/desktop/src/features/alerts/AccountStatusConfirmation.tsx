import { useEffect, useRef, useState, type FormEvent } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faLock, faLockOpen } from "@fortawesome/free-solid-svg-icons";

interface Props {
  accountNo: string;
  status: "active" | "locked";
  onConfirm: () => Promise<void>;
  onCancel: () => void;
}

export function AccountStatusConfirmation({
  accountNo,
  status,
  onConfirm,
  onCancel,
}: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const submitting = useRef(false);
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const locking = status === "locked";
  const action = locking ? "Khóa" : "Mở khóa";

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
    setError("");
    try {
      await onConfirm();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Không thể cập nhật tài khoản.",
      );
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      aria-labelledby="account-status-title"
      aria-describedby="account-status-description"
      onCancel={(event) => {
        event.preventDefault();
        if (!submitting.current) onCancel();
      }}
      className="fixed inset-0 m-auto w-[calc(100%-2rem)] max-w-100 rounded-(--admin-radius-lg) border border-(--admin-border) bg-(--admin-surface) p-5 text-(--admin-text) shadow-2xl backdrop:bg-black/60"
    >
      <div
        className={`mx-auto grid h-12 w-12 place-items-center rounded-full ${locking ? "bg-(--admin-danger-soft) text-(--admin-danger)" : "bg-(--admin-primary-soft) text-(--admin-primary)"}`}
      >
        <FontAwesomeIcon
          icon={locking ? faLock : faLockOpen}
          className="text-xl"
          aria-hidden="true"
        />
      </div>
      <h2
        id="account-status-title"
        className="mt-3 text-center text-[16px] font-semibold"
      >
        Xác nhận {action.toLowerCase()} tài khoản
      </h2>
      <p
        id="account-status-description"
        className="mt-2 text-center text-[12px] leading-5 text-(--admin-text-muted)"
      >
        Bạn muốn {action.toLowerCase()} tài khoản{" "}
        <strong className="break-all font-(family-name:--admin-font-mono) text-(--admin-text)">
          {accountNo}
        </strong>
        ?
      </p>
      <form
        onSubmit={(event) => void submit(event)}
        className="mt-5 grid gap-3"
      >
        <label
          className="grid gap-2 text-[12px] text-(--admin-text-secondary)"
          htmlFor="account-status-confirmation"
        >
          <span>
            Nhập chính xác <strong className="text-(--admin-text)">OK</strong>{" "}
            để xác nhận.
          </span>
          <input
            id="account-status-confirmation"
            autoComplete="off"
            spellCheck={false}
            value={confirmation}
            disabled={busy}
            onChange={(event) => setConfirmation(event.target.value)}
            className="h-10 w-full rounded-sm border border-(--admin-border) bg-(--admin-background) px-3 text-[12px] text-(--admin-text) outline-none focus:border-(--admin-primary) disabled:opacity-50"
          />
        </label>
        {error && (
          <p role="alert" className="text-[12px] text-(--admin-danger)">
            {error}
          </p>
        )}
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
            className={`h-10 rounded-sm px-2 text-[12px] font-semibold text-white disabled:cursor-not-allowed disabled:opacity-40 ${locking ? "bg-(--admin-danger)" : "bg-(--admin-primary)"}`}
          >
            {busy ? "Đang xử lý..." : `${action} tài khoản`}
          </button>
        </div>
      </form>
    </dialog>
  );
}
