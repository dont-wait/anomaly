import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/shared/lib/http";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { otpError, requestOtp, verifyOtp } from "./otp";

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
  } as Response;
}

function bodyOf(fetchMock: ReturnType<typeof vi.fn>): unknown {
  const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit?];
  return JSON.parse(String(init?.body));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("requestOtp", () => {
  it("posts the trimmed email and returns the server message", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(200, { message: "otp sent" }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(requestOtp("  alice@example.com  ")).resolves.toBe("otp sent");

    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit?,
    ];
    expect(url).toContain(API_ENDPOINTS.AUTH.OTP_REQUEST);
    expect(init?.method).toBe("POST");
    expect(bodyOf(fetchMock)).toEqual({ email: "alice@example.com" });
  });

  it("rejects a response without a message string", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(200, { message: 123 })),
    );

    await expect(requestOtp("alice@example.com")).rejects.toMatchObject({
      status: 0,
      message: "Phản hồi gửi mã OTP không hợp lệ.",
    });
  });
});

describe("verifyOtp", () => {
  it("posts the trimmed email and code, then returns true", async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, { verified: true }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(verifyOtp(" alice@example.com ", " 123456 ")).resolves.toBe(
      true,
    );

    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit?,
    ];
    expect(url).toContain(API_ENDPOINTS.AUTH.OTP_VERIFY);
    expect(init?.method).toBe("POST");
    expect(bodyOf(fetchMock)).toEqual({
      email: "alice@example.com",
      code: "123456",
    });
  });

  it("rejects when the server does not confirm verification", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(200, { verified: false })),
    );

    await expect(
      verifyOtp("alice@example.com", "123456"),
    ).rejects.toMatchObject({
      status: 0,
      message: "Phản hồi xác thực OTP không hợp lệ.",
    });
  });

  it("surfaces the server status so the caller can map it", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse(410, { error: "otp expired" })),
    );

    await expect(
      verifyOtp("alice@example.com", "123456"),
    ).rejects.toMatchObject({ status: 410 });
  });
});

describe("otpError", () => {
  it("maps 400 to a wrong-code message", () => {
    expect(otpError(new ApiError(400, "bad request"))).toContain(
      "Mã OTP không đúng",
    );
  });

  it("maps 410 to an expired message asking for a new code", () => {
    expect(otpError(new ApiError(410, "gone"))).toContain("hết hạn");
    expect(otpError(new ApiError(410, "gone"))).toContain("gửi lại mã mới");
  });

  it("maps 5xx to a server-busy message", () => {
    expect(otpError(new ApiError(500, "boom"))).toBe(
      "Máy chủ đang bận. Vui lòng thử lại.",
    );
    expect(otpError(new ApiError(503, "boom"))).toBe(
      "Máy chủ đang bận. Vui lòng thử lại.",
    );
  });

  it("reports a network failure as a connectivity problem", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("network down");
      }),
    );

    const error = await requestOtp("alice@example.com").catch(
      (err: unknown) => err,
    );

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(0);
    expect(otpError(error)).toBe(
      "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.",
    );
  });

  it("falls back for non-ApiError values", () => {
    expect(otpError("boom")).toBe(
      "Không thể hoàn tất yêu cầu. Vui lòng thử lại.",
    );
  });
});
