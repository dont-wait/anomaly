import type { RegistrationFlowState } from "../useRegistrationFlow";
import { StepHeading } from "../components/StepPrimitives";
import {
  faArrowRight,
  faClock,
  faShieldHalved,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { Button } from "@/shared/ui";

export function PendingStep({
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
        title="Hồ sơ đang chờ xác minh"
        icon={faClock}
      >
        AnomalyBank đã nhận đủ giấy tờ và video của bạn.
      </StepHeading>
      <div className="account-card">
        <span className="account-badge">
          <FontAwesomeIcon icon={faClock} /> Đang chờ xác minh
        </span>
        <p>Chủ tài khoản</p>
        <h2>{profile.name}</h2>
        <p>{email}</p>
        <div className="account-divider" />
        <span>
          Bộ phận xác minh sẽ đối chiếu ảnh CCCD, video và thông tin bạn đã
          khai. Bạn sẽ đăng nhập được ngay khi hồ sơ được duyệt.
        </span>
        <span>
          <FontAwesomeIcon icon={faShieldHalved} /> Kết quả dự kiến trong vòng
          24 giờ làm việc.
        </span>
      </div>
      <Button
        className="w-full"
        onClick={() => {
          onLogin();
        }}
      >
        Về trang đăng nhập
        <FontAwesomeIcon icon={faArrowRight} />
      </Button>
    </>
  );
}
