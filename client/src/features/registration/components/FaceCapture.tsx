import { useEffect, useRef, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faFingerprint, faCamera } from "@fortawesome/free-solid-svg-icons";
const instructions = [
  "Nhìn thẳng vào camera",
  "Từ từ quay đầu sang trái",
  "Từ từ quay đầu sang phải",
  "Chớp mắt và giữ yên",
];
const MAX_VIDEO_BYTES = 25 * 1024 * 1024;

export function FaceCapture({
  onComplete,
}: {
  onComplete: (file: File) => void;
}) {
  const video = useRef<HTMLVideoElement>(null);
  const stream = useRef<MediaStream | null>(null);
  const recorder = useRef<MediaRecorder | null>(null);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);
  const mounted = useRef(true);
  const opening = useRef(false);
  const [active, setActive] = useState(false);
  const [isOpening, setIsOpening] = useState(false);
  const [recording, setRecording] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const [error, setError] = useState("");
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      if (timer.current) clearInterval(timer.current);
      if (recorder.current) {
        recorder.current.onstop = null;
        recorder.current.ondataavailable = null;
        if (recorder.current.state !== "inactive") recorder.current.stop();
      }
      stream.current?.getTracks().forEach((track) => track.stop());
    };
  }, []);
  async function openCamera() {
    if (opening.current) return;
    opening.current = true;
    setIsOpening(true);
    setError("");
    try {
      if (
        !navigator.mediaDevices?.getUserMedia ||
        typeof MediaRecorder === "undefined"
      )
        throw new Error("unsupported");
      const media = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode: "user",
          width: { ideal: 640 },
          height: { ideal: 480 },
        },
        audio: false,
      });
      if (!mounted.current) {
        media.getTracks().forEach((track) => track.stop());
        return;
      }
      stream.current = media;
      if (video.current) video.current.srcObject = media;
      setActive(true);
      media.getVideoTracks()[0].onended = () => {
        if (!mounted.current) return;
        if (timer.current) clearInterval(timer.current);
        if (recorder.current && recorder.current.state !== "inactive") {
          recorder.current.onstop = null;
          recorder.current.stop();
        }
        setRecording(false);
        setActive(false);
        setError("Camera đã ngắt kết nối. Vui lòng mở lại camera.");
      };
    } catch {
      if (mounted.current)
        setError(
          "Không thể mở hoặc ghi hình camera. Kiểm tra quyền camera và sử dụng WebView/trình duyệt hỗ trợ ghi video.",
        );
    } finally {
      opening.current = false;
      if (mounted.current) setIsOpening(false);
    }
  }
  function record() {
    if (!stream.current || recorder.current?.state === "recording") return;
    const mimeType = ["video/webm;codecs=vp8", "video/webm", "video/mp4"].find(
      (type) => MediaRecorder.isTypeSupported(type),
    );
    if (!mimeType) {
      setError("Thiết bị chưa hỗ trợ định dạng ghi video phù hợp.");
      return;
    }
    try {
      const capture = new MediaRecorder(stream.current, {
        mimeType,
        videoBitsPerSecond: 1200000,
      });
      recorder.current = capture;
      const chunks: Blob[] = [];
      let size = 0;
      let failed = false;
      capture.ondataavailable = (event) => {
        if (event.data.size) {
          chunks.push(event.data);
          size += event.data.size;
        }
        if (size > MAX_VIDEO_BYTES && capture.state !== "inactive") {
          failed = true;
          capture.stop();
        }
      };
      capture.onerror = () => {
        failed = true;
        if (timer.current) clearInterval(timer.current);
        if (capture.state !== "inactive") capture.stop();
        if (mounted.current) {
          setRecording(false);
          setError("Ghi hình thất bại. Vui lòng quay lại.");
        }
      };
      capture.onstop = () => {
        if (timer.current) clearInterval(timer.current);
        if (!mounted.current) return;
        setRecording(false);
        if (failed || !size || size > MAX_VIDEO_BYTES) {
          setError(
            "Không thể dùng video này. Vui lòng quay lại video dưới 25 MB.",
          );
          return;
        }
        const type = mimeType.split(";")[0];
        onComplete(
          new File(
            chunks,
            `live-${Date.now()}.${type === "video/mp4" ? "mp4" : "webm"}`,
            { type },
          ),
        );
      };
      capture.start(1000);
      setElapsed(0);
      setRecording(true);
      setError("");
      const started = Date.now();
      timer.current = setInterval(() => {
        const seconds = Math.floor((Date.now() - started) / 1000);
        setElapsed(seconds);
        if (seconds >= 12) {
          if (timer.current) clearInterval(timer.current);
          if (capture.state !== "inactive") capture.stop();
        }
      }, 250);
    } catch {
      setError("Không thể ghi hình. Vui lòng kiểm tra camera và thử lại.");
    }
  }
  const challenge = Math.min(3, Math.floor(elapsed / 3));
  return (
    <div className="form-card">
      <div className="camera-frame">
        <video
          ref={video}
          autoPlay
          muted
          playsInline
          aria-label="Camera xác thực khuôn mặt"
        />
        <div className="face-oval">
          {!active && <FontAwesomeIcon icon={faFingerprint} />}
        </div>
        <span className="camera-tag">
          {recording
            ? `ĐANG GHI HÌNH · ${Math.min(elapsed, 12)}/12 GIÂY`
            : "CAMERA TRỰC TIẾP"}
        </span>
      </div>
      <p className="challenge" aria-live="polite">
        {instructions[challenge]}
      </p>
      <div className="challenge-dots">
        {instructions.map((text, i) => (
          <span key={text} className={challenge >= i ? "done" : ""} />
        ))}
      </div>
      {error && (
        <p role="alert" className="error-text">
          {error}
        </p>
      )}
      {!active ? (
        <button className="primary" disabled={isOpening} onClick={openCamera}>
          <FontAwesomeIcon icon={faCamera} />
          {isOpening ? "Đang mở camera…" : "Mở camera"}
        </button>
      ) : (
        <button className="primary" disabled={recording} onClick={record}>
          {recording ? "Làm theo hướng dẫn…" : "Bắt đầu quay và xác thực"}
        </button>
      )}
      <p className="field-hint centered" style={{ marginTop: 16 }}>
        Video 12 giây sẽ được gửi để xác thực khi quay xong.
      </p>
    </div>
  );
}
