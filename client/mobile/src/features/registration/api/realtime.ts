import { API_BASE_URL } from "@/shared/constants";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { requestApi } from "@/shared/lib/http";
import { LIVENESS_CHALLENGE } from "./kyc";

export interface CaptureIdentity {
  attemptId: string;
  idempotencyKey: string;
}
export interface CaptureProgress {
  challengeStep: number;
  challengePassed: boolean;
  faceState: string;
  qualityState: string;
  matchingState: string;
}
export interface RealtimeCapture extends CaptureIdentity {
  attemptLimit?: number;
  attemptsRemaining?: number;
  mimeTypes: string[];
  start: (mimeType: string) => Promise<void>;
  send: (chunk: Blob) => void;
  finish: () => Promise<void>;
  dispose: (cancelAttempt?: boolean) => void;
}

interface Attempt {
  attemptLimit?: number;
  attemptsRemaining?: number;
  attemptId: string;
  streamPath: string;
  streamTicket: string;
  mimeTypes: string[];
}

export async function openRealtimeKyc(
  front: File,
  back: File,
  token: string,
  onProgress: (progress: CaptureProgress) => void,
  onError: (error: Error) => void,
  signal: AbortSignal,
): Promise<RealtimeCapture> {
  const idempotencyKey = crypto.randomUUID();
  const body = new FormData();
  body.append("idCardFront", front);
  body.append("idCardBack", back);
  body.append("challengeType", LIVENESS_CHALLENGE);
  const attempt = await requestApi<Attempt>(API_ENDPOINTS.KYC.REALTIME, {
    method: "POST",
    body,
    token,
    signal,
    headers: { "Idempotency-Key": idempotencyKey },
    timeoutMs: 30000,
  });
  if (
    !attempt?.attemptId ||
    !attempt.streamPath ||
    !attempt.streamTicket ||
    !Array.isArray(attempt.mimeTypes)
  )
    throw new Error("Phản hồi camera không hợp lệ.");
  // Never send a credential to an endpoint supplied by another origin.
  const base = new URL(API_BASE_URL, window.location.href);
  const url = new URL(attempt.streamPath, base);
  if (
    url.origin !== base.origin ||
    !url.pathname.startsWith(`${API_ENDPOINTS.KYC.REALTIME}/`)
  )
    throw new Error("Địa chỉ camera không hợp lệ.");
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  const socket = new WebSocket(url);
  let disposed = false;
  let finished = false;
  let failed: Error | undefined;
  let seq = 0;
  let eventSeq = 0;
  let total = 0;
  let chain = Promise.resolve();
  const pending = new Map<number, number>();
  let readyResolve!: () => void;
  let startResolve!: () => void;
  let finishResolve!: () => void;
  let readyReject!: (e: Error) => void;
  let startReject!: (e: Error) => void;
  let finishReject!: (e: Error) => void;
  const ready = new Promise<void>((resolve, reject) => {
    readyResolve = resolve;
    readyReject = reject;
  });
  const started = new Promise<void>((resolve, reject) => {
    startResolve = resolve;
    startReject = reject;
  });
  const ended = new Promise<void>((resolve, reject) => {
    finishResolve = resolve;
    finishReject = reject;
  });
  // Stages may fail before their consumer reaches the await.
  void started.catch(() => {});
  void ended.catch(() => {});
  const cancelRemote = () => {
    void requestApi(
      `${API_ENDPOINTS.KYC.REALTIME}/${encodeURIComponent(attempt.attemptId)}`,
      {
        method: "DELETE",
        token,
      },
    ).catch(() => {});
  };
  const dispose = (cancelAttempt = true) => {
    if (disposed) return;
    disposed = true;
    clearTimeout(deadline);
    clearInterval(watchdog);
    signal.removeEventListener("abort", abort);
    socket.close();
    pending.clear();
    if (cancelAttempt) cancelRemote();
  };
  const fail = (error: Error) => {
    if (failed || disposed || finished) return;
    failed = error;
    readyReject(error);
    startReject(error);
    finishReject(error);
    dispose();
    onError(error);
  };
  const abort = () => fail(new Error("Đã hủy quay video."));
  const deadline = setTimeout(
    () => fail(new Error("Không thể chuẩn bị camera xác thực.")),
    20000,
  );
  const watchdog = setInterval(() => {
    const oldest = pending.values().next().value;
    if (
      socket.bufferedAmount > 1024 * 1024 ||
      (oldest !== undefined && performance.now() - oldest > 1500)
    )
      fail(
        new Error("Kết nối quá chậm để xác thực trực tiếp. Vui lòng quay lại."),
      );
  }, 250);
  signal.addEventListener("abort", abort, { once: true });
  socket.onopen = () => {
    if (signal.aborted) {
      abort();
      return;
    }
    socket.send(
      JSON.stringify({ type: "authenticate", ticket: attempt.streamTicket }),
    );
  };
  socket.onerror = () =>
    fail(new Error("Không thể kết nối xác thực trực tiếp."));
  socket.onclose = () => {
    if (!finished)
      fail(new Error("Kết nối camera đã ngắt. Vui lòng quay lại."));
  };
  socket.onmessage = (message) => {
    if (disposed || failed) return;
    try {
      const event = JSON.parse(String(message.data));
      if (
        event.attemptId !== attempt.attemptId ||
        !Number.isInteger(event.eventSeq) ||
        event.eventSeq <= eventSeq
      )
        throw new Error("Phản hồi xác thực không hợp lệ.");
      eventSeq = event.eventSeq;
      if (event.type === "ready") {
        clearTimeout(deadline);
        readyResolve();
      } else if (event.type === "started") startResolve();
      else if (event.type === "ack") pending.delete(event.chunkSeq);
      else if (event.type === "progress") {
        if (
          !Number.isInteger(event.challengeStep) ||
          event.challengeStep < 0 ||
          event.challengeStep > 3 ||
          typeof event.challengePassed !== "boolean"
        )
          throw new Error("Tiến độ xác thực không hợp lệ.");
        onProgress(event);
      } else if (event.type === "stream_finished") {
        finished = true;
        clearInterval(watchdog);
        finishResolve();
      } else if (event.type === "error") {
        const documentError =
          event.code === "CCCD_PORTRAIT_NOT_FOUND" ||
          event.code === "CCCD_IMAGE_QUALITY_LOW";
        fail(
          new Error(
            documentError
              ? "Ảnh căn cước chưa rõ chân dung. Vui lòng chụp lại CCCD."
              : "Dịch vụ xác thực chưa sẵn sàng. Vui lòng thử lại.",
          ),
        );
      }
    } catch (error) {
      fail(
        error instanceof Error
          ? error
          : new Error("Phản hồi xác thực không hợp lệ."),
      );
    }
  };
  if (signal.aborted) abort();
  await ready;
  return {
    attemptLimit: attempt.attemptLimit,
    attemptsRemaining: attempt.attemptsRemaining,
    attemptId: attempt.attemptId,
    idempotencyKey,
    mimeTypes: attempt.mimeTypes,
    async start(mimeType) {
      if (failed || disposed)
        throw failed ?? new Error("Phiên camera đã đóng.");
      socket.send(JSON.stringify({ type: "start", mimeType }));
      await Promise.race([
        started,
        new Promise<never>((_, reject) => {
          const timer = setTimeout(() => {
            const error = new Error("Không thể bắt đầu xác thực.");
            fail(error);
            reject(error);
          }, 5000);
          void started.finally(() => clearTimeout(timer)).catch(() => {});
        }),
      ]);
    },
    send(chunk) {
      if (!chunk.size || failed || disposed) return;
      total += chunk.size;
      if (total > 25 * 1024 * 1024 || chunk.size > 1024 * 1024) {
        fail(
          new Error(
            "Video vượt giới hạn ghi hình trực tiếp. Vui lòng quay lại.",
          ),
        );
        return;
      }
      chain = chain
        .then(async () => {
          if (failed || disposed) return;
          const payload = new Uint8Array(5 + chunk.size);
          payload[0] = 1;
          new DataView(payload.buffer).setUint32(1, seq);
          payload.set(new Uint8Array(await chunk.arrayBuffer()), 5);
          if (failed || disposed) return;
          pending.set(seq++, performance.now());
          socket.send(payload);
        })
        .catch((error) =>
          fail(
            error instanceof Error ? error : new Error("Gửi video thất bại."),
          ),
        );
    },
    async finish() {
      await chain;
      if (failed || disposed)
        throw failed ?? new Error("Phiên camera đã đóng.");
      socket.send(JSON.stringify({ type: "end", lastChunkSeq: seq - 1 }));
      const timer = setTimeout(
        () => fail(new Error("Xử lý video quá thời gian. Vui lòng quay lại.")),
        7000,
      );
      try {
        await ended;
      } finally {
        clearTimeout(timer);
      }
    },
    dispose,
  };
}
