import { afterEach, expect, it, vi } from "vitest";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { completeKyc, LIVENESS_CHALLENGE } from "./kyc";

const response = (data: unknown, status = 200) =>
  ({
    ok: status < 400,
    status,
    text: async () => JSON.stringify(data),
  }) as Response;

afterEach(() => vi.unstubAllGlobals());

it("sends all KYC evidence to the backend as authenticated multipart", async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    response({
      status: 200,
      message: "completed",
      data: {
        decision: "VERIFIED",
        user: { id: "account-1", isVerify: true },
      },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const front = new File(["front"], "front.png", { type: "image/png" });
  const back = new File(["back"], "back.png", { type: "image/png" });
  const video = new File(["video"], "live.webm", { type: "video/webm" });

  await completeKyc(front, back, video, "kyc-token");

  const [url, options] = fetchMock.mock.calls[0];
  expect(url).toMatch(new RegExp(`${API_ENDPOINTS.KYC.COMPLETE}$`));
  expect(options.body.get("idCardFront")).toBe(front);
  expect(options.body.get("idCardBack")).toBe(back);
  expect(options.body.get("liveVideo")).toBe(video);
  expect(options.body.get("challengeType")).toBe(LIVENESS_CHALLENGE);
  expect(options.headers).toEqual({ Authorization: "Bearer kyc-token" });
});

it("accepts a wrapped 502 SYSTEM_ERROR without consuming an attempt", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      response(
        {
          status: 502,
          message: "upstream unavailable",
          data: { decision: "SYSTEM_ERROR", reasonMessage: "Unavailable" },
        },
        502,
      ),
    ),
  );

  await expect(
    completeKyc(
      new File(["f"], "front.png"),
      new File(["b"], "back.png"),
      new File(["v"], "live.webm"),
      "token",
    ),
  ).resolves.toEqual({
    decision: "SYSTEM_ERROR",
    reasonMessage: "Unavailable",
  });
});

it("rejects VERIFIED without a verified user", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      response({ status: 200, message: "completed", data: { decision: "VERIFIED" } }),
    ),
  );
  await expect(
    completeKyc(
      new File(["f"], "front.png"),
      new File(["b"], "back.png"),
      new File(["v"], "live.webm"),
      "token",
    ),
  ).rejects.toThrow("Phản hồi xác thực không hợp lệ");
});
