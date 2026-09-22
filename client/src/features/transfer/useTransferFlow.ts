import { useEffect, useRef, useState } from "react";
import type { TransactionRecord } from "@/features/transactions/model";
import { formatVnd } from "@/features/transactions/utils/format";
import {
  OtpError,
  lookupRecipient,
  submitTransfer,
} from "@/features/transfer/api/transfer";
import {
  ANOMALY_BANK,
  MAX_OTP_ATTEMPTS,
  MIN_TRANSFER_AMOUNT,
  OTP_LENGTH,
  type Bank,
  type Recipient,
  type SourceAccount,
} from "@/features/transfer/model";

export type TransferResult =
  { ok: true; record: TransactionRecord } | { ok: false; message: string };

export type LookupState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "found"; recipient: Recipient }
  | { status: "error"; message: string };

/** Sheet trượt lên: `confirm` = tóm tắt + nhập OTP, `result` = biên lai */
export type SheetState = "closed" | "confirm" | "result";

const LOOKUP_DEBOUNCE_MS = 500;
const MIN_ACCOUNT_LENGTH = 6;

const cleanAccountNo = (value: string) => value.replace(/\s/g, "");

export function useTransferFlow(initialSource: SourceAccount) {
  // Số dư lấy từ /me không tự cập nhật sau khi chuyển, nên luồng tự giữ số dư còn lại
  // để các giao dịch liên tiếp ("Giao dịch mới") kiểm tra đúng số dư.
  const [balance, setBalance] = useState(initialSource.balance);
  const source = { ...initialSource, balance };
  const defaultNote = `${source.ownerName.toUpperCase()} chuyen tien`;

  const [bank, setBankValue] = useState<Bank>(ANOMALY_BANK);
  const [accountNo, setAccountNoValue] = useState("");
  const [lookup, setLookup] = useState<LookupState>({ status: "idle" });
  const [amount, setAmountValue] = useState(0);
  const [amountTouched, setAmountTouched] = useState(false);
  const [note, setNote] = useState(defaultNote);

  const [sheet, setSheet] = useState<SheetState>("closed");
  const [otpError, setOtpError] = useState("");
  const [otpAttemptsLeft, setOtpAttemptsLeft] = useState(MAX_OTP_ATTEMPTS);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<TransferResult | null>(null);

  const lookupId = useRef(0);
  const lookupTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(lookupTimer.current), []);

  const runLookup = async (nextBank: Bank, rawAccountNo: string) => {
    clearTimeout(lookupTimer.current);
    const id = ++lookupId.current;
    const value = cleanAccountNo(rawAccountNo);
    if (!value) {
      setLookup({ status: "idle" });
      return;
    }
    if (!/^\d{6,19}$/.test(value)) {
      setLookup({ status: "error", message: "Số tài khoản gồm 6-19 chữ số." });
      return;
    }
    if (nextBank.code === ANOMALY_BANK.code && value === source.accountNo) {
      setLookup({
        status: "error",
        message: "Không thể chuyển tiền đến chính tài khoản của bạn.",
      });
      return;
    }
    setLookup({ status: "loading" });
    try {
      const found = await lookupRecipient(nextBank, value);
      if (id !== lookupId.current) return;
      setLookup(
        found
          ? { status: "found", recipient: found }
          : {
              status: "error",
              message: `Không tìm thấy tài khoản tại ${nextBank.shortName}.`,
            },
      );
    } catch {
      if (id !== lookupId.current) return;
      setLookup({
        status: "error",
        message: "Không tra cứu được tài khoản. Vui lòng thử lại.",
      });
    }
  };

  /** Tự tra cứu sau khi ngừng gõ; số quá ngắn thì chờ tới khi rời ô nhập. */
  const scheduleLookup = (nextBank: Bank, rawAccountNo: string) => {
    clearTimeout(lookupTimer.current);
    lookupId.current++;
    setLookup({ status: "idle" });
    if (cleanAccountNo(rawAccountNo).length >= MIN_ACCOUNT_LENGTH) {
      lookupTimer.current = setTimeout(
        () => void runLookup(nextBank, rawAccountNo),
        LOOKUP_DEBOUNCE_MS,
      );
    }
  };

  const setAccountNo = (value: string) => {
    setAccountNoValue(value);
    scheduleLookup(bank, value);
  };

  const setBank = (next: Bank) => {
    setBankValue(next);
    if (accountNo) void runLookup(next, accountNo);
  };

  /** Gọi khi rời ô số tài khoản — tra cứu ngay, trừ khi đã có kết quả cho đúng số này. */
  const commitAccountNo = () => {
    const value = cleanAccountNo(accountNo);
    const alreadyFound =
      lookup.status === "found" &&
      lookup.recipient.accountNo === value &&
      lookup.recipient.bank.code === bank.code;
    if (alreadyFound || lookup.status === "loading") return;
    void runLookup(bank, accountNo);
  };

  const chooseRecipient = (recipient: Recipient) => {
    clearTimeout(lookupTimer.current);
    lookupId.current++;
    setBankValue(recipient.bank);
    setAccountNoValue(recipient.accountNo);
    setLookup({ status: "found", recipient });
  };

  const setAmount = (value: number) => {
    setAmountTouched(false);
    setAmountValue(value);
  };

  const amountError =
    amount > source.balance
      ? "Số dư không đủ để thực hiện giao dịch."
      : amountTouched && amount < MIN_TRANSFER_AMOUNT
        ? `Số tiền tối thiểu là ${formatVnd(MIN_TRANSFER_AMOUNT)}.`
        : "";

  const recipient = lookup.status === "found" ? lookup.recipient : null;
  const canSubmit =
    recipient !== null && amount > 0 && amount <= source.balance;

  const openConfirm = () => {
    if (amount < MIN_TRANSFER_AMOUNT) {
      setAmountTouched(true);
      return;
    }
    if (!canSubmit) return;
    setOtpError("");
    setSheet("confirm");
  };

  const submitOtp = async (otp: string) => {
    if (!recipient || busy) return;
    if (otp.length !== OTP_LENGTH) {
      setOtpError(`Mã OTP gồm ${OTP_LENGTH} chữ số.`);
      return;
    }
    setBusy(true);
    setOtpError("");
    try {
      const record = await submitTransfer({
        source,
        recipient,
        amount,
        note: note.trim(),
        otp,
      });
      setBalance((current) => current - record.amount - record.fee);
      setResult({ ok: true, record });
      setSheet("result");
    } catch (err) {
      if (!(err instanceof OtpError)) {
        setResult({
          ok: false,
          message: "Hệ thống đang bận. Vui lòng thử lại sau.",
        });
        setSheet("result");
        return;
      }
      const left = otpAttemptsLeft - 1;
      setOtpAttemptsLeft(left);
      if (left > 0) {
        setOtpError(`Mã OTP không đúng. Bạn còn ${left} lần thử.`);
      } else {
        setResult({
          ok: false,
          message: `Bạn đã nhập sai OTP ${MAX_OTP_ATTEMPTS} lần. Giao dịch đã bị huỷ.`,
        });
        setSheet("result");
      }
    } finally {
      setBusy(false);
    }
  };

  const reset = () => {
    clearTimeout(lookupTimer.current);
    lookupId.current++;
    setBankValue(ANOMALY_BANK);
    setAccountNoValue("");
    setLookup({ status: "idle" });
    setAmountValue(0);
    setAmountTouched(false);
    setNote(defaultNote);
    setResult(null);
    setOtpError("");
    setOtpAttemptsLeft(MAX_OTP_ATTEMPTS);
    setSheet("closed");
  };

  /** Đóng sheet: giao dịch đã thành công thì làm mới form, còn lại giữ nguyên để sửa. */
  const closeSheet = () => {
    if (busy) return;
    if (result?.ok) {
      reset();
      return;
    }
    setResult(null);
    setOtpError("");
    setOtpAttemptsLeft(MAX_OTP_ATTEMPTS);
    setSheet("closed");
  };

  const retry = () => {
    setResult(null);
    setOtpError("");
    setOtpAttemptsLeft(MAX_OTP_ATTEMPTS);
    setSheet("confirm");
  };

  return {
    source,
    bank,
    setBank,
    accountNo,
    setAccountNo,
    commitAccountNo,
    lookup,
    recipient,
    chooseRecipient,
    amount,
    setAmount,
    amountError,
    note,
    setNote,
    canSubmit,
    sheet,
    openConfirm,
    closeSheet,
    otpError,
    busy,
    submitOtp,
    result,
    retry,
    reset,
  };
}

export type TransferFlowState = ReturnType<typeof useTransferFlow>;
