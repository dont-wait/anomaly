import { useEffect, useRef, useState } from "react";
import { Button } from "@/shared/ui";
import type {
  CaptureIdentity,
  CaptureProgress,
  RealtimeCapture,
} from "../api/realtime";

const instructions = [
  "Nhìn thẳng vào camera",
  "Từ từ quay đầu sang trái",
  "Từ từ quay đầu sang phải",
  "Chớp mắt và giữ yên",
];
type PrepareCapture = (
  onProgress: (p: CaptureProgress) => void,
  onError: (e: Error) => void,
  signal: AbortSignal,
) => Promise<RealtimeCapture>;

export function FaceCapture({
  onComplete,
  prepareCapture,
}: {
  onComplete: (file: File, capture?: CaptureIdentity) => void;
  prepareCapture?: PrepareCapture;
}) {
  const video = useRef<HTMLVideoElement>(null);
  const preview = useRef<HTMLDivElement>(null);
  const stream = useRef<MediaStream | null>(null);
  const recorder = useRef<MediaRecorder | null>(null);
  const session = useRef<RealtimeCapture | null>(null);
  const controller = useRef<AbortController | null>(null);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);
  const mounted = useRef(true);
  const opening = useRef(false);
  const failed = useRef(false);
  const delivered = useRef(false);
  const recordingNow = useRef(false);
  const liveProgressSeen = useRef(false);
  const [active, setActive] = useState(false);
  const [preparing, setPreparing] = useState(false);
  const [recording, setRecording] = useState(false);
  const [finishing, setFinishing] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const [progress, setProgress] = useState<CaptureProgress | null>(null);
  const [error, setError] = useState("");

  function fitGuide() {
    const element = preview.current,
      camera = video.current;
    if (!element || !camera?.videoWidth || !camera.videoHeight) return;
    const scale = Math.min(
      element.clientWidth / camera.videoWidth,
      element.clientHeight / camera.videoHeight,
    );
    const width = Math.min(
      camera.videoWidth * scale * 0.52,
      camera.videoHeight * scale * 0.5,
    );
    element.style.setProperty("--guide-width", `${width}px`);
  }

  function stopCamera() {
    recordingNow.current = false;
    if (timer.current) clearInterval(timer.current);
    stream.current?.getTracks().forEach((track) => {
      track.onended = null;
      track.stop();
    });
    stream.current = null;
  }
  function fail(reason: Error) {
    if (failed.current || !mounted.current) return;
    failed.current = true;
    const capture = recorder.current;
    if (capture && capture.state !== "inactive") {
      capture.onstop = null;
      capture.stop();
    }
    stopCamera();
    session.current?.dispose();
    controller.current?.abort();
    setRecording(false);
    setActive(false);
    setPreparing(false);
    setFinishing(false);
    setProgress(null);
    setError(reason.message);
  }
  useEffect(() => {
    mounted.current = true;
    const observer =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(fitGuide)
        : null;
    if (preview.current) observer?.observe(preview.current);
    window.addEventListener("resize", fitGuide);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", fitGuide);
      mounted.current = false;
      session.current?.dispose(!delivered.current);
      controller.current?.abort();
      if (recorder.current) {
        recorder.current.onstop = null;
        recorder.current.ondataavailable = null;
        recorder.current.onerror = null;
        if (recorder.current.state !== "inactive") recorder.current.stop();
      }
      stopCamera();
    };
  }, []);

  async function openCamera() {
    if (opening.current) return;
    opening.current = true;
    failed.current = false;
    delivered.current = false;
    liveProgressSeen.current = false;
    setPreparing(true);
    setError("");
    setElapsed(0);
    setProgress(null);
    const abort = new AbortController();
    controller.current = abort;
    try {
      if (
        !navigator.mediaDevices?.getUserMedia ||
        typeof MediaRecorder === "undefined"
      )
        throw new Error(
          "Thiết bị chưa hỗ trợ quay video. Vui lòng kiểm tra quyền camera.",
        );
      const media = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: "user",
          width: { ideal: 480 },
          height: { ideal: 640 },
          frameRate: { ideal: 30, max: 30 },
        },
        audio: false,
      });
      if (!mounted.current || abort.signal.aborted) {
        media.getTracks().forEach((t) => t.stop());
        return;
      }
      stream.current = media;
      if (video.current) video.current.srcObject = media;
      media.getVideoTracks()[0].onended = () =>
        fail(new Error("Camera đã ngắt kết nối. Vui lòng mở lại camera."));
      setActive(true);
      if (!prepareCapture)
        throw new Error("Dịch vụ xác thực trực tiếp chưa được cấu hình.");
      const realtime = await prepareCapture(
        (value) => {
          if (
            !mounted.current ||
            failed.current ||
            abort.signal.aborted ||
            controller.current !== abort
          )
            return;
          if (recordingNow.current) liveProgressSeen.current = true;
          setProgress(value);
        },
        fail,
        abort.signal,
      );
      if (!mounted.current || abort.signal.aborted || failed.current) {
        realtime.dispose();
        return;
      }
      session.current = realtime;
    } catch (cause) {
      fail(
        cause instanceof Error
          ? cause
          : new Error("Không thể mở camera xác thực."),
      );
    } finally {
      opening.current = false;
      if (mounted.current) setPreparing(false);
    }
  }

  async function record() {
    const media = stream.current,
      realtime = session.current;
    if (!media || !realtime || preparing || recording || finishing) return;
    const mimeType = realtime.mimeTypes.find((type) =>
      MediaRecorder.isTypeSupported(type),
    );
    if (!mimeType) {
      fail(
        new Error(
          "Thiết bị chưa hỗ trợ định dạng video realtime. Vui lòng dùng thiết bị khác.",
        ),
      );
      return;
    }
    setPreparing(true);
    try {
      await realtime.start(mimeType);
      if (!mounted.current || failed.current) return;
      const capture = new MediaRecorder(media, {
        mimeType,
        videoBitsPerSecond: 1200000,
      });
      recorder.current = capture;
      const chunks: Blob[] = [];
      let size = 0;
      capture.ondataavailable = (event) => {
        if (!event.data.size || failed.current) return;
        size += event.data.size;
        if (size > 25 * 1024 * 1024) {
          fail(new Error("Video vượt quá 25 MB. Vui lòng quay lại."));
          return;
        }
        chunks.push(event.data);
        realtime.send(event.data);
      };
      capture.onerror = () =>
        fail(new Error("Ghi hình thất bại. Vui lòng quay lại."));
      capture.onstop = async () => {
        stopCamera();
        if (!mounted.current || failed.current) return;
        setRecording(false);
        setActive(false);
        setFinishing(true);
        try {
          if (!size || !liveProgressSeen.current)
            throw new Error(
              "Chưa nhận được phản hồi trực tiếp khi quay. Vui lòng thử lại.",
            );
          await realtime.finish();
          if (!mounted.current || failed.current) return;
          const type = mimeType.split(";")[0];
          const file = new File(chunks, `live-${Date.now()}.webm`, { type });
          delivered.current = true;
          realtime.dispose(false);
          setFinishing(false);
          onComplete(file, {
            attemptId: realtime.attemptId,
            idempotencyKey: realtime.idempotencyKey,
          });
        } catch (cause) {
          fail(
            cause instanceof Error ? cause : new Error("Xử lý video thất bại."),
          );
        }
      };
      capture.start(250);
      recordingNow.current = true;
      setRecording(true);
      setElapsed(0);
      setError("");
      const started = performance.now();
      timer.current = setInterval(() => {
        const seconds = Math.floor((performance.now() - started) / 1000);
        setElapsed(seconds);
        if (document.hidden) {
          fail(new Error("Ứng dụng đã xuống nền. Vui lòng quay lại video."));
          return;
        }
        if (seconds >= 12 && capture.state !== "inactive") {
          recordingNow.current = false;
          if (timer.current) clearInterval(timer.current);
          capture.stop();
        }
      }, 250);
    } catch (cause) {
      fail(cause instanceof Error ? cause : new Error("Không thể ghi hình."));
    } finally {
      if (mounted.current) setPreparing(false);
    }
  }
  const step = progress?.challengeStep ?? 0;
  const feedback =
    progress?.faceState === "multiple"
      ? "Chỉ một người trong khung hình"
      : progress?.faceState === "not_found"
        ? "Đưa khuôn mặt vào khung hình"
        : progress?.qualityState === "low"
          ? "Giữ mặt rõ và đủ sáng"
          : progress?.matchingState === "candidate_match"
            ? "Đã thu đủ bằng chứng, tiếp tục giữ camera"
            : progress?.challengePassed
              ? "Giữ yên để hoàn tất đối chiếu"
              : instructions[step];
  return (
    <div className="face-capture">
      <div className="camera-frame" ref={preview}>
        <video
          ref={video}
          autoPlay
          muted
          playsInline
          aria-label="Camera xác thực khuôn mặt"
          onLoadedMetadata={fitGuide}
        />
        <div className="face-oval" aria-hidden="true" />
        <span className="camera-tag">
          {recording
            ? `ĐANG QUAY · ${elapsed}/12 GIÂY`
            : preparing
              ? "ĐANG KẾT NỐI"
              : finishing
                ? "ĐANG ĐỐI CHIẾU"
                : "CAMERA TRỰC TIẾP"}
        </span>
        <span className="capture-stage">
          {progress?.challengePassed ? "Đủ động tác" : `Động tác ${step + 1}/4`}
        </span>
        <p className="camera-instruction" aria-live="polite">
          {finishing ? "Đang hoàn tất video…" : feedback}
        </p>
      </div>
      <div className="capture-controls">
        {error && (
          <p role="alert" className="error-text">
            {error}
          </p>
        )}
        <Button
          className="w-full"
          disabled={preparing || recording || finishing}
          onClick={active ? record : openCamera}
        >
          {preparing
            ? "Đang chuẩn bị…"
            : recording
              ? "Làm theo hướng dẫn…"
              : finishing
                ? "Đang hoàn tất…"
                : active
                  ? "Bắt đầu quay và xác thực"
                  : "Mở camera"}
        </Button>
      </div>
    </div>
  );
}
