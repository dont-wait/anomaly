import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "./http";
import { isValidCccd, login, normalizeCccd, toLoginError } from "./auth";

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
  } as Response;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("auth api", () => {
  it("normalizes CCCD whitespace before login", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse(200, {
        token: "test-token",
        expiresAt: "2026-09-10T00:00:00Z",
        user: {
          id: "user-1",
          username: "anomaly",
          email: "anomaly@example.com",
          idCardFrontUrl: "",
          idCardBackUrl: "",
          liveVideoUrl: "",
          isVerify: true,
          amount: 0,
        },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const response = await login({
      cccdNumber: " 0012 3456 7890 ",
      password: "password123",
    });

    const [requestUrl, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      RequestInit?,
    ];
    expect(requestUrl).toMatch(/\/api\/auth\/login$/);
    expect(JSON.parse(init?.body as string)).toEqual({
      cccdNumber: "001234567890",
      password: "password123",
    });
    expect(response.token).toBe("test-token");
  });

  it("rejects an invalid CCCD before calling the backend", async () => {
    const fetchMock = vi.fn(async () => jsonResponse(200, {}));
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      login({ cccdNumber: "123", password: "password123" }),
    ).rejects.toThrow("Số CCCD phải gồm đúng 12 chữ số.");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("validates CCCD normalization", () => {
    expect(normalizeCccd(" 0012 3456 7890 ")).toBe("001234567890");
    expect(isValidCccd("001234567890")).toBe(true);
    expect(isValidCccd("0012-3456-7890")).toBe(false);
  });

  it("maps authentication failures to friendly messages", () => {
    expect(toLoginError(new ApiError(401, "invalid credentials"))).toBe(
      "Số CCCD hoặc mật khẩu không đúng.",
    );
    expect(toLoginError(new ApiError(0, "network error"))).toBe(
      "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.",
    );
  });
});
