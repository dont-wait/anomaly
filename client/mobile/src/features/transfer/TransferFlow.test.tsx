import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { TransferFlow } from "./TransferFlow";

const token = "access-token";
const source = {
  accountNo: "99999180105",
  ownerName: "Pham Minh Hieu",
  balance: 3_000_000,
};
const recipient = {
  accountNo: "99999180147",
  name: "TRAN THI BICH",
  bankCode: "ANOMALY",
};

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status });

const otpInfo = (
  resendAvailableAt = new Date(Date.now() + 30_000).toISOString(),
) => ({
  channel: "email",
  maskedDestination: "p***@example.com",
  expiresAt: new Date(Date.now() + 300_000).toISOString(),
  attemptsLeft: 3,
  resendAvailableAt,
});

const intent = () => ({
  transferId: "8bfbe3d4-742c-4602-b2bf-31ccf62de7ad",
  status: "awaiting_otp",
  recipient,
  amount: 500_000,
  fee: 0,
  note: "Pham Minh Hieu chuyen tien",
  otp: otpInfo(),
});

const transaction = {
  id: "8bfbe3d4-742c-4602-b2bf-31ccf62de7ad",
  reference: "FT8BFBE3D4742C4602B2BF31CCF62DE7AD",
  direction: "out",
  status: "success",
  kind: "transfer",
  amount: 500_000,
  fee: 0,
  note: "Pham Minh Hieu chuyen tien",
  counterparty: { ...recipient, bank: "AnomalyBank" },
  createdAt: "2026-10-07T05:01:00Z",
  balanceAfter: 2_400_000,
};

function defaultFetch(input: RequestInfo | URL, init?: RequestInit) {
  const url = String(input);
  if (url.includes("/api/transfers/recent-recipients")) {
    return Promise.resolve(json({ items: [] }));
  }
  if (url.includes("/api/accounts/lookup")) {
    return Promise.resolve(json(recipient));
  }
  if (url.endsWith("/api/transfers") && init?.method === "POST") {
    return Promise.resolve(json(intent(), 201));
  }
  if (url.includes("/confirm")) return Promise.resolve(json(transaction));
  throw new Error(`Unexpected request: ${url}`);
}

beforeEach(() => {
  vi.spyOn(globalThis.crypto, "randomUUID").mockReturnValue(
    "34297a72-9433-4adf-8958-868d5e50d197",
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function renderFlow(fetchImplementation = defaultFetch) {
  const fetchMock = vi.fn(fetchImplementation);
  vi.stubGlobal("fetch", fetchMock);
  render(
    <TransferFlow
      token={token}
      source={source}
      onExit={vi.fn()}
      onViewHistory={vi.fn()}
    />,
  );
  return fetchMock;
}

async function enterRecipient() {
  const input = screen.getByLabelText("Số tài khoản");
  fireEvent.change(input, { target: { value: recipient.accountNo } });
  fireEvent.blur(input);
  await waitFor(() =>
    expect(
      (screen.getByLabelText("Tên người nhận") as HTMLInputElement).value,
    ).toBe(recipient.name),
  );
}

async function createIntent(amount = "500000") {
  await enterRecipient();
  fireEvent.change(screen.getByLabelText("Số tiền chuyển"), {
    target: { value: amount },
  });
  fireEvent.click(screen.getByRole("button", { name: "Chuyển tiền" }));
  return screen.findByRole("dialog", { name: "Xác thực giao dịch" });
}

it("loads recent recipients and looks up only ANOMALY accounts with auth", async () => {
  const fetchMock = renderFlow();
  await screen.findByText("Chưa có người nhận gần đây.");
  await enterRecipient();

  const recentCall = fetchMock.mock.calls.find(([url]) =>
    String(url).includes("/api/transfers/recent-recipients?limit=4"),
  );
  expect(recentCall?.[1]).toMatchObject({
    headers: expect.objectContaining({ Authorization: `Bearer ${token}` }),
  });

  const lookupCall = fetchMock.mock.calls.find(([url]) =>
    String(url).includes("/api/accounts/lookup"),
  );
  expect(String(lookupCall?.[0])).toContain("bankCode=ANOMALY");
  expect(String(lookupCall?.[0])).toContain(`accountNo=${recipient.accountNo}`);
  expect(screen.getByText("AnomalyBank")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /Ngân hàng/ })).toBeNull();
});

it("creates the intent before opening OTP and confirms into a local transaction record", async () => {
  const fetchMock = renderFlow();
  const sheet = await createIntent();

  const createCall = fetchMock.mock.calls.find(
    ([url, init]) =>
      String(url).endsWith("/api/transfers") && init?.method === "POST",
  );
  expect(createCall?.[1]).toMatchObject({
    headers: expect.objectContaining({
      Authorization: `Bearer ${token}`,
      "Idempotency-Key": "34297a72-9433-4adf-8958-868d5e50d197",
    }),
    body: JSON.stringify({
      toBankCode: "ANOMALY",
      toAccountNo: recipient.accountNo,
      amount: 500_000,
      note: "PHAM MINH HIEU chuyen tien",
    }),
  });
  expect(within(sheet).getByText(/p\*\*\*@example.com/)).toBeTruthy();

  fireEvent.change(within(sheet).getByLabelText("Mã OTP"), {
    target: { value: "123456" },
  });
  await screen.findByRole("heading", { name: "Chuyển tiền thành công" });
  expect(screen.getByText("2.400.000₫")).toBeTruthy();
  const confirmCall = fetchMock.mock.calls.find(([url]) =>
    String(url).includes(`${intent().transferId}/confirm`),
  );
  expect(confirmCall?.[1]).toMatchObject({
    method: "POST",
    body: JSON.stringify({ otp: "123456" }),
  });
});

it("uses server attemptsLeft for invalid OTP errors", async () => {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input).includes("/confirm")) {
      return Promise.resolve(
        json(
          { error: "invalid otp", code: "INVALID_OTP", attemptsLeft: 1 },
          400,
        ),
      );
    }
    return defaultFetch(input, init);
  });
  vi.stubGlobal("fetch", fetchMock);
  render(
    <TransferFlow
      token={token}
      source={source}
      onExit={vi.fn()}
      onViewHistory={vi.fn()}
    />,
  );
  const sheet = await createIntent();
  fireEvent.change(within(sheet).getByLabelText("Mã OTP"), {
    target: { value: "000000" },
  });
  expect(
    await within(sheet).findByText("Mã OTP không đúng. Bạn còn 1 lần thử."),
  ).toBeTruthy();
  expect(within(sheet).getByText(/Còn 1 lần thử/)).toBeTruthy();
});

it("reuses the exact idempotency key and body after an ambiguous create failure", async () => {
  let creates = 0;
  const fetchMock = renderFlow((input, init) => {
    if (String(input).endsWith("/api/transfers") && init?.method === "POST") {
      creates += 1;
      if (creates === 1) return Promise.reject(new TypeError("network down"));
    }
    return defaultFetch(input, init);
  });
  await enterRecipient();
  fireEvent.change(screen.getByLabelText("Số tiền chuyển"), {
    target: { value: "500000" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Chuyển tiền" }));
  const failure = await screen.findByRole("dialog", {
    name: "Giao dịch chưa hoàn tất",
  });
  fireEvent.click(within(failure).getByRole("button", { name: "Thử lại" }));
  await screen.findByRole("dialog", { name: "Xác thực giao dịch" });

  const calls = fetchMock.mock.calls.filter(
    ([url, init]) =>
      String(url).endsWith("/api/transfers") && init?.method === "POST",
  );
  expect(calls).toHaveLength(2);
  expect(calls[1][1]?.headers).toEqual(calls[0][1]?.headers);
  expect(calls[1][1]?.body).toBe(calls[0][1]?.body);
});

it("resends through the backend when its cooldown timestamp has passed", async () => {
  const fetchMock = renderFlow((input, init) => {
    const url = String(input);
    if (url.endsWith("/api/transfers") && init?.method === "POST") {
      return Promise.resolve(
        json(
          {
            ...intent(),
            otp: otpInfo(new Date(Date.now() - 1_000).toISOString()),
          },
          201,
        ),
      );
    }
    if (url.includes("/otp/resend")) {
      return Promise.resolve(
        json(otpInfo(new Date(Date.now() + 30_000).toISOString())),
      );
    }
    return defaultFetch(input, init);
  });
  const sheet = await createIntent();
  fireEvent.click(within(sheet).getByRole("button", { name: "Gửi lại mã" }));
  expect(await within(sheet).findByText("Đã gửi lại mã OTP.")).toBeTruthy();
  expect(
    fetchMock.mock.calls.some(([url]) => String(url).includes("/otp/resend")),
  ).toBe(true);
});

it("treats an expired OTP as terminal", async () => {
  renderFlow((input, init) => {
    if (String(input).includes("/confirm")) {
      return Promise.resolve(
        json({ code: "OTP_EXPIRED", error: "expired" }, 410),
      );
    }
    return defaultFetch(input, init);
  });
  const sheet = await createIntent();
  fireEvent.change(within(sheet).getByLabelText("Mã OTP"), {
    target: { value: "123456" },
  });
  const failure = await screen.findByRole("dialog", {
    name: "Giao dịch chưa hoàn tất",
  });
  expect(within(failure).getByText(/đã hết hạn/)).toBeTruthy();
  expect(within(failure).queryByRole("button", { name: "Thử lại" })).toBeNull();
});

it("shows a retry action when recent recipients fail", async () => {
  let recentCalls = 0;
  renderFlow((input, init) => {
    if (String(input).includes("/recent-recipients")) {
      recentCalls += 1;
      if (recentCalls === 1) return Promise.reject(new TypeError("offline"));
      return Promise.resolve(json({ items: [] }));
    }
    return defaultFetch(input, init);
  });
  await screen.findByText("Không tải được người nhận gần đây.");
  fireEvent.click(screen.getByRole("button", { name: "Thử tải lại" }));
  await screen.findByText("Chưa có người nhận gần đây.");
  expect(recentCalls).toBe(2);
});
