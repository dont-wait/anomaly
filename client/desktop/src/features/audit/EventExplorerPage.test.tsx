import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { EventExplorerPage } from "./EventExplorerPage";
import {
  getEventExplorer,
  getEventExplorerDetail,
} from "./api/eventExplorerApi";

vi.mock("./api/eventExplorerApi", () => ({
  EventExplorerApiError: class EventExplorerApiError extends Error {},
  getEventExplorer: vi.fn(),
  getEventExplorerDetail: vi.fn(),
}));

const response = {
  items: [
    {
      eventId: "event-1",
      eventType: "TransferCompleted",
      category: "transfer" as const,
      occurredAt: "2026-10-30T07:30:00Z",
      sourceStream: "banking",
      sourceRevision: 12,
      accounts: [{ accountNo: "0123456789", customerName: "Alice Nguyen" }],
      transactionId: "tx-1",
      summary: "TransferCompleted",
      details: { status: "success", amount: 500000 },
    },
  ],
  nextCursor: "cursor-1",
  hasMore: true,
  scanComplete: false,
  scanned: 20,
};

describe("EventExplorerPage", () => {
  beforeEach(() => {
    vi.mocked(getEventExplorer).mockReset();
    vi.mocked(getEventExplorerDetail).mockReset();
  });

  it("keeps the explorer empty until the operator searches", () => {
    render(<EventExplorerPage />);

    expect(screen.getByText(/Chưa thực hiện tra cứu/i)).toBeTruthy();
    expect(getEventExplorer).not.toHaveBeenCalled();
  });

  it("searches, paginates and opens event details", async () => {
    vi.mocked(getEventExplorer)
      .mockResolvedValueOnce(response)
      .mockResolvedValueOnce({
        ...response,
        nextCursor: undefined,
        hasMore: false,
        scanComplete: true,
      });
    vi.mocked(getEventExplorerDetail).mockResolvedValue(response.items[0]);
    render(<EventExplorerPage />);

    fireEvent.change(
      screen.getByPlaceholderText(/Nhập mã giao dịch hoặc số tài khoản/i),
      { target: { value: "tx-1" } },
    );
    fireEvent.click(screen.getByRole("button", { name: "Tìm kiếm" }));

    expect(await screen.findByText("Chuyển tiền thành công")).toBeTruthy();
    expect(getEventExplorer).toHaveBeenCalledWith(
      expect.objectContaining({
        searchType: "transactionId",
        search: "tx-1",
        limit: 20,
      }),
    );

    fireEvent.click(
      screen.getByRole("button", { name: /Chuyển tiền thành công/i }),
    );
    expect(screen.getByRole("dialog", { name: "Chi tiết event" })).toBeTruthy();
    expect(getEventExplorerDetail).toHaveBeenCalledWith(
      expect.objectContaining({ eventId: "event-1" }),
    );
    await waitFor(() =>
      expect(screen.getByText("Số tiền: 500.000 VND")).toBeTruthy(),
    );
    fireEvent.click(screen.getByText("Thông tin kỹ thuật"));
    expect(screen.getByText("event-1")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Đóng chi tiết" }));
    fireEvent.click(screen.getByRole("button", { name: "Trang sau →" }));
    await waitFor(() =>
      expect(getEventExplorer).toHaveBeenNthCalledWith(
        2,
        expect.objectContaining({ cursor: "cursor-1" }),
      ),
    );
  });
});
