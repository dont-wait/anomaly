import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import jsQR from "jsqr";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { buildAccountQr } from "./payload";
import { QrScanner } from "./QrScanner";

vi.mock("jsqr", () => ({ default: vi.fn() }));

const stop = vi.fn();
const getUserMedia = vi.fn();

function selectImage() {
  const input = screen.getByLabelText<HTMLInputElement>("Chọn ảnh có mã QR");
  fireEvent.change(input, {
    target: { files: [new File(["x"], "qr.png", { type: "image/png" })] },
  });
}

beforeEach(() => {
  vi.mocked(jsQR).mockReset();
  stop.mockReset();
  getUserMedia.mockReset();
  getUserMedia.mockRejectedValue(new DOMException("no", "NotAllowedError"));
  vi.stubGlobal("navigator", { mediaDevices: { getUserMedia } });
  vi.stubGlobal(
    "createImageBitmap",
    vi.fn(async () => ({ width: 10, height: 10, close: vi.fn() })),
  );
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
    drawImage: vi.fn(),
    getImageData: () => ({
      data: new Uint8ClampedArray(400),
      width: 10,
      height: 10,
    }),
  } as unknown as RenderingContext);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it("offers the image option when camera permission is denied", async () => {
  render(<QrScanner onAccountNo={vi.fn()} />);
  expect(await screen.findByText(/Chưa có quyền dùng camera/)).toBeTruthy();
  expect(screen.getByLabelText("Chọn ảnh có mã QR")).toBeTruthy();
});

it("returns the account number from a valid image", async () => {
  vi.mocked(jsQR).mockReturnValue({
    data: buildAccountQr("1234567890"),
  } as ReturnType<typeof jsQR>);
  const onAccountNo = vi.fn();
  render(<QrScanner onAccountNo={onAccountNo} />);
  selectImage();
  await waitFor(() => expect(onAccountNo).toHaveBeenCalledWith("1234567890"));
});

it("rejects a QR from elsewhere without leaving the screen", async () => {
  vi.mocked(jsQR).mockReturnValue({
    data: "https://evil.example",
  } as ReturnType<typeof jsQR>);
  const onAccountNo = vi.fn();
  render(<QrScanner onAccountNo={onAccountNo} />);
  selectImage();
  expect(await screen.findByText(/không phải QR nhận tiền/)).toBeTruthy();
  expect(onAccountNo).not.toHaveBeenCalled();
});

it("reports an image without a QR", async () => {
  vi.mocked(jsQR).mockReturnValue(null);
  render(<QrScanner onAccountNo={vi.fn()} />);
  selectImage();
  expect(await screen.findByText(/Không tìm thấy mã QR/)).toBeTruthy();
});

it("releases the camera when leaving the screen", async () => {
  getUserMedia.mockResolvedValue({ getTracks: () => [{ stop }] });
  vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
  const view = render(<QrScanner onAccountNo={vi.fn()} />);
  await screen.findByText("Đưa mã QR vào trong khung.");
  view.unmount();
  expect(stop).toHaveBeenCalledOnce();
});
