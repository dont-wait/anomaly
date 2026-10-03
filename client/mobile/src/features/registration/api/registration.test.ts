import { describe, expect, it } from "vitest";
import { ApiError } from "@/shared/lib/http";
import { registrationError } from "./registration";

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

  it("maps invalid authentication codes to a session message", () => {
    expect(
      registrationError(
        new ApiError(401, "Unauthorized", undefined, [
          { code: "INVALID_TOKEN", detail: "invalid token" },
        ]),
      ),
    ).toBe("Phiên đăng nhập không hợp lệ. Vui lòng đăng nhập lại để tiếp tục.");
  });
});
