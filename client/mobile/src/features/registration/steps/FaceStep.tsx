import type { RegistrationFlowState } from "../useRegistrationFlow";
import { FaceCapture } from "../components/FaceCapture";
import { Button } from "@/shared/ui";

export function FaceStep({
  remaining,
  attemptLimit,
  heading,
  verifyVideo,
  prepareCapture,
  go,
  hasPendingCapture,
  retryPendingCapture,
  discardPendingCapture,
}: Pick<
  RegistrationFlowState,
  | "remaining"
  | "attemptLimit"
  | "heading"
  | "verifyVideo"
  | "prepareCapture"
  | "go"
  | "hasPendingCapture"
  | "retryPendingCapture"
  | "discardPendingCapture"
>) {
  return (
    <>
      <h1 className="capture-heading" ref={heading} tabIndex={-1}>
        Xác thực khuôn mặt
      </h1>
      {hasPendingCapture ? (
        <div className="form-card">
          <p>
            Video đã quay vẫn được giữ trong phiên này. Bạn có thể thử gửi lại.
          </p>
          <Button className="w-full" onClick={retryPendingCapture}>
            Thử gửi lại video
          </Button>
          <Button
            variant="secondary"
            className="mt-3 w-full"
            onClick={discardPendingCapture}
          >
            Hủy video và quay lại
          </Button>
        </div>
      ) : (
        <FaceCapture onComplete={verifyVideo} prepareCapture={prepareCapture} />
      )}
      <div className="capture-options">
        <span className="attempts">
          Còn {remaining}/{attemptLimit} lượt xác thực
        </span>
        {!hasPendingCapture && (
          <button
            type="button"
            className="help-button"
            onClick={() => go("document")}
          >
            Chụp lại CCCD
          </button>
        )}
      </div>
    </>
  );
}
