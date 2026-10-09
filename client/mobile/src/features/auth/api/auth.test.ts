import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/shared/lib/http";
import {
  createKycSession,
  isValidCccd,
  login,
  normalizeCccd,
  toLoginError,
} from "./auth";

function jsonResponse(status: number, body: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => JSON.stringify(body),
  } as Response;
}

const testPassword = ["A", "bcdefgh", "1", "!"].join("");

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("auth api", () => {
  it("normalizes CCCD whitespace before login", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      jsonResponse(200, {
        status: 200,
        message: "Login successful",
        data: {
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

  it("uses the server error code when mapping login failures", () => {
    expect(
      toLoginError(
        new ApiError(400, "Bad Request", undefined, [
          { code: "INVALID_CREDENTIALS", detail: "invalid credentials" },
        ]),
      ),
    ).toBe("Số CCCD hoặc mật khẩu không đúng.");
  });

  it("creates a purpose-limited KYC session with the entered credentials", async () => {
    const fetchMock = vi.fn<typeof fetch>(async () =>
      jsonResponse(200, {
        status: 200,
        message: "KYC session created",
        data: {
          user: { id: "user-1", isVerify: false },
          kycToken: "kyc-token",
          kycExpiresAt: "2030-01-01T00:00:00Z",
        },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await createKycSession({
      cccdNumber: "001234567890",
      password: testPassword,
    });

    expect(fetchMock.mock.calls[0]?.[0]).toMatch(/\/api\/kyc\/session$/);
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      cccdNumber: "001234567890",
      password: testPassword,
    });
  });
});
