import type { RegistrationFlowState } from "../useRegistrationFlow";
import { Notice, StepHeading } from "../components/StepPrimitives";
import { faFingerprint } from "@fortawesome/free-solid-svg-icons";
import { FaceCapture } from "../components/FaceCapture";

export function FaceStep({
  remaining,
  heading,
  verifyVideo,
}: Pick<RegistrationFlowState, "remaining" | "heading" | "verifyVideo">) {
  return (
    <>
      <StepHeading
        headingRef={heading}
        title="Xác thực khuôn mặt"
        icon={faFingerprint}
      >
        Đưa khuôn mặt vào khung hình và làm theo hướng dẫn.
      </StepHeading>
      <FaceCapture onComplete={verifyVideo} />
      <p className="attempts">Còn {remaining}/3 lượt xác thực</p>
      <Notice>
        Đảm bảo đủ ánh sáng, tháo khẩu trang và kính râm. Xác thực yêu cầu video
        trực tiếp.
      </Notice>
    </>
  );
}
