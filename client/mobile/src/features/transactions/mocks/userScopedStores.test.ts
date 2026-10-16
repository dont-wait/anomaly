import { beforeEach, expect, it } from "vitest";
import { transactionStore } from "@/features/transactions/mocks/transactions";
import type { TransactionRecord } from "@/features/transactions/model";

const OWNER_A = "99999180105";
const OWNER_B = "99999180999";

const makeRecord = (id: string, ownerAccountNo: string): TransactionRecord => ({
  id,
  reference: `FT-${id}`,
  direction: "out",
  status: "success",
  kind: "transfer",
  amount: 10_000,
  fee: 0,
  note: "",
  counterparty: { name: "NGUOI NHAN" },
  createdAt: new Date(),
  balanceAfter: 1_000_000,
  ownerAccountNo,
});

beforeEach(() => {
  transactionStore.clear();
});

it("cô lập giao dịch theo STK và purge khi logout", () => {
  const seedCount = transactionStore.list().length;

  transactionStore.add(makeRecord("tx_owner_a", OWNER_A));

  expect(
    transactionStore.list(OWNER_A).some((r) => r.id === "tx_owner_a"),
  ).toBe(true);
  // User khác không thấy record + không mở được detail.
  expect(
    transactionStore.list(OWNER_B).some((r) => r.id === "tx_owner_a"),
  ).toBe(false);
  expect(transactionStore.get("tx_owner_a", OWNER_B)).toBeUndefined();
  expect(transactionStore.get("tx_owner_a", OWNER_A)?.id).toBe("tx_owner_a");

  // Purge 1 owner: mất của A, giữ seed.
  transactionStore.clear(OWNER_A);
  expect(
    transactionStore.list(OWNER_A).some((r) => r.id === "tx_owner_a"),
  ).toBe(false);
  expect(transactionStore.list().length).toBe(seedCount);

  // Purge hết (logout): mất mọi record có owner, giữ seed.
  transactionStore.add(makeRecord("tx_owner_b", OWNER_B));
  transactionStore.clear();
  expect(transactionStore.list().length).toBe(seedCount);
});
