import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AccountTimelinePage } from "./AccountTimelinePage";
import { getAccountTimeline } from "./api/auditApi";

vi.mock("./api/auditApi", () => ({
  getAccountTimeline: vi.fn(),
}));

const response = {
  account: {
    id: "account-1",
    accountNo: "ACC-0001",
    username: "alice",
    email: "alice@example.com",
    customerName: "Alice Nguyen",
    currency: "VND",
    status: "active",
    balance: 1500000,
  },
  items: [
    {
      eventId: "event-1",
      eventType: "TransferCompleted",
      category: "transfer" as const,
      occurredAt: "2026-10-30T07:30:00Z",
      sourceStream: "banking",
      sourceRevision: 12,
      transactionId: "tx-1",
      summary: "Chuyển tiền thành công",
      details: {
        amount: 500000,
        balanceBefore: 2000000,
        balanceAfter: 1500000,
        currency: "VND",
        direction: "out",
      },
    },
  ],
  hasMore: false,
};

describe("AccountTimelinePage", () => {
  beforeEach(() => {
    vi.mocked(getAccountTimeline).mockReset();
  });

  it("keeps an empty search neutral and does not call the API", () => {
    render(<AccountTimelinePage />);

    const button = screen.getByRole("button", { name: "Xem lịch sử" });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByRole("alert")).toBeNull();
    expect(getAccountTimeline).not.toHaveBeenCalled();
  });

  it("searches by account and renders the grouped timeline", async () => {
    vi.mocked(getAccountTimeline).mockResolvedValue(response);
    render(<AccountTimelinePage />);

    fireEvent.change(screen.getByPlaceholderText(/Account ID/i), {
      target: { value: "ACC-0001" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Xem lịch sử" }));

    expect(await screen.findByText("Alice Nguyen")).toBeTruthy();
    expect(screen.getByText("Chuyển tiền thành công")).toBeTruthy();
    expect(screen.getByText("Giao dịch: tx-1")).toBeTruthy();
    await waitFor(() =>
      expect(getAccountTimeline).toHaveBeenCalledWith(
        "ACC-0001",
        expect.objectContaining({ category: "all" }),
      ),
    );
  });

  it("uses the submitted query and cursor for pagination", async () => {
    vi.mocked(getAccountTimeline)
      .mockResolvedValueOnce({
        ...response,
        nextCursor: "cursor-1",
        hasMore: true,
      })
      .mockResolvedValueOnce({
        ...response,
        nextCursor: undefined,
        hasMore: false,
      });
    render(<AccountTimelinePage />);

    fireEvent.change(screen.getByPlaceholderText(/Account ID/i), {
      target: { value: "ACC-0001" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Xem lịch sử" }));
    await screen.findByText("Alice Nguyen");

    const nextButton = screen.getByRole("button", {
      name: "Trang sau →",
    }) as HTMLButtonElement;
    expect(nextButton.disabled).toBe(false);
    fireEvent.click(nextButton);

    await waitFor(() =>
      expect(getAccountTimeline).toHaveBeenNthCalledWith(
        2,
        "account-1",
        expect.objectContaining({
          category: "all",
          cursor: "cursor-1",
          limit: 20,
        }),
      ),
    );

    fireEvent.change(screen.getByRole("combobox", { name: "Loại event" }), {
      target: { value: "otp" },
    });
    expect(
      (screen.getByRole("button", { name: "Trang sau →" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
});
