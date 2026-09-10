import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { FormEvent, ReactNode } from "react";
import { AuthProvider } from "@/auth/AuthProvider";
import { RegistrationFlow } from "./RegistrationFlow";
import { useRegistrationFlow } from "./useRegistrationFlow";
import { AUTH_TOKEN_STORAGE_KEY } from "@/api/session";

const user = {
  id: "account-1",
  username: "Nguyen A",
  email: "a@example.com",
  isVerify: false,
};
const verified = {
  success: true,
  decision: "VERIFIED",
  reason_code: null,
  reason_message: null,
};
const front = new File(["front"], "front.png", { type: "image/png" });
const back = new File(["back"], "back.png", { type: "image/png" });
const video = new File(["video"], "live.webm", { type: "video/webm" });
const response = (body: unknown, status = 200) =>
  ({
    ok: status < 400,
    status,
    text: async () => JSON.stringify(body),
  }) as Response;
const fetchMock = vi.fn();
function wrapper({ children }: { children: ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>;
}
beforeEach(() => {
  localStorage.clear();
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal(
    "URL",
    class extends URL {
      static createObjectURL = vi.fn(() => "blob:preview");
      static revokeObjectURL = vi.fn();
    },
  );
  fetchMock.mockImplementation(async (url: string, options?: RequestInit) => {
    if (url.endsWith("/verify-face")) return response(verified);
    if (url.endsWith("/register")) return response(user, 201);
    if (url.endsWith("/login"))
      return response({ token: "login-token", user, expiresAt: "2030-01-01" });
    if (url.endsWith("/upload"))
      return response({ key: (options?.body as FormData).get("key") }, 201);
    return response({ ...user, isVerify: true });
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});
const submitEvent = { preventDefault() {} } as FormEvent;
async function prepare() {
  const hook = renderHook(useRegistrationFlow, { wrapper });
  act(() => {
    hook.result.current.setEmail("a@example.com");
    hook.result.current.setConsent(true);
    hook.result.current.setDocuments({ front, back });
    hook.result.current.setProfile({
      name: "Nguyen A",
      id: "012345678901",
      dob: "1995-01-01",
      issuedDate: "2020-01-01",
    });
    hook.result.current.setPassword("Strong123!");
    hook.result.current.setConfirm("Strong123!");
  });
  await act(async () => {
    await hook.result.current.verifyVideo(video);
  });
  return hook;
}
function count(path: string) {
  return fetchMock.mock.calls.filter(([url]) => String(url).endsWith(path))
    .length;
}
it("requires both CCCD sides and goes from email directly to documents without fake OTP", () => {
  render(<RegistrationFlow onLogin={vi.fn()} />, { wrapper });
  fireEvent.change(screen.getByRole("textbox", { name: /Địa chỉ email/ }), {
    target: { value: "a@example.com" },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
  expect(screen.queryByLabelText("Mã OTP")).toBeNull();
  fireEvent.change(screen.getByLabelText("Ảnh mặt trước CCCD"), {
    target: { files: [front] },
  });
  expect(
    (screen.getByRole("button", { name: "Tiếp tục" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  fireEvent.change(screen.getByLabelText("Ảnh mặt sau CCCD"), {
    target: { files: [back] },
  });
  expect(
    (screen.getByRole("button", { name: "Tiếp tục" }) as HTMLButtonElement)
      .disabled,
  ).toBe(false);
});
it("registers, logs in using the existing token store, uploads media and commits verification", async () => {
  const { result } = await prepare();
  expect(count("/register")).toBe(0);
  act(() => {
    result.current.submit(submitEvent);
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.screen).toBe("success"));
  expect(count("/register")).toBe(1);
  expect(count("/upload")).toBe(3);
  expect(localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBe("login-token");
  const registration = fetchMock.mock.calls.find(([url]) =>
    url.endsWith("/register"),
  )!;
  expect(JSON.parse(registration[1].body)).toEqual({
    username: "Nguyen A",
    cccdNumber: "012345678901",
    dob: "1995-01-01T00:00:00Z",
    cccdIssuedDate: "2020-01-01T00:00:00Z",
    email: "a@example.com",
    password: "Strong123!",
    idempotencyKey: expect.any(String),
  });
  const commit = fetchMock.mock.calls.find(([url]) =>
    url.endsWith("/account-1/verify"),
  )!;
  expect(commit[1].headers.Authorization).toBe("Bearer login-token");
  expect(Object.keys(JSON.parse(commit[1].body))).toEqual([
    "idCardFrontUrl",
    "idCardBackUrl",
    "liveVideoUrl",
  ]);
});
it("does not create an account on KYC failure and preserves attempts on system errors", async () => {
  fetchMock.mockResolvedValue(
    response({
      success: false,
      decision: "SYSTEM_ERROR",
      reason_message: "Unavailable",
    }),
  );
  const { result } = await prepare();
  expect(result.current.remaining).toBe(3);
  fetchMock.mockResolvedValue(
    response({
      success: false,
      decision: "RETRY_ALLOWED",
      reason_message: "Face mismatch",
    }),
  );
  for (let remaining = 2; remaining >= 0; remaining--) {
    await act(async () => {
      await result.current.verifyVideo(video);
    });
    expect(result.current.remaining).toBe(remaining);
  }
  await act(async () => {
    await result.current.verifyVideo(video);
  });
  expect(fetchMock).toHaveBeenCalledTimes(4);
  expect(count("/register")).toBe(0);
});
it("retries after login failure without creating the account a second time", async () => {
  const { result } = await prepare();
  const normal = fetchMock.getMockImplementation()!;
  let fail = true;
  fetchMock.mockImplementation(async (url, options) =>
    url.endsWith("/login") && fail ? response({}, 503) : normal(url, options),
  );
  act(() => result.current.submit(submitEvent));
  await waitFor(() => expect(result.current.error).not.toBe(""));
  expect(result.current.createdAccount?.id).toBe(user.id);
  fail = false;
  act(() => result.current.submit(submitEvent));
  await waitFor(() => expect(result.current.screen).toBe("success"));
  expect(count("/register")).toBe(1);
});
it("reuses uploaded files when the verify endpoint fails", async () => {
  const { result } = await prepare();
  const normal = fetchMock.getMockImplementation()!;
  let fail = true;
  fetchMock.mockImplementation(async (url, options) =>
    url.endsWith("/account-1/verify") && fail
      ? response({}, 500)
      : normal(url, options),
  );
  act(() => result.current.submit(submitEvent));
  await waitFor(() => expect(result.current.error).not.toBe(""));
  expect(result.current.screen).toBe("password");
  fail = false;
  act(() => result.current.submit(submitEvent));
  await waitFor(() => expect(result.current.screen).toBe("success"));
  expect(count("/upload")).toBe(3);
  expect(count("/register")).toBe(1);
});
it("aborts pending verification when leaving the page", async () => {
  let signal: AbortSignal | undefined;
  fetchMock.mockImplementation((_url, options) => {
    signal = options.signal;
    return new Promise(() => {});
  });
  const { result, unmount } = renderHook(useRegistrationFlow, { wrapper });
  act(() => result.current.setDocuments({ front, back }));
  act(() => {
    void result.current.verifyVideo(video);
  });
  unmount();
  expect(signal?.aborted).toBe(true);
});

it("reuses the registration key after a lost response and finishes onboarding", async () => {
  const { result } = await prepare();
  const normal = fetchMock.getMockImplementation()!;
  let lost = true;
  fetchMock.mockImplementation(async (url, options) => {
    if (url.endsWith("/register") && lost) {
      lost = false;
      throw new TypeError("connection lost after commit");
    }
    return normal(url, options);
  });
  act(() => result.current.submit(submitEvent));
  await waitFor(() => expect(result.current.error).not.toBe(""));
  expect(result.current.createdAccount).toBeNull();
  act(() => result.current.submit(submitEvent));
  await waitFor(() => expect(result.current.screen).toBe("success"));
  const attempts = fetchMock.mock.calls.filter(([url]) =>
    url.endsWith("/register"),
  );
  expect(attempts).toHaveLength(2);
  const first = JSON.parse(attempts[0][1].body);
  expect(first.idempotencyKey).toMatch(/^[a-f0-9-]{36}$/);
  expect(JSON.parse(attempts[1][1].body)).toEqual(first);
});
