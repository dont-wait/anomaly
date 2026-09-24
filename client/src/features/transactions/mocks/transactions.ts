import type { TransactionRecord } from "@/features/transactions/model";

const HOUR = 60 * 60 * 1000;
const hoursAgo = (hours: number) => new Date(Date.now() - hours * HOUR);

/** Seed demo — giữ bất biến, `clear()` chỉ xóa record của user, không xóa seed. */
const seed: TransactionRecord[] = [
  {
    id: "tx_1001",
    reference: "FT26265118302",
    direction: "out",
    status: "success",
    kind: "payment",
    amount: 55_000,
    fee: 0,
    note: "Thanh toan QR Highlands Coffee",
    counterparty: { name: "Highlands Coffee" },
    createdAt: hoursAgo(2),
    balanceAfter: 128_540_000,
  },
  {
    id: "tx_1002",
    reference: "FT26265097741",
    direction: "out",
    status: "pending",
    kind: "transfer",
    amount: 1_200_000,
    fee: 0,
    note: "Tien nha thang 9",
    counterparty: {
      name: "LE VAN CUONG",
      accountNo: "99999180233",
      bank: "AnomalyBank",
    },
    createdAt: hoursAgo(5),
  },
  {
    id: "tx_1003",
    reference: "FT26264175520",
    direction: "in",
    status: "success",
    kind: "transfer",
    amount: 2_500_000,
    fee: 0,
    note: "Tra tien an trua",
    counterparty: {
      name: "TRAN THI BICH",
      accountNo: "99999180147",
      bank: "AnomalyBank",
    },
    createdAt: hoursAgo(26),
    balanceAfter: 129_795_000,
  },
  {
    id: "tx_1004",
    reference: "FT26264121904",
    direction: "out",
    status: "success",
    kind: "payment",
    amount: 348_000,
    fee: 0,
    note: "Thanh toan WinMart",
    counterparty: { name: "Siêu thị WinMart" },
    createdAt: hoursAgo(31),
    balanceAfter: 127_295_000,
  },
  {
    id: "tx_1005",
    reference: "FT26263080017",
    direction: "out",
    status: "failed",
    kind: "transfer",
    amount: 5_000_000,
    fee: 0,
    note: "Chuyen tien hoc phi",
    counterparty: {
      name: "NGUYEN MINH DUC",
      accountNo: "99999180389",
      bank: "AnomalyBank",
    },
    createdAt: hoursAgo(52),
  },
  {
    id: "tx_1006",
    reference: "FT26262094410",
    direction: "out",
    status: "success",
    kind: "bill",
    amount: 712_400,
    fee: 0,
    note: "Hoa don dien EVN ky 08/2026",
    counterparty: { name: "EVN Hà Nội" },
    createdAt: hoursAgo(76),
    balanceAfter: 127_643_000,
  },
  {
    id: "tx_1007",
    reference: "FT26261160032",
    direction: "out",
    status: "success",
    kind: "topup",
    amount: 100_000,
    fee: 0,
    note: "Nap tien dien thoai 0912xxx888",
    counterparty: { name: "Viettel" },
    createdAt: hoursAgo(98),
    balanceAfter: 128_355_400,
  },
  {
    id: "tx_1008",
    reference: "FT26258000125",
    direction: "in",
    status: "success",
    kind: "salary",
    amount: 18_500_000,
    fee: 0,
    note: "Luong thang 8/2026",
    counterparty: { name: "CONG TY TNHH ANOMALY TECH" },
    createdAt: hoursAgo(24 * 6 + 3),
    balanceAfter: 128_455_400,
  },
  {
    id: "tx_1009",
    reference: "FT26240000118",
    direction: "in",
    status: "success",
    kind: "savings",
    amount: 485_000,
    fee: 0,
    note: "Tien lai tiet kiem ky han 6 thang",
    counterparty: { name: "AnomalyBank" },
    createdAt: hoursAgo(24 * 25),
    balanceAfter: 109_955_400,
  },
];

/** Kho giao dịch mock trong bộ nhớ (RAM, mất khi reload) — thay bằng API phân trang khi backend có. */
let records: TransactionRecord[] = [...seed];

const sortedDesc = (items: TransactionRecord[]): TransactionRecord[] =>
  [...items].sort((a, b) => b.createdAt.getTime() - a.createdAt.getTime());

const visibleTo = (
  items: TransactionRecord[],
  ownerAccountNo?: string,
): TransactionRecord[] =>
  ownerAccountNo === undefined
    ? items
    : items.filter(
        (record) =>
          !record.ownerAccountNo || record.ownerAccountNo === ownerAccountNo,
      );

/**
 * Store mock cô lập theo STK + purge khi logout để khỏi leak sang user sau
 * và khỏi phình RAM. Seed không có `ownerAccountNo` nên hiện với mọi user.
 */
export const transactionStore = {
  list: (ownerAccountNo?: string): TransactionRecord[] =>
    sortedDesc(visibleTo(records, ownerAccountNo)),

  get: (id: string, ownerAccountNo?: string) => {
    const record = records.find((item) => item.id === id);
    if (!record) return undefined;
    if (
      ownerAccountNo !== undefined &&
      record.ownerAccountNo &&
      record.ownerAccountNo !== ownerAccountNo
    ) {
      return undefined;
    }
    return record;
  },

  add: (record: TransactionRecord) => {
    records.push(record);
  },

  /**
   * Xóa record của user để khỏi thành "rác" RAM + leak sau logout.
   * Không args: xóa mọi record có owner, giữ seed. Có owner: chỉ xóa của owner đó.
   */
  clear: (ownerAccountNo?: string) => {
    records =
      ownerAccountNo === undefined
        ? records.filter((record) => !record.ownerAccountNo)
        : records.filter(
            (record) =>
              !record.ownerAccountNo ||
              record.ownerAccountNo !== ownerAccountNo,
          );
  },
};
