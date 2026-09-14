import type { RegistrationFlowState } from "../useRegistrationFlow";
import { StepHeading } from "../components/StepPrimitives";
import { faCircleExclamation } from "@fortawesome/free-solid-svg-icons";
export function ErrorStep({
  error,
  remaining,
  heading,
  retry,
  go,
  onLogin,
}: Pick<
  RegistrationFlowState,
  "error" | "remaining" | "heading" | "retry" | "go"
> & { onLogin: () => void }) {
  return (
    <>
      <StepHeading
        headingRef={heading}
        title={remaining ? "Xác thực chưa hoàn tất" : "Đã hết lượt xác thực"}
        icon={faCircleExclamation}
      >
        {remaining
          ? "Kiểm tra thông tin dưới đây để tiếp tục."
          : "Vui lòng liên hệ hỗ trợ để được hướng dẫn."}
      </StepHeading>
      <div className="form-card">
        <p role="alert" className="error-text">
          {error}
        </p>
        <p className="attempts">Còn {remaining}/3 lượt xác thực</p>
        {remaining > 0 && (
          <>
            <button className="primary" onClick={retry}>
              Quay lại video
            </button>
            <button className="secondary" onClick={() => go("document")}>
              Chụp lại CCCD
            </button>
          </>
        )}
        <button className="secondary" onClick={onLogin}>
          Về trang đăng nhập
        </button>
      </div>
    </>
  );
}
