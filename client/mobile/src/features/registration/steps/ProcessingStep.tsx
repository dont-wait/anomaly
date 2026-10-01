import type { RegistrationFlowState } from "../useRegistrationFlow";
import { StepHeading } from "../components/StepPrimitives";
import {
  faFingerprint,
  faIdCard,
  faShieldHalved,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

export function ProcessingStep({
  heading,
  progress,
}: Pick<RegistrationFlowState, "heading" | "progress">) {
  return (
    <>
      <StepHeading
        headingRef={heading}
        title="Đang xác minh thông tin của bạn"
        icon={faFingerprint}
      >
        Vui lòng đợi trong giây lát.
      </StepHeading>
      <div className="form-card">
        <div className="spinner" />
        <p className="centered">{progress}</p>
        <ul className="processing-list">
          <li>
            <FontAwesomeIcon icon={faIdCard} /> Kiểm tra ảnh CCCD
          </li>
          <li>
            <FontAwesomeIcon icon={faFingerprint} /> Đối chiếu khuôn mặt
          </li>
          <li>
            <FontAwesomeIcon icon={faShieldHalved} /> Kiểm tra thông tin định
            danh
          </li>
        </ul>
      </div>
    </>
  );
}
