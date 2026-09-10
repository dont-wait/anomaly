import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { FaceCapture } from "./FaceCapture";
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
it("records all four timed instructions then supplies a real File and closes the camera", async () => {
  vi.useFakeTimers();
  const stop = vi.fn();
  const track = { stop, onended: null };
  vi.stubGlobal("navigator", {
    mediaDevices: {
      getUserMedia: vi
        .fn()
        .mockResolvedValue({
          getTracks: () => [track],
          getVideoTracks: () => [track],
        }),
    },
  });
  vi.stubGlobal("MediaRecorder", Recorder);
  const onComplete = vi.fn();
  const { unmount } = render(<FaceCapture onComplete={onComplete} />);
  await act(async () =>
    fireEvent.click(screen.getByRole("button", { name: "Mở camera" })),
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Bắt đầu quay và xác thực" }),
  );
  act(() => vi.advanceTimersByTime(3000));
  expect(screen.getByText("Từ từ quay đầu sang trái")).toBeTruthy();
  act(() => vi.advanceTimersByTime(3000));
  expect(screen.getByText("Từ từ quay đầu sang phải")).toBeTruthy();
  act(() => vi.advanceTimersByTime(3000));
  expect(screen.getByText("Chớp mắt và giữ yên")).toBeTruthy();
  expect(onComplete).not.toHaveBeenCalled();
  act(() => vi.advanceTimersByTime(3000));
  expect(onComplete).toHaveBeenCalledOnce();
  const file = onComplete.mock.calls[0][0];
  expect(file).toBeInstanceOf(File);
  expect(file.type).toBe("video/webm");
  expect(file.size).toBeGreaterThan(0);
  unmount();
  expect(stop).toHaveBeenCalledOnce();
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
  await act(async () => {
    resolve({ getTracks: () => [{ stop }] });
  });
  expect(stop).toHaveBeenCalledOnce();
  expect(onComplete).not.toHaveBeenCalled();
});
