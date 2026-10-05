import { useState, type FormEvent } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faEye,
  faEyeSlash,
  faIdCard,
  faLock,
  faShieldHalved,
} from "@fortawesome/free-solid-svg-icons";
import { routes, navigate } from "@/app/routes";
import { loginAdmin } from "./api/adminAuth";

interface LoginErrors {
  cccdNumber?: string;
  password?: string;
}

export function LoginPage() {
  const [cccdNumber, setCccdNumber] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<LoginErrors>({});

  const validate = (): LoginErrors => {
    const errors: LoginErrors = {};
    const normalizedCccd = cccdNumber.replace(/\s+/g, "");
    if (!/^\d{12}$/.test(normalizedCccd)) {
      errors.cccdNumber = "Số CCCD phải gồm đúng 12 chữ số.";
    }
    if (!password) errors.password = "Vui lòng nhập mật khẩu.";
    return errors;
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const errors = validate();
    setFieldErrors(errors);
    setFormError(null);
    if (Object.keys(errors).length > 0) return;

    setIsSubmitting(true);
    try {
      await loginAdmin({
        cccdNumber: cccdNumber.replace(/\s+/g, ""),
        password,
      });
      navigate(routes.monitor);
    } catch (error) {
      setFormError(
        error instanceof Error
          ? error.message
          : "Không thể đăng nhập. Vui lòng thử lại.",
      );
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <main className="admin-login-page">
      <div className="admin-login-systembar" aria-label="Trạng thái hệ thống">
        <span className="admin-login-system-id">ANOMALY // ADMIN CONSOLE</span>
        <span>AUTH_GATEWAY / V1.0</span>
      </div>

      <section className="admin-login-card" aria-labelledby="login-title">
        <div className="admin-brand" aria-label="AnomalyBank Admin Console">
          <span className="admin-brand-mark" aria-hidden="true">
            <FontAwesomeIcon icon={faShieldHalved} />
          </span>
          <span className="admin-brand-name">AnomalyBank Admin</span>
        </div>

        <h1 className="admin-login-heading" id="login-title">
          Đăng nhập quản trị
        </h1>
        <p className="admin-login-subtitle">
          Sử dụng tài khoản nhân viên của bạn để tiếp tục.
        </p>

        <form className="admin-login-form" onSubmit={handleSubmit} noValidate>
          <div className="admin-field">
            <label className="admin-field-label" htmlFor="admin-cccd">
              Số CCCD
            </label>
            <div
              className="admin-field-control"
              data-invalid={Boolean(fieldErrors.cccdNumber)}
            >
              <FontAwesomeIcon
                className="admin-field-icon"
                icon={faIdCard}
                aria-hidden="true"
              />
              <input
                id="admin-cccd"
                name="cccdNumber"
                type="text"
                inputMode="numeric"
                autoComplete="off"
                maxLength={12}
                pattern="[0-9]*"
                placeholder="vd: 001234567890"
                value={cccdNumber}
                disabled={isSubmitting}
                aria-invalid={Boolean(fieldErrors.cccdNumber)}
                aria-describedby={
                  fieldErrors.cccdNumber ? "cccd-error" : undefined
                }
                onChange={(event) => setCccdNumber(event.target.value)}
              />
            </div>
            {fieldErrors.cccdNumber && (
              <p className="admin-field-error" id="cccd-error">
                {fieldErrors.cccdNumber}
              </p>
            )}
          </div>

          <div className="admin-field">
            <label className="admin-field-label" htmlFor="admin-password">
              Mật khẩu
            </label>
            <div
              className="admin-field-control"
              data-invalid={Boolean(fieldErrors.password)}
            >
              <FontAwesomeIcon
                className="admin-field-icon"
                icon={faLock}
                aria-hidden="true"
              />
              <input
                id="admin-password"
                name="password"
                type={showPassword ? "text" : "password"}
                autoComplete="current-password"
                placeholder="Nhập mật khẩu"
                value={password}
                disabled={isSubmitting}
                aria-invalid={Boolean(fieldErrors.password)}
                aria-describedby={
                  fieldErrors.password ? "password-error" : undefined
                }
                onChange={(event) => setPassword(event.target.value)}
              />
              <button
                className="admin-password-toggle"
                type="button"
                aria-label={showPassword ? "Ẩn mật khẩu" : "Hiện mật khẩu"}
                onClick={() => setShowPassword((visible) => !visible)}
              >
                <FontAwesomeIcon
                  icon={showPassword ? faEyeSlash : faEye}
                  aria-hidden="true"
                />
              </button>
            </div>
            {fieldErrors.password && (
              <p className="admin-field-error" id="password-error">
                {fieldErrors.password}
              </p>
            )}
          </div>

          {formError && (
            <p className="admin-form-error" role="alert">
              {formError}
            </p>
          )}

          <button
            className="admin-login-submit"
            type="submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? "Đang đăng nhập..." : "Đăng nhập"}
          </button>
        </form>

        <p className="admin-login-note">
          <FontAwesomeIcon icon={faShieldHalved} aria-hidden="true" />
          Khu vực truy cập nội bộ. Hoạt động đăng nhập được ghi nhận.
        </p>
        <p className="admin-login-footer">Anomaly Admin Console · v0.1.0</p>
      </section>

      <div className="admin-login-systemfooter">
        <span className="admin-login-system-status">SYSTEM READY</span>
        <span>INTERNAL ACCESS ONLY</span>
      </div>
    </main>
  );
}
