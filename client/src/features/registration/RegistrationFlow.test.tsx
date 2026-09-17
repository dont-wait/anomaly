import { Toaster } from "@/shared/notifications/Toaster";
import { toast } from "@/shared/notifications/toast";
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
import { AuthProvider, AUTH_TOKEN_STORAGE_KEY } from "@/features/auth";
import { RegistrationFlow } from "./RegistrationFlow";
import { useRegistrationFlow } from "./useRegistrationFlow";

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
  return (
    <AuthProvider>
      {children}
      <Toaster />
    </AuthProvider>
  );
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
    if (url.endsWith("/otp/request")) return response({ message: "otp sent" });
    if (url.endsWith("/otp/verify")) return response({ verified: true });
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
  toast.dismiss();
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
it("verifies the emailed OTP between the email and document steps", async () => {
  render(<RegistrationFlow onLogin={vi.fn()} />, { wrapper });
  fireEvent.change(screen.getByRole("textbox", { name: /Địa chỉ email/ }), {
    target: { value: "a@example.com" },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  expect(screen.queryByLabelText("Chữ số thứ 1 của mã OTP")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));

  const firstBox = await screen.findByLabelText("Chữ số thứ 1 của mã OTP");
  expect(count("/otp/request")).toBe(1);
  expect(
    (screen.getByRole("button", { name: /Xác thực/ }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  // Dán mã điền đủ 6 ô và tự xác thực, không cần bấm nút.
  fireEvent.paste(firstBox, { clipboardData: { getData: () => "12a34b56" } });

  await screen.findByLabelText("Ảnh mặt trước CCCD");
  const otpRequest = fetchMock.mock.calls.find(([url]) =>
    String(url).endsWith("/otp/verify"),
  )!;
  expect(JSON.parse(String(otpRequest[1].body))).toEqual({
    email: "a@example.com",
    code: "123456",
  });
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
it("opens the OTP screen before the code has been sent", async () => {
  let release: (() => void) | undefined;
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) => {
    if (!String(url).endsWith("/otp/request")) return normal(url, options);
    await new Promise<void>((resolve) => {
      release = resolve;
    });
    return response({ message: "otp sent" });
  });
  render(<RegistrationFlow onLogin={vi.fn()} />, { wrapper });
  fireEvent.change(screen.getByRole("textbox", { name: /Địa chỉ email/ }), {
    target: { value: "a@example.com" },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));

  // Màn OTP đã hiển thị dù request gửi mã còn đang treo.
  const firstBox = screen.getByLabelText(
    "Chữ số thứ 1 của mã OTP",
  ) as HTMLInputElement;
  expect(firstBox.disabled).toBe(true);
  expect(screen.getByRole("status").textContent).toBe("Đang gửi mã…");

  await act(async () => {
    release!();
  });
  await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  expect(
    (screen.getByLabelText("Chữ số thứ 1 của mã OTP") as HTMLInputElement)
      .disabled,
  ).toBe(false);
});
it("reports a failed send on the OTP screen instead of holding back the email step", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith("/otp/request")
      ? response({ error: "smtp down" }, 500)
      : normal(url, options),
  );
  const { result } = renderHook(useRegistrationFlow, { wrapper });
  act(() => {
    result.current.setEmail("a@example.com");
    result.current.setConsent(true);
  });
  await act(async () => {
    result.current.submit(submitEvent);
  });
  expect(result.current.screen).toBe("otp");
  await waitFor(() =>
    expect(result.current.otpErrorMsg).toBe(
      "Máy chủ đang bận. Vui lòng thử lại.",
    ),
  );
  expect(result.current.resendIn).toBe(0);
});
it("starts a 30s resend cooldown and counts it down on the OTP screen", async () => {
  vi.useFakeTimers();
  try {
    const { result } = renderHook(useRegistrationFlow, { wrapper });
    act(() => {
      result.current.setEmail("a@example.com");
      result.current.setConsent(true);
    });
    await act(async () => {
      result.current.submit(submitEvent);
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.screen).toBe("otp");
    expect(result.current.resendIn).toBe(30);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(result.current.resendIn).toBe(27);

    await act(async () => {
      await result.current.resendOtp();
    });
    expect(count("/otp/request")).toBe(1);
  } finally {
    vi.useRealTimers();
  }
});
it("keeps the user on the OTP screen and explains an expired code", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith("/otp/verify")
      ? response({ error: "otp expired" }, 410)
      : normal(url, options),
  );
  const { result } = renderHook(useRegistrationFlow, { wrapper });
  act(() => {
    result.current.setEmail("a@example.com");
    result.current.setConsent(true);
  });
  await act(async () => {
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.screen).toBe("otp"));
  act(() => result.current.setOtpCode("123456"));
  await act(async () => {
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.otpErrorMsg).not.toBe(""));
  expect(result.current.screen).toBe("otp");
  expect(result.current.otpErrorMsg).toContain("gửi lại mã mới");
  expect(result.current.otpCode).toBe("");
});
it("clears the consumed code so stepping back to OTP does not verify again", async () => {
  const { result } = renderHook(useRegistrationFlow, { wrapper });
  act(() => {
    result.current.setEmail("a@example.com");
    result.current.setConsent(true);
  });
  await act(async () => {
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.screen).toBe("otp"));
  act(() => result.current.setOtpCode("123456"));
  await act(async () => {
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.screen).toBe("document"));
  expect(result.current.otpCode).toBe("");

  // Nút Back của flow đưa về màn OTP: không còn mã nên không tự xác thực lại.
  act(() => result.current.go("otp"));
  expect(result.current.screen).toBe("otp");
  expect(result.current.otpCode).toBe("");
  await act(async () => {
    result.current.submit(submitEvent);
  });
  expect(count("/otp/verify")).toBe(1);
});
it("keeps a rejected code on screen so the user can correct it", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith("/otp/verify")
      ? response({ error: "invalid otp" }, 400)
      : normal(url, options),
  );
  const { result } = renderHook(useRegistrationFlow, { wrapper });
  act(() => {
    result.current.setEmail("a@example.com");
    result.current.setConsent(true);
  });
  await act(async () => {
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.screen).toBe("otp"));
  act(() => result.current.setOtpCode("123456"));
  await act(async () => {
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.otpErrorMsg).not.toBe(""));
  expect(result.current.otpCode).toBe("123456");
  expect(result.current.otpErrorMsg).toContain("Mã OTP không đúng");
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
  await screen.findByRole("alert");
  expect(result.current.error).toBe("");
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
  await screen.findByRole("alert");
  expect(result.current.error).toBe("");
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
  await screen.findByRole("alert");
  expect(result.current.error).toBe("");
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
