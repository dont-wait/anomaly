import { beforeEach, expect, it } from "vitest";
import { transactionStore } from "@/features/transactions/mocks/transactions";
import { contactStore } from "@/features/transfer/api/transfer";
import { ANOMALY_BANK } from "@/features/transfer/model";
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
  contactStore.clear();
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

it("cô lập danh bạ theo STK và purge khi logout", () => {
  // Seed demo hiện với mọi owner; chỉ contact user tự thêm mới cô lập.
  const ownContact = {
    accountNo: "99999180999",
    name: "VO THI LAN",
    bank: ANOMALY_BANK,
  };
  contactStore.add(ownContact, OWNER_A);
  expect(contactStore.list(OWNER_A).some((c) => c.name === "VO THI LAN")).toBe(
    true,
  );
  expect(contactStore.list(OWNER_B).some((c) => c.name === "VO THI LAN")).toBe(
    false,
  );

  // Purge 1 owner: contact tự thêm mất, seed giữ nguyên.
  contactStore.clear(OWNER_A);
  expect(contactStore.list(OWNER_A).some((c) => c.name === "VO THI LAN")).toBe(
    false,
  );
  expect(contactStore.list(OWNER_A).some((c) => c.name === "PHAM THU HA")).toBe(
    true, // seed còn
  );

  const contact = {
    accountNo: "99999180412",
    name: "PHAM THU HA",
    bank: ANOMALY_BANK,
  };
  contactStore.remove(contact, OWNER_A);

  contactStore.remove(contact, OWNER_A);
  expect(contactStore.list(OWNER_A).some((c) => c.name === "PHAM THU HA")).toBe(
    false,
  );
  // Xoá seed của A không ảnh hưởng seed của B.
  expect(contactStore.list(OWNER_B).some((c) => c.name === "PHAM THU HA")).toBe(
    true,
  );
  contactStore.clear();
  expect(contactStore.list(OWNER_A).some((c) => c.name === "PHAM THU HA")).toBe(
    true,
  );
});
