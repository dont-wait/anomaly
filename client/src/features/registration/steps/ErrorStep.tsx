import type { RegistrationFlowState } from "../useRegistrationFlow";
import { StepHeading } from "../components/StepPrimitives";
import { faCircleExclamation } from "@fortawesome/free-solid-svg-icons";
import { Button } from "@/shared/ui";
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
            <Button className="w-full" onClick={retry}>
              Quay lại video
            </Button>
            <Button
              variant="secondary"
              className="mt-3 w-full"
              onClick={() => go("document")}
            >
              Chụp lại CCCD
            </Button>
          </>
        )}
        <Button variant="secondary" className="mt-3 w-full" onClick={onLogin}>
          Về trang đăng nhập
        </Button>
      </div>
    </>
  );
}
