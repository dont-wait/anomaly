import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  KYC_RESUME_STORAGE_KEY,
  clearKycResumeSession,
  restoreKycResumeSession,
  saveKycResumeSession,
} from "./kycResume";

const user = {
  id: "account-1",
  accountNo: "",
  username: "Nguyen A",
  email: "a@example.com",
  currency: "VND",
  idCardFrontUrl: "",
  idCardBackUrl: "",
  liveVideoUrl: "",
  isVerify: false,
  amount: 0,
};

beforeEach(() => sessionStorage.clear());
afterEach(() => {
  vi.useRealTimers();
  sessionStorage.clear();
});

it("stores only resumable KYC and non-secret account metadata", () => {
  saveKycResumeSession({
    kycToken: "kyc-token",
    kycExpiresAt: "2030-01-01T00:00:00Z",
    user,
    profile: {
      name: "Nguyen A",
      id: "012345678901",
      dob: "1995-01-01",
      issuedDate: "2020-01-01",
      email: "a@example.com",
    },
  });

  const raw = sessionStorage.getItem(KYC_RESUME_STORAGE_KEY)!;
  expect(raw).toContain("kyc-token");
  expect(raw).not.toContain("Strong123!");
  expect(raw.toLowerCase()).not.toContain("password");
  expect(restoreKycResumeSession().session?.user.id).toBe("account-1");
});

it("clears expired resume state", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2030-01-02T00:00:00Z"));
  sessionStorage.setItem(
    KYC_RESUME_STORAGE_KEY,
    JSON.stringify({
      kycToken: "expired",
      kycExpiresAt: "2030-01-01T00:00:00Z",
      user,
    }),
  );

  expect(restoreKycResumeSession()).toEqual({ session: null, expired: true });
  expect(sessionStorage.getItem(KYC_RESUME_STORAGE_KEY)).toBeNull();
  clearKycResumeSession();
});
