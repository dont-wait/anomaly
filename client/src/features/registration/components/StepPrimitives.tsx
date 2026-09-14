import { useState, type ReactNode, type RefObject } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faShieldHalved,
  faArrowRight,
  faLock,
  faEye,
  faEyeSlash,
} from "@fortawesome/free-solid-svg-icons";
import type { IconDefinition } from "@fortawesome/fontawesome-svg-core";
export function Notice({ children }: { children: ReactNode }) {
  return (
    <div className="notice">
      <FontAwesomeIcon icon={faShieldHalved} />
      <span>{children}</span>
    </div>
  );
}
export function Next({
  children = "Tiếp tục",
  disabled = false,
}: {
  children?: ReactNode;
  disabled?: boolean;
}) {
  return (
    <button className="primary" disabled={disabled} type="submit">
      {children}
      <FontAwesomeIcon icon={faArrowRight} />
    </button>
  );
}
export function Password({
  label,
  value,
  onChange,
  disabled = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const [visible, setVisible] = useState(false);
  return (
    <label className="field">
      {label}
      <span className="input-wrap">
        <FontAwesomeIcon icon={faLock} />
        <input
          required
          disabled={disabled}
          autoComplete={
            label === "Mật khẩu đăng nhập" ? "current-password" : "new-password"
          }
          type={visible ? "text" : "password"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder="Nhập mật khẩu"
        />
        <button
          type="button"
          className="icon-button"
          aria-label={visible ? `Ẩn ${label}` : `Hiện ${label}`}
          onClick={() => setVisible(!visible)}
        >
          <FontAwesomeIcon icon={visible ? faEyeSlash : faEye} />
        </button>
      </span>
    </label>
  );
}

export function StepHeading({
  headingRef,
  title,
  icon,
  children,
}: {
  headingRef: RefObject<HTMLHeadingElement | null>;
  title: string;
  icon: IconDefinition;
  children: ReactNode;
}) {
  return (
    <div className="screen-heading">
      <div className="hero-icon">
        <FontAwesomeIcon icon={icon} />
      </div>
      <h1 ref={headingRef} tabIndex={-1}>
        {title}
      </h1>
      <p>{children}</p>
    </div>
  );
}
