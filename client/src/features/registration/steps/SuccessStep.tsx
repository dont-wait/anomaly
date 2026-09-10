import type { RegistrationFlowState } from "../useRegistrationFlow";
import { StepHeading } from "../components/StepPrimitives";
import { faArrowRight, faCheck } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

export function SuccessStep({
  email,
  profile,
  heading,
  onLogin,
}: Pick<RegistrationFlowState, "email" | "profile" | "heading"> & {
  onLogin: () => void;
}) {
  return (
    <>
      <StepHeading
        headingRef={heading}
        title="Bạn đã hoàn tất đăng ký!"
        icon={faCheck}
      >
        Cảm ơn bạn đã trải nghiệm AnomalyBank.
      </StepHeading>
      <div className="account-card">
        <span className="account-badge">✓ Tài khoản đã được xác thực</span>
        <p>Chủ tài khoản</p>
        <h2>{profile.name}</h2>
        <p>{email}</p>
        <div className="account-divider" />
        <span>Bạn đã có thể đăng nhập bằng CCCD và mật khẩu vừa tạo.</span>
      </div>
      <button
        className="primary"
        onClick={() => {
          onLogin();
        }}
      >
        Về trang đăng nhập
        <FontAwesomeIcon icon={faArrowRight} />
      </button>
    </>
  );
}
