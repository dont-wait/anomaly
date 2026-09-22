import { useState } from "react";
import type { TransactionRecord } from "@/features/transactions/model";
import {
  OtpError,
  lookupRecipient,
  submitTransfer,
} from "@/features/transfer/api/transfer";
import {
  MAX_OTP_ATTEMPTS,
  MIN_TRANSFER_AMOUNT,
  type Recipient,
  type SourceAccount,
  type TransferStep,
} from "@/features/transfer/model";
import { formatVnd } from "@/features/transactions/utils/format";

export type TransferResult =
  { ok: true; record: TransactionRecord } | { ok: false; message: string };

const PREVIOUS_STEP: Partial<Record<TransferStep, TransferStep>> = {
  amount: "recipient",
  confirm: "amount",
  otp: "confirm",
};

const toAsciiUpper = (text: string) =>
  text.normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/đ/gi, "D").toUpperCase();

export function useTransferFlow(initialSource: SourceAccount) {
  // Số dư lấy từ /me không tự cập nhật sau khi chuyển, nên luồng tự giữ số dư còn lại
  // để các giao dịch liên tiếp ("Giao dịch mới") kiểm tra đúng số dư.
  const [balance, setBalance] = useState(initialSource.balance);
  const source = { ...initialSource, balance };
  const [step, setStep] = useState<TransferStep>("recipient");
  const [recipient, setRecipient] = useState<Recipient | null>(null);
  const [amount, setAmountValue] = useState(0);
  const [note, setNote] = useState(
    `${toAsciiUpper(source.ownerName)} chuyen tien`,
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [otpAttemptsLeft, setOtpAttemptsLeft] = useState(MAX_OTP_ATTEMPTS);
  const [result, setResult] = useState<TransferResult | null>(null);

  const goTo = (next: TransferStep) => {
    setError("");
    setStep(next);
  };

  const chooseRecipient = (next: Recipient) => {
    setRecipient(next);
    goTo("amount");
  };

  const findRecipient = async (rawAccountNo: string) => {
    const accountNo = rawAccountNo.replace(/\s/g, "");
    if (!/^\d{8,19}$/.test(accountNo)) {
      setError("Số tài khoản gồm 8–19 chữ số.");
      return;
    }
    if (accountNo === source.accountNo) {
      setError("Không thể chuyển tiền đến chính tài khoản của bạn.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const found = await lookupRecipient(accountNo);
      if (found) chooseRecipient(found);
      else setError("Không tìm thấy tài khoản. Vui lòng kiểm tra lại.");
    } finally {
      setBusy(false);
    }
  };

  const setAmount = (value: number) => {
    setError("");
    setAmountValue(value);
  };

  const amountError =
    amount > source.balance ? "Số dư không đủ để thực hiện giao dịch." : "";

  const submitAmount = () => {
    if (amount < MIN_TRANSFER_AMOUNT) {
      setError(`Số tiền tối thiểu là ${formatVnd(MIN_TRANSFER_AMOUNT)}.`);
      return;
    }
    if (amountError) return;
    goTo("confirm");
  };

  const submitOtp = async (otp: string) => {
    if (!recipient) return;
    if (!/^\d{6}$/.test(otp)) {
      setError("Mã OTP gồm 6 chữ số.");
      return;
    }
    setBusy(true);
    setError("");
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
      goTo("result");
    } catch (err) {
      if (!(err instanceof OtpError)) {
        setResult({
          ok: false,
          message: "Hệ thống đang bận. Vui lòng thử lại sau.",
        });
        goTo("result");
        return;
      }
      const left = otpAttemptsLeft - 1;
      setOtpAttemptsLeft(left);
      if (left > 0) {
        setError(`Mã OTP không đúng. Bạn còn ${left} lần thử.`);
      } else {
        setResult({
          ok: false,
          message: `Bạn đã nhập sai OTP ${MAX_OTP_ATTEMPTS} lần. Giao dịch đã bị huỷ.`,
        });
        goTo("result");
      }
    } finally {
      setBusy(false);
    }
  };

  const back = () => {
    const previous = PREVIOUS_STEP[step];
    if (previous) goTo(previous);
  };

  const retry = () => {
    setResult(null);
    setOtpAttemptsLeft(MAX_OTP_ATTEMPTS);
    goTo("confirm");
  };

  const reset = () => {
    setRecipient(null);
    setAmountValue(0);
    setNote(`${toAsciiUpper(source.ownerName)} chuyen tien`);
    setResult(null);
    setOtpAttemptsLeft(MAX_OTP_ATTEMPTS);
    goTo("recipient");
  };

  return {
    step,
    source,
    recipient,
    amount,
    setAmount,
    note,
    setNote,
    error,
    amountError,
    busy,
    result,
    canGoBack: step in PREVIOUS_STEP,
    chooseRecipient,
    findRecipient,
    submitAmount,
    confirm: () => goTo("otp"),
    submitOtp,
    back,
    retry,
    reset,
  };
}

export type TransferFlowState = ReturnType<typeof useTransferFlow>;
