import { transactionStore } from "@/features/transactions";
import type { TransactionRecord } from "@/features/transactions/model";
import type { Recipient, SourceAccount } from "@/features/transfer/model";

export const MOCK_LATENCY_MS = 400;
export const DEMO_OTP = "123456";

const BANK = "AnomalyBank";
const directory: Recipient[] = [
  { accountNo: "99999180147", name: "TRAN THI BICH", bank: BANK },
  { accountNo: "99999180233", name: "LE VAN CUONG", bank: BANK },
  { accountNo: "99999180389", name: "NGUYEN MINH DUC", bank: BANK },
  { accountNo: "99999180412", name: "PHAM THU HA", bank: BANK },
  { accountNo: "99999180556", name: "VO HOANG NAM", bank: BANK },
];

const delay = () =>
  new Promise((resolve) => setTimeout(resolve, MOCK_LATENCY_MS));

export const recentRecipients: Recipient[] = directory.slice(0, 4);

export async function lookupRecipient(
  accountNo: string,
): Promise<Recipient | null> {
  await delay();
  return directory.find((r) => r.accountNo === accountNo) ?? null;
}

export class OtpError extends Error {}

export interface TransferInput {
  source: SourceAccount;
  recipient: Recipient;
  amount: number;
  note: string;
  otp: string;
}

export async function submitTransfer(
  input: TransferInput,
): Promise<TransactionRecord> {
  await delay();
  if (input.otp !== DEMO_OTP) throw new OtpError("Mã OTP không đúng");

  const now = new Date();
  const record: TransactionRecord = {
    id: `tx_${now.getTime()}`,
    reference: `FT${now.getTime().toString().slice(-11)}`,
    direction: "out",
    status: "success",
    kind: "transfer",
    amount: input.amount,
    fee: 0,
    note: input.note,
    counterparty: { ...input.recipient },
    createdAt: now,
    balanceAfter: input.source.balance - input.amount,
  };
  transactionStore.add(record);
  return record;
}
