import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LoanContractsAdminPage, SavingsContractsPage } from "./ContractsPages";
import {
  listLoanContracts,
  listSavingsContracts,
  getLoanContract,
  getPackage,
} from "./api";
import { getLoanCustomer } from "@/features/loans/api/loansApi";
import type { LoanContractDetail, PackageDetail } from "./model";

vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  listSavingsContracts: vi.fn(),
  getSavingsContract: vi.fn(),
  listLoanContracts: vi.fn(),
  getLoanContract: vi.fn(),
  getPackage: vi.fn(),
}));
vi.mock("@/features/loans/api/loansApi", async (original) => ({
  ...(await original<typeof import("@/features/loans/api/loansApi")>()),
  getLoanCustomer: vi.fn(),
}));

const loan = {
  loanId: "l1",
  loanNo: "LN1",
  customerId: "c1",
  loanPackageId: "P",
  loanPackageVersion: 2,
  principal: 10_000_000,
  outstandingPrincipal: 9_000_000,
  annualInterestRate: "15",
  termMonths: 12,
  status: "active",
  paidInstallments: 1,
  totalInstallments: 12,
  nextDueDate: "2026-10-01",
  nextDueAmount: 900_000,
  overdueInstallments: 2,
  oldestOverdueDate: "2026-10-01",
};

describe("admin contract pages", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows the customer and the applied package version", async () => {
    vi.mocked(listLoanContracts).mockResolvedValue({
      items: [loan],
      nextCursor: null,
    } as never);
    vi.mocked(getLoanContract).mockResolvedValue({
      ...loan,
      repaymentSchedule: [],
      payments: [],
    } as unknown as LoanContractDetail);
    vi.mocked(getLoanCustomer).mockResolvedValue({
      id: "c1",
      accountNo: "1234567890",
      username: "nguyenvana",
      email: "a@example.com",
      isVerify: true,
      amount: 5_000_000,
    });
    vi.mocked(getPackage).mockResolvedValue({
      name: "Tên mới",
      versions: [{ version: 2, name: "Vay mua xe" }],
    } as unknown as PackageDetail);
    render(<LoanContractsAdminPage />);
    fireEvent.click(await screen.findByRole("button", { name: "LN1" }));
    expect(await screen.findByText("nguyenvana")).toBeTruthy();
    expect(getLoanCustomer).toHaveBeenCalledWith("c1");
    expect(
      await screen.findByText("Gói áp dụng: Vay mua xe (P) · phiên bản 2"),
    ).toBeTruthy();
  });

  it("moves between pages and returns to page 1 when a filter changes", async () => {
    vi.mocked(listLoanContracts).mockImplementation(
      async ({ cursor, status }) =>
        ({
          items: [
            {
              ...loan,
              loanId: cursor ?? status ?? "a",
              loanNo: cursor ?? "LN1",
            },
          ],
          nextCursor: cursor ? null : "c2",
        }) as never,
    );
    render(<LoanContractsAdminPage />);
    await screen.findByRole("button", { name: "LN1" });
    fireEvent.click(screen.getByRole("button", { name: "Trang sau" }));
    await screen.findByRole("button", { name: "c2" });
    expect(screen.getByText("Trang 2")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Trang sau" })).toHaveProperty(
      "disabled",
      true,
    );
    fireEvent.click(screen.getByRole("button", { name: "Trang trước" }));
    await screen.findByRole("button", { name: "LN1" });
    fireEvent.click(screen.getByRole("button", { name: "Trang sau" }));
    await screen.findByRole("button", { name: "c2" });
    fireEvent.change(screen.getByLabelText("Trạng thái"), {
      target: { value: "active" },
    });
    await waitFor(() => expect(screen.getByText("Trang 1")).toBeTruthy());
    expect(
      vi.mocked(listLoanContracts).mock.lastCall?.[0].cursor,
    ).toBeUndefined();
  });

  it("filters savings by status", async () => {
    vi.mocked(listSavingsContracts).mockResolvedValue({
      items: [],
      nextCursor: null,
    });
    render(<SavingsContractsPage />);
    expect(await screen.findByText("Không có sổ phù hợp.")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Trạng thái"), {
      target: { value: "matured" },
    });
    await waitFor(() =>
      expect(listSavingsContracts).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: "matured" }),
      ),
    );
  });

  it("asks only for overdue loans and shows how late they are", async () => {
    vi.mocked(listLoanContracts).mockResolvedValue({
      items: [loan],
      nextCursor: null,
    });
    render(<LoanContractsAdminPage overdue />);
    expect(await screen.findByText(/2 kỳ, từ/)).toBeTruthy();
    expect(listLoanContracts).toHaveBeenCalledWith(
      expect.objectContaining({ overdue: "true", status: "active" }),
    );
  });

  it("opens a contract with its schedule", async () => {
    vi.mocked(listLoanContracts).mockResolvedValue({
      items: [loan],
      nextCursor: null,
    });
    vi.mocked(getLoanContract).mockResolvedValue({
      ...loan,
      repaymentSchedule: [
        {
          installmentNo: 2,
          dueDate: "2026-10-01",
          principalDue: 1,
          interestDue: 1,
          totalDue: 2,
          status: "overdue",
        },
      ],
      payments: [],
    });
    render(<LoanContractsAdminPage />);
    fireEvent.click(await screen.findByRole("button", { name: "LN1" }));
    expect(await screen.findByText("Quá hạn")).toBeTruthy();
    expect(getLoanContract).toHaveBeenCalledWith("l1");
  });
});
