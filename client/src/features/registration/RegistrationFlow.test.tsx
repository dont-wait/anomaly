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
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
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
    if (String(url).endsWith(API_ENDPOINTS.AUTH.OTP_REQUEST))
      return response({ message: "otp sent" });
    if (String(url).endsWith(API_ENDPOINTS.AUTH.OTP_VERIFY))
      return response({ verified: true });
    if (String(url).endsWith(API_ENDPOINTS.KYC.VERIFY_FACE))
      return response(verified);
    if (String(url).endsWith(API_ENDPOINTS.AUTH.REGISTER))
      return response(user, 201);
    if (String(url).endsWith(API_ENDPOINTS.AUTH.LOGIN))
      return response({ token: "login-token", user, expiresAt: "2030-01-01" });
    if (String(url).endsWith(API_ENDPOINTS.MEDIA.UPLOAD))
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
  expect(count(API_ENDPOINTS.AUTH.OTP_REQUEST)).toBe(1);
  expect(
    (screen.getByRole("button", { name: /Xác thực/ }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  // Dán mã điền đủ 6 ô và tự xác thực, không cần bấm nút.
  fireEvent.paste(firstBox, { clipboardData: { getData: () => "12a34b56" } });

  await screen.findByLabelText("Ảnh mặt trước CCCD");
  const otpRequest = fetchMock.mock.calls.find(([url]) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.OTP_VERIFY),
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
// Gửi mã treo lại cho tới khi gọi release(), để kiểm tra trạng thái "đang gửi".
function deferSend() {
  let release: (() => void) | undefined;
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) => {
    if (!String(url).endsWith(API_ENDPOINTS.AUTH.OTP_REQUEST))
      return normal(url, options);
    await new Promise<void>((resolve) => {
      release = resolve;
    });
    return response({ message: "otp sent" });
  });
  return () => release!();
}
function openOtpScreen() {
  render(<RegistrationFlow onLogin={vi.fn()} />, { wrapper });
  fireEvent.change(screen.getByRole("textbox", { name: /Địa chỉ email/ }), {
    target: { value: "a@example.com" },
  });
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
}
const otpBox = (position: number) =>
  screen.getByLabelText(
    `Chữ số thứ ${position} của mã OTP`,
  ) as HTMLInputElement;

it("lets the user type while the code is still being sent", async () => {
  const release = deferSend();
  openOtpScreen();

  // Ô nhập mở ngay, không bắt chờ mã gửi xong.
  expect(otpBox(1).disabled).toBe(false);
  expect(screen.queryByRole("status")).toBeNull();
  expect(
    (screen.getByRole("button", { name: /Gửi lại/ }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);

  await act(async () => {
    release();
  });
  await waitFor(() => expect(otpBox(1).disabled).toBe(false));
});

it("waits for the send to finish before verifying a code typed early", async () => {
  const release = deferSend();
  openOtpScreen();

  fireEvent.paste(otpBox(1), { clipboardData: { getData: () => "123456" } });
  expect(otpBox(6).value).toBe("6");
  // Mã đủ nhưng chưa gửi xong: chờ, tuyệt đối không bỏ qua bước gọi server.
  expect(count(API_ENDPOINTS.AUTH.OTP_VERIFY)).toBe(0);
  expect(screen.getByRole("status").textContent).toBe(
    "Đã nhập đủ mã, đang chờ gửi xong để xác thực…",
  );

  await act(async () => {
    release();
  });
  await screen.findByLabelText("Ảnh mặt trước CCCD");
  expect(count(API_ENDPOINTS.AUTH.OTP_VERIFY)).toBe(1);
});

it("starts the resend cooldown when the send begins, not when it returns", async () => {
  const release = deferSend();
  const { result } = renderHook(useRegistrationFlow, { wrapper });
  act(() => {
    result.current.setEmail("a@example.com");
    result.current.setConsent(true);
  });
  await act(async () => {
    result.current.submit(submitEvent);
  });

  expect(result.current.screen).toBe("otp");
  expect(result.current.otpSending).toBe(true);
  expect(result.current.otpSent).toBe(false);
  expect(result.current.resendIn).toBe(30);

  await act(async () => {
    release();
  });
  await waitFor(() => expect(result.current.otpSent).toBe(true));
  expect(result.current.resendIn).toBe(30);
});

it("keeps a code typed during a send that then fails, and reopens resend", async () => {
  let fail: (() => void) | undefined;
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) => {
    if (!String(url).endsWith(API_ENDPOINTS.AUTH.OTP_REQUEST))
      return normal(url, options);
    await new Promise<void>((resolve) => {
      fail = resolve;
    });
    return response({ error: "smtp down" }, 500);
  });
  const { result } = renderHook(useRegistrationFlow, { wrapper });
  act(() => {
    result.current.setEmail("a@example.com");
    result.current.setConsent(true);
  });
  await act(async () => {
    result.current.submit(submitEvent);
  });
  act(() => result.current.setOtpCode("123456"));

  await act(async () => {
    fail!();
  });
  await waitFor(() => expect(result.current.otpErrorMsg).not.toBe(""));
  expect(result.current.otpCode).toBe("123456");
  expect(result.current.otpSent).toBe(false);
  expect(result.current.resendIn).toBe(0);
  expect(count(API_ENDPOINTS.AUTH.OTP_VERIFY)).toBe(0);
});

it("does not verify manually when no send has succeeded", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.OTP_REQUEST)
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
  await waitFor(() => expect(result.current.otpErrorMsg).not.toBe(""));
  expect(result.current.otpSent).toBe(false);
  act(() => result.current.setOtpCode("123456"));
  // Bấm tay khi chưa gửi thành công: chặn, không gọi /otp/verify vô ích.
  await act(async () => {
    result.current.submit(submitEvent);
  });
  expect(count(API_ENDPOINTS.AUTH.OTP_VERIFY)).toBe(0);
  expect(result.current.otpCode).toBe("123456");
});
it("tells the truth about the send state in the OTP heading", async () => {
  const release = deferSend();
  openOtpScreen();
  // Đang gửi: không khẳng định "đã gửi".
  expect(screen.getByText(/đang được gửi tới/)).toBeTruthy();
  expect(screen.queryByText(/đã được gửi tới/)).toBeNull();
  await act(async () => {
    release();
  });
  await waitFor(() => expect(screen.getByText(/đã được gửi tới/)).toBeTruthy());
});
it("admits a failed send in the OTP heading", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.OTP_REQUEST)
      ? response({ error: "smtp down" }, 500)
      : normal(url, options),
  );
  openOtpScreen();
  await waitFor(() =>
    expect(screen.getByText(/Chưa gửi được mã/)).toBeTruthy(),
  );
});
it("reports a failed send on the OTP screen instead of holding back the email step", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.OTP_REQUEST)
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
    expect(count(API_ENDPOINTS.AUTH.OTP_REQUEST)).toBe(1);
  } finally {
    vi.useRealTimers();
  }
});
it("sends a new code and restarts the cooldown once it has elapsed", async () => {
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
    expect(result.current.resendIn).toBe(30);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });
    expect(result.current.resendIn).toBe(0);

    await act(async () => {
      await result.current.resendOtp();
    });
    expect(count(API_ENDPOINTS.AUTH.OTP_REQUEST)).toBe(2);
    expect(result.current.resendIn).toBe(30);
    expect(result.current.otpErrorMsg).toBe("");
  } finally {
    vi.useRealTimers();
  }
});
it("keeps the user on the OTP screen and explains an expired code", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.OTP_VERIFY)
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
  expect(count(API_ENDPOINTS.AUTH.OTP_VERIFY)).toBe(1);
});
it("keeps a rejected code on screen so the user can correct it", async () => {
  const normal = fetchMock.getMockImplementation()!;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.OTP_VERIFY)
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
  expect(count(API_ENDPOINTS.AUTH.REGISTER)).toBe(0);
  act(() => {
    result.current.submit(submitEvent);
    result.current.submit(submitEvent);
  });
  await waitFor(() => expect(result.current.screen).toBe("success"));
  expect(count(API_ENDPOINTS.AUTH.REGISTER)).toBe(1);
  expect(count(API_ENDPOINTS.MEDIA.UPLOAD)).toBe(3);
  expect(localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)).toBe("login-token");
  const registration = fetchMock.mock.calls.find(([url]) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.REGISTER),
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
    String(url).endsWith(API_ENDPOINTS.ACCOUNTS.VERIFY("account-1")),
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
  expect(count(API_ENDPOINTS.AUTH.REGISTER)).toBe(0);
});
it("retries after login failure without creating the account a second time", async () => {
  const { result } = await prepare();
  const normal = fetchMock.getMockImplementation()!;
  let fail = true;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith(API_ENDPOINTS.AUTH.LOGIN) && fail
      ? response({}, 503)
      : normal(url, options),
  );
  act(() => result.current.submit(submitEvent));
  await screen.findByRole("alert");
  expect(result.current.error).toBe("");
  expect(result.current.createdAccount?.id).toBe(user.id);
  fail = false;
  act(() => result.current.submit(submitEvent));
  await waitFor(() => expect(result.current.screen).toBe("success"));
  expect(count(API_ENDPOINTS.AUTH.REGISTER)).toBe(1);
});
it("reuses uploaded files when the verify endpoint fails", async () => {
  const { result } = await prepare();
  const normal = fetchMock.getMockImplementation()!;
  let fail = true;
  fetchMock.mockImplementation(async (url, options) =>
    String(url).endsWith(API_ENDPOINTS.ACCOUNTS.VERIFY("account-1")) && fail
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
  expect(count(API_ENDPOINTS.MEDIA.UPLOAD)).toBe(3);
  expect(count(API_ENDPOINTS.AUTH.REGISTER)).toBe(1);
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
    if (String(url).endsWith(API_ENDPOINTS.AUTH.REGISTER) && lost) {
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
    String(url).endsWith(API_ENDPOINTS.AUTH.REGISTER),
  );
  expect(attempts).toHaveLength(2);
  const first = JSON.parse(attempts[0][1].body);
  expect(first.idempotencyKey).toMatch(/^[a-f0-9-]{36}$/);
  expect(JSON.parse(attempts[1][1].body)).toEqual(first);
});
