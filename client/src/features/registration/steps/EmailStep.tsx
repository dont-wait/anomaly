import type { RegistrationFlowState } from "../useRegistrationFlow";
import { Notice, Next, StepHeading } from "../components/StepPrimitives";
import { faEnvelope } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

export function EmailStep({
  email,
  setEmail,
  consent,
  setConsent,
  setDialog,
  heading,
  submit,
  onLogin,
}: Pick<
  RegistrationFlowState,
  | "email"
  | "setEmail"
  | "consent"
  | "setConsent"
  | "setDialog"
  | "heading"
  | "submit"
> & { onLogin: () => void }) {
  return (
    <>
      <StepHeading
        headingRef={heading}
        title="Mở tài khoản AnomalyBank"
        icon={faEnvelope}
      >
        Chỉ mất vài phút để bắt đầu
      </StepHeading>
      <form className="form-card" onSubmit={submit}>
        <label className="field">
          Địa chỉ email <span className="required">*</span>
          <span className="input-wrap">
            <FontAwesomeIcon icon={faEnvelope} />
            <input
              type="email"
              autoComplete="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="ban@example.com"
            />
          </span>
        </label>
        <p className="field-hint">
          Sử dụng email bạn đang dùng để đăng ký tài khoản.
        </p>
        <Notice>Email được sử dụng làm thông tin liên hệ của tài khoản.</Notice>
        <label className="consent">
          <input
            type="checkbox"
            checked={consent}
            onChange={(e) => setConsent(e.target.checked)}
          />
          <span>
            Tôi đồng ý với{" "}
            <button
              type="button"
              className="text-link"
              onClick={() => setDialog("Điều khoản sử dụng")}
            >
              Điều khoản sử dụng
            </button>{" "}
            và{" "}
            <button
              type="button"
              className="text-link"
              onClick={() => setDialog("Chính sách bảo mật")}
            >
              Chính sách bảo mật
            </button>{" "}
            của AnomalyBank.
          </span>
        </label>
        <Next disabled={!consent || !email.trim()} />
      </form>
      <p className="bottom-link">
        Đã có tài khoản? <button onClick={() => onLogin()}>Đăng nhập</button>
      </p>
    </>
  );
}
