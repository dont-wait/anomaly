import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { FormEvent } from "react";
import { ApiError } from "@/shared/lib/http";
import { KYC_RESUME_STORAGE_KEY } from "@/features/registration/kycResume";
import { useLoginForm } from "./useLoginForm";

const authLogin = vi.fn();
const testPassword = ["A", "bcdefgh", "1", "!"].join("");
vi.mock("@/features/auth/useAuth", () => ({
  useAuth: () => ({
    status: "unauthenticated",
    user: null,
    login: authLogin,
    logout: vi.fn(),
  }),
}));

beforeEach(() => {
  authLogin.mockReset();
  sessionStorage.clear();
  window.location.hash = "#/login";
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  sessionStorage.clear();
});

it("resumes KYC without establishing an auth session when login requires KYC", async () => {
  authLogin.mockRejectedValue(
    new ApiError(403, "KYC required", { code: "KYC_REQUIRED" }),
  );
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () =>
      JSON.stringify({
        status: 200,
        message: "KYC session created",
        data: {
          user: { id: "account-1", isVerify: false },
          kycToken: "kyc-token",
          kycExpiresAt: "2030-01-01T00:00:00Z",
        },
      }),
  } as Response);
  vi.stubGlobal("fetch", fetchMock);
  const { result } = renderHook(useLoginForm);
  act(() => {
    result.current.setCccd("012345678901");
    result.current.setPassword(testPassword);
  });

  await act(async () => {
    await result.current.handleSubmit({ preventDefault() {} } as FormEvent<HTMLFormElement>);
  });

  expect(authLogin).toHaveBeenCalledTimes(1);
  expect(fetchMock.mock.calls[0][0]).toMatch(/\/api\/kyc\/session$/);
  expect(result.current.password).toBe("");
  expect(window.location.hash).toBe("#/register");
  const persisted = sessionStorage.getItem(KYC_RESUME_STORAGE_KEY)!;
  expect(persisted).toContain("kyc-token");
  expect(persisted).not.toContain(testPassword);
});
