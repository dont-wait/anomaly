import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/shared/lib/http";
import { registerAccount, registrationError } from "./registration";

const testPassword = ["A", "bcdefgh", "1", "!"].join("");

afterEach(() => vi.unstubAllGlobals());

it("reads the wrapped registration user and KYC credential", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      text: async () =>
        JSON.stringify({
          status: 201,
          message: "registered",
          data: {
            user: { id: "account-1", isVerify: false },
            kycToken: "kyc-token",
            kycExpiresAt: "2030-01-01T00:00:00Z",
          },
        }),
    } as Response),
  );

  const result = await registerAccount({
    username: "Nguyen A",
    cccdNumber: "012345678901",
    cccdIssuedDate: "2020-01-01T00:00:00Z",
    dob: "1995-01-01T00:00:00Z",
    email: "a@example.com",
    password: testPassword,
  });

  expect(result.user.id).toBe("account-1");
  expect(result.kycToken).toBe("kyc-token");
});

describe("registrationError", () => {
  it("maps validation codes to actionable messages", () => {
    expect(
      registrationError(
        new ApiError(400, "Bad Request", undefined, [
          { code: "INVALID_EMAIL", detail: "invalid email" },
        ]),
      ),
    ).toBe("Địa chỉ email không hợp lệ. Vui lòng kiểm tra lại.");

    expect(
      registrationError(
        new ApiError(400, "Bad Request", undefined, [
          { code: "WEAK_PASSWORD", detail: "password is too short" },
        ]),
      ),
    ).toBe("Mật khẩu phải có ít nhất 8 ký tự.");
  });

  it("maps invalid authentication codes to a KYC session message", () => {
    expect(
      registrationError(
        new ApiError(401, "Unauthorized", undefined, [
          { code: "INVALID_TOKEN", detail: "invalid token" },
        ]),
      ),
    ).toBe(
      "Phiên xác thực danh tính không hợp lệ. Vui lòng đăng nhập để tiếp tục.",
    );
  });
});
