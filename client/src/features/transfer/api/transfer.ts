import { transactionStore } from "@/features/transactions";
import type { TransactionRecord } from "@/features/transactions/model";
import {
  ANOMALY_BANK,
  type Bank,
  type Recipient,
  type SourceAccount,
} from "@/features/transfer/model";

// TODO: thay toàn bộ file này bằng API thật khi backend có endpoint chuyển tiền.
export const MOCK_LATENCY_MS = 400;
export const DEMO_OTP = "123456";

const vietQrBank = (
  code: string,
  bin: string,
  shortName: string,
  name: string,
): Bank => ({
  code,
  bin,
  shortName,
  name,
  logo: `https://cdn.vietqr.io/img/${code}.png`,
});

const VCB = vietQrBank(
  "VCB",
  "970436",
  "Vietcombank",
  "Ngân hàng TMCP Ngoại Thương Việt Nam",
);
const TCB = vietQrBank(
  "TCB",
  "970407",
  "Techcombank",
  "Ngân hàng TMCP Kỹ thương Việt Nam",
);
const MB = vietQrBank("MB", "970422", "MBBank", "Ngân hàng TMCP Quân đội");
const BIDV = vietQrBank(
  "BIDV",
  "970418",
  "BIDV",
  "Ngân hàng TMCP Đầu tư và Phát triển Việt Nam",
);

const directory: Recipient[] = [
  { accountNo: "99999180147", name: "TRAN THI BICH", bank: ANOMALY_BANK },
  { accountNo: "99999180233", name: "LE VAN CUONG", bank: ANOMALY_BANK },
  { accountNo: "99999180389", name: "NGUYEN MINH DUC", bank: ANOMALY_BANK },
  { accountNo: "99999180412", name: "PHAM THU HA", bank: ANOMALY_BANK },
  { accountNo: "0011001234567", name: "NGUYEN VAN AN", bank: VCB },
  { accountNo: "19036541234012", name: "LE THI MAI", bank: TCB },
  { accountNo: "0901234567", name: "HOANG DUC ANH", bank: MB },
  { accountNo: "12510000123456", name: "TRAN QUOC BAO", bank: BIDV },
];

const delay = () =>
  new Promise((resolve) => setTimeout(resolve, MOCK_LATENCY_MS));

// Chừa vài tài khoản chưa lưu để demo được nút "Lưu vào danh bạ".
const seedContacts: Recipient[] = directory.filter(
  (recipient) => !["TCB", "BIDV"].includes(recipient.bank.code),
);

/**
 * Danh bạ mock trong RAM, cô lập theo STK chủ sở hữu.
 * Không owner (đường legacy/test) dùng `globalContacts`; có owner dùng map riêng
 * clone từ seed. `clear()` purge để khỏi leak + phình RAM sau logout.
 */
const globalContacts: Recipient[] = [...seedContacts];
const contactsByOwner = new Map<string, Recipient[]>();

const resolveContacts = (ownerAccountNo?: string): Recipient[] => {
  if (ownerAccountNo === undefined) return globalContacts;
  let list = contactsByOwner.get(ownerAccountNo);
  if (!list) {
    list = [...seedContacts];
    contactsByOwner.set(ownerAccountNo, list);
  }
  return list;
};

const sameRecipient = (a: Recipient, b: Recipient) =>
  a.accountNo === b.accountNo && a.bank.code === b.bank.code;

/** Danh bạ người nhận trong bộ nhớ — thay bằng API danh bạ khi backend có. */
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
    const list = resolveContacts(ownerAccountNo);
    if (!list.some((contact) => sameRecipient(contact, recipient)))
      list.push(recipient);
  },
  remove: (recipient: Recipient, ownerAccountNo?: string) => {
    const list = resolveContacts(ownerAccountNo);
    const index = list.findIndex((contact) =>
      sameRecipient(contact, recipient),
    );
    if (index >= 0) list.splice(index, 1);
  },
  /**
   * Purge danh bạ khỏi RAM khi logout. Có owner: chỉ xóa của owner đó
   * (map entry bị drop, lần sau clone lại từ seed). Không args: purge tất cả.
   */
  clear: (ownerAccountNo?: string) => {
    if (ownerAccountNo === undefined) {
      globalContacts.splice(0, globalContacts.length, ...seedContacts);
      contactsByOwner.clear();
      return;
    }
    contactsByOwner.delete(ownerAccountNo);
  },
};

export const recentRecipients: Recipient[] = [
  directory[0],
  directory[4],
  directory[1],
  directory[5],
];

/** Tra cứu chủ tài khoản theo ngân hàng + số tài khoản; `null` nếu không tồn tại. */
export async function lookupRecipient(
  bank: Bank,
  accountNo: string,
): Promise<Recipient | null> {
  await delay();
  return (
    directory.find(
      (r) => r.bank.code === bank.code && r.accountNo === accountNo,
    ) ?? null
  );
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
    counterparty: {
      name: input.recipient.name,
      accountNo: input.recipient.accountNo,
      bank: input.recipient.bank.shortName,
    },
    createdAt: now,
    balanceAfter: input.source.balance - input.amount,
    ownerAccountNo: input.source.accountNo,
  };
  transactionStore.add(record);
  return record;
}
