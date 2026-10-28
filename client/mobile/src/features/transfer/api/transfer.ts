import type { TransactionRecord } from "@/features/transactions/model";
import { ANOMALY_BANK, type Recipient } from "@/features/transfer/model";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { getDeviceId, getPlatform } from "@/shared/lib/device";
import { requestJson } from "@/shared/lib/http";

interface RecipientDto {
  accountNo: string;
  name: string;
  bankCode: string;
}

export interface OtpInfo {
  channel: string;
  maskedDestination: string;
  expiresAt: string;
  attemptsLeft: number;
  resendAvailableAt: string;
}

export interface TransferIntent {
  transferId: string;
  status: "awaiting_otp" | "success" | "failed" | "cancelled";
  recipient: RecipientDto;
  amount: number;
  fee: number;
  note: string;
  otp?: OtpInfo;
}

export interface CreateTransferInput {
  toBankCode: "ANOMALY";
  toAccountNo: string;
  amount: number;
  note: string;
}

interface TransactionDto extends Omit<
  TransactionRecord,
  "createdAt" | "counterparty"
> {
  createdAt: string;
  counterparty: {
    name: string;
    accountNo?: string;
    bank?: string;
    bankCode?: string;
  };
}

export interface RiskCheckPending {
  status: "RISK_CHECK_PENDING";
  transferId: string;
}

const toRecipient = (dto: RecipientDto): Recipient => ({
  accountNo: dto.accountNo,
  name: dto.name,
  bank: ANOMALY_BANK,
});

const toRecord = (dto: TransactionDto): TransactionRecord => ({
  ...dto,
  createdAt: new Date(dto.createdAt),
  counterparty: {
    name: dto.counterparty.name,
    accountNo: dto.counterparty.accountNo,
    bank: dto.counterparty.bank ?? dto.counterparty.bankCode,
  },
});

export async function lookupRecipient(
  token: string,
  accountNo: string,
): Promise<Recipient> {
  const params = new URLSearchParams({ bankCode: "ANOMALY", accountNo });
  const dto = await requestJson<RecipientDto>(
    `${API_ENDPOINTS.ACCOUNT_LOOKUP}?${params}`,
    { token },
  );
  return toRecipient(dto);
}

export function createTransfer(
  token: string,
  idempotencyKey: string,
  input: CreateTransferInput,
): Promise<TransferIntent> {
  return requestJson<TransferIntent>(API_ENDPOINTS.TRANSFERS.CREATE, {
    method: "POST",
    token,
    headers: {
      "Idempotency-Key": idempotencyKey,
      "X-Device-ID": getDeviceId(),
      "X-Platform": getPlatform(),
    },
    body: input,
  });
}

export async function confirmTransfer(
  token: string,
  transferId: string,
  otp: string,
): Promise<TransactionRecord | RiskCheckPending> {
  const dto = await requestJson<TransactionDto | RiskCheckPending>(
    API_ENDPOINTS.TRANSFERS.CONFIRM(transferId),
    { method: "POST", token, body: { otp } },
  );
  if (dto.status === "RISK_CHECK_PENDING") return dto;
  return toRecord(dto);
}

export function resendTransferOtp(
  token: string,
  transferId: string,
): Promise<OtpInfo> {
  return requestJson<OtpInfo>(API_ENDPOINTS.TRANSFERS.RESEND_OTP(transferId), {
    method: "POST",
    token,
  });
}

export async function listRecentRecipients(
  token: string,
  limit = 4,
): Promise<Recipient[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  const response = await requestJson<{
    items: Array<RecipientDto & { lastTransferredAt: string }>;
  }>(`${API_ENDPOINTS.TRANSFERS.RECENT_RECIPIENTS}?${params}`, { token });
  return response.items
    .filter((item) => item.bankCode === ANOMALY_BANK.code)
    .map(toRecipient);
}

interface RecipientContactDto {
  accountNo: string;
  name: string;
  bankCode: string;
}

export async function listRecipientContacts(
  token: string,
  query?: string,
): Promise<Recipient[]> {
  const params = new URLSearchParams();
  if (query?.trim()) params.set("q", query.trim());
  const suffix = params.toString() ? `?${params.toString()}` : "";
  const response = await requestJson<{ items: RecipientContactDto[] }>(
    `${API_ENDPOINTS.CONTACTS.RECIPIENTS}${suffix}`,
    { token },
  );
  return response.items
    .filter((item) => item.bankCode === ANOMALY_BANK.code)
    .map(toRecipient);
}

export async function addRecipientContact(
  token: string,
  recipient: Recipient,
): Promise<Recipient> {
  const response = await requestJson<RecipientContactDto>(
    API_ENDPOINTS.CONTACTS.RECIPIENTS,
    {
      method: "POST",
      token,
      body: { accountNo: recipient.accountNo, bankCode: recipient.bank.code },
    },
  );
  return toRecipient(response);
}

export async function deleteRecipientContact(
  token: string,
  recipient: Recipient,
): Promise<void> {
  await requestJson<void>(
    API_ENDPOINTS.CONTACTS.DELETE_RECIPIENT(
      recipient.bank.code,
      recipient.accountNo,
    ),
    { method: "DELETE", token },
  );
}
