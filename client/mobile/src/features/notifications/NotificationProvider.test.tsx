import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "@/shared/notifications/toast";
import { NotificationProvider } from "./NotificationProvider";
import { useNotifications } from "./notificationContext";

const logout = vi.fn();
vi.mock("@/features/auth/useAuth", () => ({
  useAuth: () => ({ status: "authenticated", token: "access-token", logout }),
}));

const encoder = new TextEncoder();

/** A fetch Response whose body yields the given SSE chunks, then ends. */
function streamResponse(chunks: string[]) {
  let index = 0;
  return {
    ok: true,
    status: 200,
    body: {
      getReader: () => ({
        read: async () =>
          index < chunks.length
            ? { done: false, value: encoder.encode(chunks[index++]) }
            : { done: true, value: undefined },
        releaseLock: () => {},
      }),
    },
  } as unknown as Response;
}

const dto = {
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
};

function Badge() {
  const { unreadCount, live } = useNotifications();
  return (
    <p>
      count:{unreadCount} live:{live.length}
    </p>
  );
}

beforeEach(() => {
  logout.mockReset();
});
afterEach(() => {
  cleanup();
  toast.dismiss();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

it("updates the badge, shows a banner and reconnects with the last event id", async () => {
  const show = vi.spyOn(toast, "show");
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(
      streamResponse([
        ': connected\n\nevent: unread-count\ndata: {"count":2}\n\n',
        `id: cursor-1\nevent: notification\ndata: ${JSON.stringify(dto)}\n\n`,
        'event: unread-count\ndata: {"count":3}\n\n',
      ]),
    )
    // The replay after reconnect resends n1: it must not be announced twice.
    .mockResolvedValue(
      streamResponse([
        `id: cursor-1\nevent: notification\ndata: ${JSON.stringify(dto)}\n\n`,
      ]),
    );
  vi.stubGlobal("fetch", fetchMock);

  render(
    <NotificationProvider>
      <Badge />
    </NotificationProvider>,
  );

  await waitFor(() => expect(screen.getByText("count:3 live:1")).toBeTruthy());
  expect(show).toHaveBeenCalledWith(
    "+500.000 VND",
    "success",
    expect.objectContaining({ title: "Nhận tiền thành công" }),
  );
  const [firstUrl, firstInit] = fetchMock.mock.calls[0];
  expect(String(firstUrl)).not.toContain("cursor=");
  expect(firstInit.headers.Authorization).toBe("Bearer access-token");

  await waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(1), {
    timeout: 3000,
  });
  expect(String(fetchMock.mock.calls[1][0])).toContain("cursor=cursor-1");
  expect(show).toHaveBeenCalledTimes(1);
});

it("logs out when the stream is unauthorized", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      text: async () => "{}",
    } as Response),
  );
  render(
    <NotificationProvider>
      <Badge />
    </NotificationProvider>,
  );
  await waitFor(() => expect(logout).toHaveBeenCalledOnce());
});
