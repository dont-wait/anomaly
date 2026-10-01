import { useState, type FormEvent } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faEye,
  faEyeSlash,
  faLock,
  faShieldHalved,
  faUser,
} from "@fortawesome/free-solid-svg-icons";
import { routes, navigate } from "@/app/routes";
import { loginAdmin } from "./api/adminAuth";

interface LoginErrors {
  login?: string;
  password?: string;
}

export function LoginPage() {
  const [login, setLogin] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<LoginErrors>({});

  const validate = (): LoginErrors => {
    const errors: LoginErrors = {};
    if (!login.trim()) errors.login = "Vui lòng nhập email hoặc username.";
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
      await loginAdmin({ login: login.trim(), password });
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
            <label className="admin-field-label" htmlFor="admin-login">
              Email hoặc username
            </label>
            <div
              className="admin-field-control"
              data-invalid={Boolean(fieldErrors.login)}
            >
              <FontAwesomeIcon
                className="admin-field-icon"
                icon={faUser}
                aria-hidden="true"
              />
              <input
                id="admin-login"
                name="login"
                type="text"
                autoComplete="username"
                placeholder="vd: risk@example.com"
                value={login}
                disabled={isSubmitting}
                aria-invalid={Boolean(fieldErrors.login)}
                aria-describedby={fieldErrors.login ? "login-error" : undefined}
                onChange={(event) => setLogin(event.target.value)}
              />
            </div>
            {fieldErrors.login && (
              <p className="admin-field-error" id="login-error">
                {fieldErrors.login}
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
    </main>
  );
}
