import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { TransactionRecord } from "@/features/transactions/model";
import { TransactionHistory } from "./TransactionHistory";
import { TransactionReceipt } from "./components/TransactionReceipt";

const now = new Date(2026, 8, 22, 12, 0);
const make = (overrides: Partial<TransactionRecord>): TransactionRecord => ({
  id: "tx",
  reference: "FT000",
  direction: "out",
  status: "success",
  kind: "transfer",
  amount: 100_000,
  fee: 0,
  note: "",
  counterparty: {
    name: "NGUOI NHAN",
    accountNo: "99999180147",
    bank: "AnomalyBank",
  },
  createdAt: new Date(2026, 8, 22, 9, 0),
  ...overrides,
});

const records = [
  make({
    id: "a",
    counterparty: { name: "Highlands Coffee" },
    kind: "payment",
    amount: 55_000,
  }),
  make({
    id: "b",
    direction: "in",
    amount: 2_000_000,
    note: "Trả tiền ăn",
    createdAt: new Date(2026, 8, 21, 18, 0),
  }),
  make({
    id: "c",
    status: "failed",
    amount: 700_000,
    createdAt: new Date(2026, 8, 10, 8, 0),
  }),
  make({
    id: "d",
    direction: "in",
    amount: 9_000_000,
    createdAt: new Date(2026, 7, 30, 8, 0),
  }),
];

afterEach(cleanup);

it("renders a loan disbursement in history and its receipt", () => {
  const record = make({
    id: "loan-1",
    reference: "LDloan-1",
    kind: "loan_disbursement",
    direction: "in",
    counterparty: { name: "ANOMALY LOAN", bank: "ANOMALY" },
  });
  const onSelect = vi.fn();
  const { unmount } = render(
    <TransactionHistory records={[record]} onSelect={onSelect} now={now} />,
  );
  expect(screen.getByText(/Giải ngân khoản vay/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /ANOMALY LOAN/ }));
  expect(onSelect).toHaveBeenCalledWith(record);

  unmount();
  render(<TransactionReceipt record={record} />);
  expect(screen.getByText("Giải ngân khoản vay")).toBeTruthy();
  expect(screen.getByText("+100.000₫")).toBeTruthy();
});

it("summarises successful in/out totals for the selected range", () => {
  render(<TransactionHistory records={records} onSelect={vi.fn()} now={now} />);
  const summary = screen.getByRole("region", {
    name: "Tổng quan từ 01/09/2026 đến 22/09/2026",
  });
  expect(within(summary).getByText("+2.000.000₫")).toBeTruthy();
  expect(within(summary).getByText("-55.000₫")).toBeTruthy();
});

it("groups by day, filters by direction and searches without diacritics", () => {
  const onSelect = vi.fn();
  render(
    <TransactionHistory records={records} onSelect={onSelect} now={now} />,
  );
  expect(screen.getByRole("heading", { name: "Hôm nay" })).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Hôm qua" })).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "Tiền vào" }));
  expect(screen.queryByText("Highlands Coffee")).toBeNull();
  expect(screen.getAllByText(/Nhận từ/)).toHaveLength(2);

  fireEvent.click(screen.getByRole("button", { name: "Tất cả" }));
  fireEvent.change(screen.getByLabelText("Tìm kiếm giao dịch"), {
    target: { value: "tra tien an" },
  });
  expect(screen.getAllByRole("button", { name: /Nhận từ/ })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: /Nhận từ/ }));
  expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "b" }));

  fireEvent.change(screen.getByLabelText("Tìm kiếm giao dịch"), {
    target: { value: "khong co" },
  });
  expect(screen.getByText("Không có giao dịch phù hợp")).toBeTruthy();
});

it("supports controlled filters, query and a server-provided summary", () => {
  const onFilterChange = vi.fn();
  const onQueryChange = vi.fn();
  const { rerender } = render(
    <TransactionHistory
      records={records.slice(0, 1)}
      onSelect={vi.fn()}
      now={now}
      filter="all"
      onFilterChange={onFilterChange}
      query=""
      onQueryChange={onQueryChange}
      summary={{ totalIn: 5_000_000, totalOut: 1_500_000 }}
    />,
  );

  expect(screen.getByText("+5.000.000₫")).toBeTruthy();
  expect(screen.getByText("-1.500.000₫")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Tiền ra" }));
  fireEvent.change(screen.getByLabelText("Tìm kiếm giao dịch"), {
    target: { value: "coffee" },
  });
  expect(onFilterChange).toHaveBeenCalledWith("out");
  expect(onQueryChange).toHaveBeenCalledWith("coffee");

  rerender(
    <TransactionHistory
      records={records.slice(0, 1)}
      onSelect={vi.fn()}
      now={now}
      filter="all"
      onFilterChange={onFilterChange}
      query=""
      onQueryChange={onQueryChange}
      summary={{ totalIn: 5_000_000, totalOut: 1_500_000 }}
    />,
  );
  expect(
    screen.getByRole("button", { name: "Tất cả" }).getAttribute("aria-pressed"),
  ).toBe("true");
  expect(
    (screen.getByLabelText("Tìm kiếm giao dịch") as HTMLInputElement).value,
  ).toBe("");
});
