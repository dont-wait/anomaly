import { afterEach, expect, it, vi } from "vitest";
import { requestApi } from "@/shared/lib/http";
import { openRealtimeKyc } from "./realtime";

vi.mock("@/shared/lib/http", () => ({ requestApi: vi.fn() }));

class Socket {
  static instance: Socket;
  bufferedAmount = 0;
  sent: unknown[] = [];
  onopen?: () => void;
  onmessage?: (event: { data: string }) => void;
  onclose?: () => void;
  onerror?: () => void;
  close = vi.fn();
  constructor() {
    Socket.instance = this;
  }
  send(message: unknown) {
    this.sent.push(message);
  }
  emit(type: string, eventSeq: number, extra = {}) {
    this.onmessage?.({
      data: JSON.stringify({ type, eventSeq, attemptId: "attempt", ...extra }),
    });
  }
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

async function open() {
  vi.stubGlobal("WebSocket", Socket);
  vi.mocked(requestApi).mockResolvedValue({
    attemptId: "attempt",
    streamPath: "/api/kyc/realtime/attempts/attempt/stream",
    streamTicket: "one-use",
    mimeTypes: ["video/webm"],
  });
  const progress = vi.fn(),
    error = vi.fn();
  const opening = openRealtimeKyc(
    new File(["front"], "front.jpg"),
    new File(["back"], "back.jpg"),
    "kyc-token",
    progress,
    error,
    new AbortController().signal,
  );
  await Promise.resolve();
  await Promise.resolve();
  const socket = Socket.instance;
  socket.onopen?.();
  socket.emit("ready", 1);
  return { capture: await opening, socket, progress, error };
}

it("authenticates by message, orders binary chunks, and waits for the server before completion", async () => {
  const { capture, socket, progress, error } = await open();
  expect(JSON.parse(socket.sent[0] as string)).toEqual({
    type: "authenticate",
    ticket: "one-use",
  });
  const starting = capture.start("video/webm");
  socket.emit("started", 2);
  await starting;
  const chunks = [new Uint8Array([11, 22, 33]), new Uint8Array([44, 55])].map(
    (bytes) => {
      const chunk = new Blob([bytes]);
      // jsdom's Blob lacks arrayBuffer on some supported test runtimes.
      Object.defineProperty(chunk, "arrayBuffer", {
        value: async () => bytes.buffer,
      });
      return chunk;
    },
  );
  chunks.forEach((chunk) => capture.send(chunk));
  const finished = capture.finish();
  await vi.waitFor(() =>
    expect(socket.sent.filter((m) => m instanceof Uint8Array)).toHaveLength(2),
  );
  const binary = socket.sent.filter(
    (m) => m instanceof Uint8Array,
  ) as Uint8Array[];
  expect([...binary[0]]).toEqual([1, 0, 0, 0, 0, 11, 22, 33]);
  expect([...binary[1]]).toEqual([1, 0, 0, 0, 1, 44, 55]);
  socket.emit("ack", 3, { chunkSeq: 0 });
  socket.emit("ack", 4, { chunkSeq: 1 });
  socket.emit("progress", 5, {
    challengeStep: 1,
    challengePassed: false,
    faceState: "single",
    qualityState: "passed",
  });
  expect(progress).toHaveBeenCalledOnce();
  let done = false;
  void finished.then(() => {
    done = true;
  });
  await Promise.resolve();
  expect(done).toBe(false);
  socket.emit("stream_finished", 6);
  await finished;
  expect(error).not.toHaveBeenCalled();
  capture.dispose(false);
  socket.emit("progress", 6, { challengeStep: 2, challengePassed: false });
  expect(progress).toHaveBeenCalledOnce();
  expect(socket.close).toHaveBeenCalledOnce();
  expect(requestApi).toHaveBeenCalledTimes(1);
});

it("fails and cancels a congested connection instead of buffering video without a bound", async () => {
  vi.useFakeTimers();
  const { capture, socket, error } = await open();
  socket.bufferedAmount = 1024 * 1024 + 1;
  await vi.advanceTimersByTimeAsync(250);
  expect(error).toHaveBeenCalledOnce();
  expect(socket.close).toHaveBeenCalledOnce();
  expect(requestApi).toHaveBeenLastCalledWith(
    "/api/kyc/realtime/attempts/attempt",
    expect.objectContaining({ method: "DELETE", token: "kyc-token" }),
  );
  capture.dispose();
  expect(socket.close).toHaveBeenCalledOnce();
});
