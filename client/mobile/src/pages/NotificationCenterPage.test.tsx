import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { transactionDetailRoute } from "@/app/routes";
import { NotificationContext } from "@/features/notifications/notificationContext";
import NotificationCenterPage from "./NotificationCenterPage";

vi.mock("@/features/auth/useAuth", () => ({
  useAuth: () => ({ token: "access-token", logout: vi.fn() }),
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const ok = (data: unknown) =>
  ({
    ok: true,
    status: 200,
    text: async () => JSON.stringify({ status: 200, message: "ok", data }),
  }) as Response;

it("lists notifications, marks one read and opens its transaction", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      ok({
        items: [
          {
            id: "n1",
            type: "TRANSFER_RECEIVED",
            category: "TRANSACTION",
            severity: "SUCCESS",
            title: "Nhận tiền thành công",
            body: "+500.000 VND",
            data: { transferId: "tx-1" },
            occurredAt: "2026-11-04T08:00:00Z",
            createdAt: "2026-11-04T08:00:01Z",
            read: false,
          },
        ],
        nextCursor: "",
      }),
    ),
  );
  const markRead = vi.fn().mockResolvedValue(undefined);
  render(
    <NotificationContext.Provider
      value={{ unreadCount: 1, live: [], markRead, markAllRead: vi.fn() }}
    >
      <NotificationCenterPage />
    </NotificationContext.Provider>,
  );

  fireEvent.click(await screen.findByText("Nhận tiền thành công"));
  await waitFor(() =>
    expect(window.location.hash).toBe(transactionDetailRoute("tx-1")),
  );
  expect(markRead).toHaveBeenCalledWith(expect.objectContaining({ id: "n1" }));
  expect(screen.queryByText("Xem thêm")).toBeNull();
});
