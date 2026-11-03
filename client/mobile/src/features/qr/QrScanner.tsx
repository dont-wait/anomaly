import { useEffect, useRef, useState, type ChangeEvent } from "react";
import jsQR from "jsqr";
import { parseAccountQr } from "./payload";

const SCAN_INTERVAL_MS = 200;
const MAX_FRAME_SIZE = 720;
const MAX_IMAGE_SIZE = 1600;
const UNSUPPORTED_QR = "Mã này không phải QR nhận tiền của AnomalyBank.";

/** Đọc một khung hình; thu nhỏ trước khi giải mã để không làm giật máy yếu. */
function decode(
  source: CanvasImageSource,
  width: number,
  height: number,
  canvas: HTMLCanvasElement,
  maxSize: number,
) {
  const scale = Math.min(1, maxSize / Math.max(width, height));
  canvas.width = Math.round(width * scale);
  canvas.height = Math.round(height * scale);
  const context = canvas.getContext("2d", { willReadFrequently: true });
  if (!context) return null;
  context.drawImage(source, 0, 0, canvas.width, canvas.height);
  const image = context.getImageData(0, 0, canvas.width, canvas.height);
  return jsQR(image.data, image.width, image.height)?.data ?? null;
}

function cameraError(error: unknown) {
  return error instanceof DOMException && error.name === "NotAllowedError"
    ? "Chưa có quyền dùng camera. Hãy cấp quyền hoặc chọn ảnh có mã QR."
    : "Không mở được camera. Bạn có thể chọn ảnh có mã QR.";
}

export function QrScanner({
  onAccountNo,
}: {
  onAccountNo: (accountNo: string) => void;
}) {
  const video = useRef<HTMLVideoElement>(null);
  const canvas = useRef<HTMLCanvasElement | null>(null);
  const onFound = useRef(onAccountNo);
  const [message, setMessage] = useState("");
  const [cameraReady, setCameraReady] = useState(false);

  useEffect(() => {
    onFound.current = onAccountNo;
  }, [onAccountNo]);

  useEffect(() => {
    let stopped = false;
    let stream: MediaStream | undefined;
    let timer: ReturnType<typeof setInterval> | undefined;
    canvas.current ??= document.createElement("canvas");

    const scanFrame = () => {
      const node = video.current;
      if (!node || !canvas.current || node.readyState < 2) return;
      const text = decode(
        node,
        node.videoWidth,
        node.videoHeight,
        canvas.current,
        MAX_FRAME_SIZE,
      );
      if (!text) return;
      const accountNo = parseAccountQr(text);
      if (!accountNo) {
        setMessage(UNSUPPORTED_QR);
        return;
      }
      clearInterval(timer);
      onFound.current(accountNo);
    };

    const start = async () => {
      try {
        if (!navigator.mediaDevices?.getUserMedia) {
          throw new Error("camera unavailable");
        }
        const media = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: { ideal: "environment" } },
          audio: false,
        });
        if (stopped) {
          media.getTracks().forEach((track) => track.stop());
          return;
        }
        stream = media;
        if (video.current) {
          video.current.srcObject = media;
          await video.current.play();
        }
        if (stopped) return;
        setCameraReady(true);
        timer = setInterval(scanFrame, SCAN_INTERVAL_MS);
      } catch (error) {
        if (!stopped) setMessage(cameraError(error));
      }
    };
    void start();

    return () => {
      stopped = true;
      clearInterval(timer);
      stream?.getTracks().forEach((track) => track.stop());
    };
  }, []);

  const scanImage = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    // Cho phép chọn lại đúng ảnh đó sau khi báo lỗi.
    event.target.value = "";
    if (!file) return;
    canvas.current ??= document.createElement("canvas");
    try {
      const bitmap = await createImageBitmap(file);
      const text = decode(
        bitmap,
        bitmap.width,
        bitmap.height,
        canvas.current,
        MAX_IMAGE_SIZE,
      );
      bitmap.close();
      if (!text) {
        setMessage("Không tìm thấy mã QR trong ảnh này.");
        return;
      }
      const accountNo = parseAccountQr(text);
      if (accountNo) onFound.current(accountNo);
      else setMessage(UNSUPPORTED_QR);
    } catch {
      setMessage("Không đọc được ảnh này. Vui lòng chọn ảnh khác.");
    }
  };

  return (
    <section className="flex flex-col gap-4">
      <div className="relative aspect-square w-full overflow-hidden rounded-3xl bg-black">
        <video
          ref={video}
          muted
          playsInline
          aria-label="Khung quét mã QR"
          className="h-full w-full object-cover"
        />
        {cameraReady && (
          <div
            aria-hidden="true"
            className="absolute inset-[14%] rounded-3xl border-4 border-white/85"
          />
        )}
      </div>
      <p role="status" className="min-h-5 text-center text-sm text-on-surface">
        {message || (cameraReady ? "Đưa mã QR vào trong khung." : "")}
      </p>
      <label className="relative flex min-h-12 cursor-pointer items-center justify-center rounded-full bg-surface-container px-4 text-base font-bold text-primary">
        Chọn ảnh có mã QR
        <input
          className="sr-only"
          type="file"
          accept="image/*"
          onChange={(event) => void scanImage(event)}
        />
      </label>
    </section>
  );
}
