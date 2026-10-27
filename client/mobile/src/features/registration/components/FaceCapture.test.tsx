import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { FaceCapture } from "./FaceCapture";
import type { CaptureProgress, RealtimeCapture } from "../api/realtime";

class Recorder {
  static isTypeSupported = () => true;
  state = "inactive";
  ondataavailable: ((event: { data: Blob }) => void) | null = null;
  onstop: (() => void) | null = null;
  start() {
    this.state = "recording";
  }
  stop() {
    this.state = "inactive";
    this.ondataavailable?.({
      data: new Blob(["live recording"], { type: "video/webm" }),
    });
    this.onstop?.();
  }
}
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function setup() {
  vi.useFakeTimers();
  const stop = vi.fn();
  const track = { stop, onended: null };
  vi.stubGlobal("navigator", {
    mediaDevices: {
      getUserMedia: vi.fn().mockResolvedValue({
        getTracks: () => [track],
        getVideoTracks: () => [track],
      }),
    },
  });
  vi.stubGlobal("MediaRecorder", Recorder);
  let progress!: (value: CaptureProgress) => void;
  const session: RealtimeCapture = {
    attemptId: "attempt",
    idempotencyKey: "key",
    mimeTypes: ["video/webm"],
    start: vi.fn().mockResolvedValue(undefined),
    send: vi.fn(),
    finish: vi.fn().mockResolvedValue(undefined),
    dispose: vi.fn(),
  };
  const prepare = vi.fn(async (callback) => {
    progress = callback;
    return session;
  });
  const onComplete = vi.fn();
  const view = render(
    <FaceCapture onComplete={onComplete} prepareCapture={prepare} />,
  );
  return {
    stop,
    session,
    onComplete,
    view,
    progress: (step: number) =>
      progress({
        challengeStep: step,
        challengePassed: step === 3,
        faceState: "single",
        qualityState: "passed",
        matchingState: "collecting",
      }),
  };
}

it("uses server evidence instead of the timer and hands off the exact recording with its attempt", async () => {
  const test = setup();
  await act(async () =>
    fireEvent.click(screen.getByRole("button", { name: "Mở camera" })),
  );
  await act(async () =>
    fireEvent.click(
      screen.getByRole("button", { name: "Bắt đầu quay và xác thực" }),
    ),
  );
  act(() => vi.advanceTimersByTime(3000));
  expect(screen.getByText("Nhìn thẳng vào camera")).toBeTruthy();
  act(() => test.progress(1));
  expect(screen.getByText("Từ từ quay đầu sang trái")).toBeTruthy();
  act(() => test.progress(2));
  expect(screen.getByText("Từ từ quay đầu sang phải")).toBeTruthy();
  expect(test.onComplete).not.toHaveBeenCalled();
  await act(async () => vi.advanceTimersByTime(9000));
  expect(test.session.send).toHaveBeenCalledOnce();
  expect(test.session.finish).toHaveBeenCalledOnce();
  expect(test.onComplete).toHaveBeenCalledWith(expect.any(File), {
    attemptId: "attempt",
    idempotencyKey: "key",
  });
  const completed = test.onComplete.mock.calls[0][0] as File;
  expect(completed.type).toBe("video/webm");
  expect(await completed.text()).toBe("live recording");
  expect(test.stop).toHaveBeenCalledOnce();
  expect(test.session.dispose).toHaveBeenCalledWith(false);
});

it("does not submit an offline recording if no realtime feedback arrived while recording", async () => {
  const test = setup();
  await act(async () =>
    fireEvent.click(screen.getByRole("button", { name: "Mở camera" })),
  );
  await act(async () =>
    fireEvent.click(
      screen.getByRole("button", { name: "Bắt đầu quay và xác thực" }),
    ),
  );
  await act(async () => vi.advanceTimersByTime(12000));
  expect(test.onComplete).not.toHaveBeenCalled();
  expect(screen.getByRole("alert").textContent).toContain(
    "Chưa nhận được phản hồi trực tiếp",
  );
  expect(test.session.dispose).toHaveBeenCalled();
});

it("stops a camera permission request that resolves after leaving the page", async () => {
  let resolve!: (stream: unknown) => void;
  const stop = vi.fn();
  vi.stubGlobal("navigator", {
    mediaDevices: {
      getUserMedia: () =>
        new Promise((r) => {
          resolve = r;
        }),
    },
  });
  vi.stubGlobal("MediaRecorder", Recorder);
  const onComplete = vi.fn();
  const { unmount } = render(<FaceCapture onComplete={onComplete} />);
  fireEvent.click(screen.getByRole("button", { name: "Mở camera" }));
  unmount();
  await act(async () => resolve({ getTracks: () => [{ stop }] }));
  expect(stop).toHaveBeenCalledOnce();
  expect(onComplete).not.toHaveBeenCalled();
});
