import type { RegistrationFlowState } from "../useRegistrationFlow";
import { Next, Password, StepHeading } from "../components/StepPrimitives";
import { faCheck, faLock } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { passwordRules } from "../model";

export function PasswordStep({
  error,
  busy,
  progress,
  createdAccount,
  password,
  setPassword,
  confirm,
  setConfirm,
  heading,
  submit,
}: Pick<
  RegistrationFlowState,
  | "error"
  | "busy"
  | "progress"
  | "createdAccount"
  | "password"
  | "setPassword"
  | "confirm"
  | "setConfirm"
  | "heading"
  | "submit"
>) {
  return (
    <>
      <StepHeading
        headingRef={heading}
        title="Tạo mật khẩu đăng nhập"
        icon={faLock}
      >
        Bước cuối cùng để bảo vệ tài khoản của bạn.
      </StepHeading>
      <form className="form-card" onSubmit={submit}>
        <fieldset disabled={busy}>
          <Password
            disabled={!!createdAccount}
            label="Mật khẩu"
            value={password}
            onChange={setPassword}
          />
          <Password
            disabled={!!createdAccount}
            label="Xác nhận mật khẩu"
            value={confirm}
            onChange={setConfirm}
          />
          <ul className="password-rules">
            {[
              "Ít nhất 8 ký tự",
              "Chữ hoa và chữ thường",
              "Có ít nhất 1 chữ số và ký tự đặc biệt",
            ].map((rule, i) => (
              <li
                key={rule}
                className={passwordRules(password)[i] ? "met" : ""}
              >
                <FontAwesomeIcon icon={faCheck} />
                {rule}
              </li>
            ))}
          </ul>
          {confirm && password !== confirm && (
            <p role="alert" className="error-text">
              Mật khẩu xác nhận chưa khớp.
            </p>
          )}
          {createdAccount && (
            <p className="field-hint">
              Tài khoản đã được tạo. Nếu bước sau gặp lỗi, nhấn tiếp tục để hoàn
              tất xác thực.
            </p>
          )}
          {error && (
            <p role="alert" className="error-text">
              {error}
            </p>
          )}
          {busy && <p role="status">{progress}</p>}
          <Next
            disabled={
              busy ||
              !passwordRules(password).every(Boolean) ||
              password !== confirm
            }
          >
            {busy
              ? "Đang xử lý…"
              : createdAccount
                ? "Tiếp tục xác thực"
                : "Hoàn tất đăng ký"}
          </Next>
        </fieldset>
      </form>
    </>
  );
}
