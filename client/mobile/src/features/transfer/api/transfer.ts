import type { TransactionRecord } from "@/features/transactions/model";
import { ANOMALY_BANK, type Recipient } from "@/features/transfer/model";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
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
  status: "awaiting_otp" | "success" | "cancelled";
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

const contactsByOwner = new Map<string, Recipient[]>();
const globalContacts: Recipient[] = [];

const resolveContacts = (ownerAccountNo?: string): Recipient[] => {
  if (ownerAccountNo === undefined) return globalContacts;
  let list = contactsByOwner.get(ownerAccountNo);
  if (!list) {
    list = [];
    contactsByOwner.set(ownerAccountNo, list);
  }
  return list;
};

const sameRecipient = (a: Recipient, b: Recipient) =>
  a.accountNo === b.accountNo && a.bank.code === b.bank.code;

/** Local-only contacts until a contacts API is available. */
export const contactStore = {
  list: (ownerAccountNo?: string): Recipient[] =>
    [...resolveContacts(ownerAccountNo)].sort((a, b) =>
      a.name.localeCompare(b.name, "vi"),
    ),
  has: (recipient: Recipient, ownerAccountNo?: string) =>
    resolveContacts(ownerAccountNo).some((contact) =>
      sameRecipient(contact, recipient),
    ),
  add: (recipient: Recipient, ownerAccountNo?: string) => {
    if (recipient.bank.code !== ANOMALY_BANK.code) return;
    const list = resolveContacts(ownerAccountNo);
    if (!list.some((contact) => sameRecipient(contact, recipient))) {
      list.push(recipient);
    }
  },
  remove: (recipient: Recipient, ownerAccountNo?: string) => {
    const list = resolveContacts(ownerAccountNo);
    const index = list.findIndex((contact) =>
      sameRecipient(contact, recipient),
    );
    if (index >= 0) list.splice(index, 1);
  },
  clear: (ownerAccountNo?: string) => {
    if (ownerAccountNo === undefined) {
      globalContacts.splice(0);
      contactsByOwner.clear();
      return;
    }
    contactsByOwner.delete(ownerAccountNo);
  },
};

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
    headers: { "Idempotency-Key": idempotencyKey },
    body: input,
  });
}

export async function confirmTransfer(
  token: string,
  transferId: string,
  otp: string,
): Promise<TransactionRecord> {
  const dto = await requestJson<TransactionDto>(
    API_ENDPOINTS.TRANSFERS.CONFIRM(transferId),
    { method: "POST", token, body: { otp } },
  );
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
