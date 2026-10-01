import { afterEach, expect, it, vi } from "vitest";
import {
  verifyFace,
  validateKycEndpoint,
  KYC_BASE_URL,
  LIVENESS_CHALLENGE,
} from "./kyc";
import { uploadMedia } from "./registration";
const response = (data: unknown) =>
  ({
    ok: true,
    status: 200,
    text: async () => JSON.stringify(data),
  }) as Response;
afterEach(() => vi.unstubAllGlobals());
it("sends the actual image and recorded video as multipart to KYC without leaking auth tokens", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(response({ success: true, decision: "VERIFIED" }));
  vi.stubGlobal("fetch", fetchMock);
  const front = new File(["image"], "front.png", { type: "image/png" });
  const video = new File(["video"], "live.webm", { type: "video/webm" });
  await verifyFace(front, video);
  const [url, options] = fetchMock.mock.calls[0];
  expect(url).toBe(`${KYC_BASE_URL}/v1/kyc/verify-face`);
  expect(options.body.get("cccd_front_image")).toBe(front);
  expect(options.body.get("live_video")).toBe(video);
  expect(options.body.get("challenge_type")).toBe(LIVENESS_CHALLENGE);
  expect(options.headers).not.toHaveProperty("Content-Type");
  expect(options.headers).not.toHaveProperty("Authorization");
});
it("rejects inconsistent KYC decisions instead of proceeding", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(response({ success: false, decision: "VERIFIED" })),
  );
  await expect(
    verifyFace(new File(["x"], "x.png"), new File(["v"], "v.webm")),
  ).rejects.toThrow("Phản hồi xác thực không hợp lệ");
});
it("uploads using the existing bearer token and preserves the multipart boundary", async () => {
  const fetchMock = vi.fn().mockResolvedValue(response({ key: "kyc/key" }));
  vi.stubGlobal("fetch", fetchMock);
  await uploadMedia(new File(["x"], "x.png"), "kyc/key", "token");
  const options = fetchMock.mock.calls[0][1];
  expect(options.headers).toEqual({ Authorization: "Bearer token" });
  expect(options.body.get("key")).toBe("kyc/key");
});

it.each([
  "http://example.com",
  "http://10.0.2.2:8090",
  "http://192.168.1.2:8090",
  "ftp://localhost",
  "http://localhost.evil.com",
  "https://user:password@example.com",
])("rejects unsafe KYC endpoint %s", (url) => {
  expect(() => validateKycEndpoint(url, true)).toThrow();
});
it.each([
  "http://localhost:8090",
  "http://127.0.0.1:8090",
  "http://[::1]:8090",
])("allows development loopback only: %s", (url) => {
  expect(validateKycEndpoint(url, true)).toBe(url);
  expect(() => validateKycEndpoint(url, false)).toThrow();
});
it("allows HTTPS in production", () => {
  expect(validateKycEndpoint("https://kyc.example.com/", false)).toBe(
    "https://kyc.example.com",
  );
});
