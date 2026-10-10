import { useEffect, useRef, useState } from "react";
import type { TransactionRecord } from "@/features/transactions/model";
import { formatVnd } from "@/features/transactions/utils/format";
import {
  confirmTransfer,
  createTransfer,
  lookupRecipient,
  resendTransferOtp,
  type CreateTransferInput,
  type OtpInfo,
} from "@/features/transfer/api/transfer";
import {
  ANOMALY_BANK,
  MIN_TRANSFER_AMOUNT,
  OTP_LENGTH,
  type Recipient,
  type SourceAccount,
} from "@/features/transfer/model";
import { ApiError } from "@/shared/lib/http";

export type TransferResult =
  | { ok: true; record: TransactionRecord }
  | { ok: false; message: string; retryable: boolean };

export type LookupState =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "found"; recipient: Recipient }
  | { status: "error"; message: string };

export type SheetState = "closed" | "confirm" | "result";

const LOOKUP_DEBOUNCE_MS = 500;
const MIN_ACCOUNT_LENGTH = 6;

const cleanAccountNo = (value: string) => value.replace(/\s/g, "");
const isAmbiguous = (error: unknown) =>
  error instanceof ApiError && (error.status === 0 || error.status >= 500);

const errorCode = (error: unknown) => {
  if (
    !(error instanceof ApiError) ||
    !error.body ||
    typeof error.body !== "object"
  ) {
    return undefined;
  }
  const body = error.body as {
    code?: unknown;
    errors?: Array<{ code?: unknown }>;
  };
  return typeof body.code === "string"
    ? body.code
    : typeof body.errors?.[0]?.code === "string"
      ? body.errors[0].code
      : undefined;
};

const attemptsLeftFrom = (error: unknown) => {
  if (
    !(error instanceof ApiError) ||
    !error.body ||
    typeof error.body !== "object"
  ) {
    return undefined;
  }
  const value = (error.body as { attemptsLeft?: unknown }).attemptsLeft;
  return typeof value === "number" ? value : undefined;
};

const createErrorMessage = (code?: string) => {
  switch (code) {
    case "INSUFFICIENT_FUNDS":
      return "Số dư không đủ để thực hiện giao dịch.";
    case "KYC_REQUIRED":
      return "Bạn cần hoàn tất xác minh danh tính trước khi chuyển tiền.";
    case "ACCOUNT_INACTIVE":
      return "Tài khoản hiện không hoạt động.";
    case "SELF_TRANSFER":
      return "Không thể chuyển tiền đến chính tài khoản của bạn.";
    case "ACCOUNT_NOT_FOUND":
      return "Không tìm thấy tài khoản người nhận.";
    case "OTP_DELIVERY_FAILED":
      return "Không thể gửi mã OTP. Giao dịch đã bị huỷ.";
    case "IDEMPOTENCY_CONFLICT":
      return "Thông tin giao dịch đã thay đổi. Vui lòng quay lại và thử lại.";
    default:
      return "Không thể tạo giao dịch. Vui lòng thử lại.";
  }
};

export function useTransferFlow(
  initialSource: SourceAccount,
  token: string,
  onUnauthorized?: () => void,
) {
  const [balance, setBalance] = useState(initialSource.balance);
  const source = { ...initialSource, balance };
  const defaultNote = `${source.ownerName.toUpperCase()} chuyen tien`;

  const [accountNo, setAccountNoValue] = useState("");
  const [lookup, setLookup] = useState<LookupState>({ status: "idle" });
  const [amount, setAmountValue] = useState(0);
  const [amountTouched, setAmountTouched] = useState(false);
  const [note, setNote] = useState(defaultNote);
  const [sheet, setSheet] = useState<SheetState>("closed");
  const [otpError, setOtpError] = useState("");
  const [otpInfo, setOtpInfo] = useState<OtpInfo | null>(null);
  const [resendMessage, setResendMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<TransferResult | null>(null);

  const transferId = useRef<string | null>(null);
  const activeBody = useRef<CreateTransferInput | null>(null);
  const pendingCreate = useRef<{
    key: string;
    body: CreateTransferInput;
  } | null>(null);
  const retryKind = useRef<"create" | "confirm" | null>(null);
  const lookupId = useRef(0);
  const lookupTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(lookupTimer.current), []);

  const runLookup = async (rawAccountNo: string) => {
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
    if (value === source.accountNo) {
      setLookup({
        status: "error",
        message: "Không thể chuyển tiền đến chính tài khoản của bạn.",
      });
      return;
    }
    setLookup({ status: "loading" });
    try {
      const found = await lookupRecipient(token, value);
      if (id === lookupId.current)
        setLookup({ status: "found", recipient: found });
    } catch (error) {
      if (id !== lookupId.current) return;
      if (error instanceof ApiError && error.status === 401) {
        onUnauthorized?.();
        return;
      }
      setLookup({
        status: "error",
        message:
          errorCode(error) === "ACCOUNT_NOT_FOUND"
            ? "Không tìm thấy tài khoản tại AnomalyBank."
            : "Không tra cứu được tài khoản. Vui lòng thử lại.",
      });
    }
  };

  const scheduleLookup = (rawAccountNo: string) => {
    clearTimeout(lookupTimer.current);
    lookupId.current++;
    setLookup({ status: "idle" });
    if (cleanAccountNo(rawAccountNo).length >= MIN_ACCOUNT_LENGTH) {
      lookupTimer.current = setTimeout(
        () => void runLookup(rawAccountNo),
        LOOKUP_DEBOUNCE_MS,
      );
    }
  };

  const setAccountNo = (value: string) => {
    setAccountNoValue(value);
    scheduleLookup(value);
  };

  const commitAccountNo = () => {
    const value = cleanAccountNo(accountNo);
    const alreadyFound =
      lookup.status === "found" && lookup.recipient.accountNo === value;
    if (alreadyFound || lookup.status === "loading") return;
    void runLookup(accountNo);
  };

  const chooseRecipient = (recipient: Recipient) => {
    if (recipient.bank.code !== ANOMALY_BANK.code) return;
    clearTimeout(lookupTimer.current);
    lookupId.current++;
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
    recipient !== null && amount > 0 && amount <= source.balance && !busy;

  const currentBody = (): CreateTransferInput | null =>
    recipient
      ? {
          toBankCode: "ANOMALY",
          toAccountNo: recipient.accountNo,
          amount,
          note: note.trim(),
        }
      : null;

  const openConfirm = async () => {
    if (amount < MIN_TRANSFER_AMOUNT) {
      setAmountTouched(true);
      return;
    }
    const body = currentBody();
    if (!body || busy || amount > source.balance) return;
    const serialized = JSON.stringify(body);

    if (
      transferId.current &&
      activeBody.current &&
      JSON.stringify(activeBody.current) === serialized
    ) {
      setResult(null);
      setSheet("confirm");
      return;
    }

    const pending = pendingCreate.current;
    const request =
      pending && JSON.stringify(pending.body) === serialized
        ? pending
        : { key: crypto.randomUUID(), body };
    pendingCreate.current = request;
    setBusy(true);
    setOtpError("");
    setResendMessage("");
    try {
      const intent = await createTransfer(token, request.key, request.body);
      pendingCreate.current = null;
      if (intent.status !== "awaiting_otp" || !intent.otp) {
        retryKind.current = null;
        setResult({
          ok: false,
          retryable: false,
          message:
            intent.status === "success"
              ? "Giao dịch đã hoàn tất. Vui lòng kiểm tra lịch sử giao dịch."
              : "Giao dịch đã bị huỷ. Vui lòng tạo giao dịch mới.",
        });
        setSheet("result");
        return;
      }
      transferId.current = intent.transferId;
      activeBody.current = request.body;
      setOtpInfo(intent.otp);
      setResult(null);
      setSheet("confirm");
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        onUnauthorized?.();
        return;
      }
      const ambiguous = isAmbiguous(error);
      if (!ambiguous) pendingCreate.current = null;
      retryKind.current = ambiguous ? "create" : null;
      setResult({
        ok: false,
        retryable: ambiguous,
        message: ambiguous
          ? "Chưa xác định được trạng thái giao dịch. Hãy thử lại để kiểm tra."
          : createErrorMessage(errorCode(error)),
      });
      setSheet("result");
    } finally {
      setBusy(false);
    }
  };

  const submitOtp = async (otp: string) => {
    if (!transferId.current || busy) return;
    if (otp.length !== OTP_LENGTH) {
      setOtpError(`Mã OTP gồm ${OTP_LENGTH} chữ số.`);
      return;
    }
    setBusy(true);
    setOtpError("");
    try {
      const record = await confirmTransfer(token, transferId.current, otp);
      if (typeof record.balanceAfter === "number")
        setBalance(record.balanceAfter);
      setResult({ ok: true, record });
      setSheet("closed");
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        onUnauthorized?.();
        return;
      }
      const code = errorCode(error);
      if (code === "INVALID_OTP") {
        const left = attemptsLeftFrom(error);
        if (typeof left === "number") {
          setOtpInfo((current) =>
            current ? { ...current, attemptsLeft: left } : current,
          );
        }
        setOtpError(
          typeof left === "number"
            ? `Mã OTP không đúng. Bạn còn ${left} lần thử.`
            : "Mã OTP không đúng. Vui lòng thử lại.",
        );
        return;
      }

      const terminal =
        code === "OTP_EXPIRED" || code === "OTP_ATTEMPTS_EXCEEDED";
      const ambiguous = isAmbiguous(error);
      retryKind.current = ambiguous ? "confirm" : null;
      if (terminal) {
        transferId.current = null;
        activeBody.current = null;
      }
      setResult({
        ok: false,
        retryable: ambiguous,
        message: terminal
          ? code === "OTP_EXPIRED"
            ? "Mã OTP đã hết hạn. Giao dịch đã bị huỷ."
            : "Bạn đã nhập OTP quá số lần cho phép. Giao dịch đã bị huỷ."
          : code === "INSUFFICIENT_FUNDS"
            ? "Số dư không đủ để hoàn tất giao dịch."
            : ambiguous
              ? "Chưa xác định được kết quả giao dịch. Hãy thử xác nhận lại."
              : "Không thể hoàn tất giao dịch. Vui lòng thử lại sau.",
      });
      setSheet("result");
    } finally {
      setBusy(false);
    }
  };

  const resendOtp = async () => {
    if (!transferId.current || busy || !otpInfo) return;
    if (new Date(otpInfo.resendAvailableAt).getTime() > Date.now()) return;
    setBusy(true);
    setOtpError("");
    setResendMessage("");
    try {
      setOtpInfo(await resendTransferOtp(token, transferId.current));
      setResendMessage("Đã gửi lại mã OTP.");
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        onUnauthorized?.();
        return;
      }
      const code = errorCode(error);
      if (code === "TRANSFER_ALREADY_PROCESSED") {
        transferId.current = null;
        activeBody.current = null;
        retryKind.current = null;
        setResult({
          ok: false,
          retryable: false,
          message: "Giao dịch đã kết thúc, không thể gửi lại OTP.",
        });
        setSheet("result");
      } else if (code !== "OTP_RESEND_TOO_SOON") {
        setResendMessage("Không thể gửi lại OTP. Vui lòng thử lại.");
      }
    } finally {
      setBusy(false);
    }
  };

  const reset = () => {
    clearTimeout(lookupTimer.current);
    lookupId.current++;
    transferId.current = null;
    activeBody.current = null;
    pendingCreate.current = null;
    retryKind.current = null;
    setAccountNoValue("");
    setLookup({ status: "idle" });
    setAmountValue(0);
    setAmountTouched(false);
    setNote(defaultNote);
    setResult(null);
    setOtpError("");
    setOtpInfo(null);
    setResendMessage("");
    setSheet("closed");
  };

  const closeSheet = () => {
    if (busy) return;
    setResult(null);
    setOtpError("");
    setResendMessage("");
    setSheet("closed");
  };

  const retry = () => {
    setResult(null);
    if (retryKind.current === "create") {
      void openConfirm();
    } else if (retryKind.current === "confirm") {
      setSheet("confirm");
    }
  };

  return {
    source,
    bank: ANOMALY_BANK,
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
    otpInfo,
    resendMessage,
    resendOtp,
    busy,
    submitOtp,
    result,
    retry,
    reset,
  };
}

export type TransferFlowState = ReturnType<typeof useTransferFlow>;
