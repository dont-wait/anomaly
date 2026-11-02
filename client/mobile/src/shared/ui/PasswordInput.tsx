import { useState, type ComponentProps } from "react";
import { faEye, faEyeSlash } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { Input } from "./Input";

/** Nút con mắt, chỉ đổi cách hiển thị nên không bị khóa khi form đang gửi. */
export function PasswordToggle({
  label,
  visible,
  onToggle,
  className,
}: {
  label: string;
  visible: boolean;
  onToggle: () => void;
  className?: string;
}) {
  const name = label.toLowerCase();
  return (
    <button
      type="button"
      className={className}
      aria-label={visible ? `Ẩn ${name}` : `Hiện ${name}`}
      onClick={onToggle}
    >
      <FontAwesomeIcon icon={visible ? faEyeSlash : faEye} />
    </button>
  );
}

type PasswordInputProps = Omit<
  ComponentProps<typeof Input>,
  "type" | "rightElement"
> & { label: string };

export function PasswordInput(props: PasswordInputProps) {
  const [visible, setVisible] = useState(false);
  return (
    <Input
      {...props}
      type={visible ? "text" : "password"}
      rightElement={
        <PasswordToggle
          label={props.label}
          visible={visible}
          onToggle={() => setVisible(!visible)}
          className="ml-2 flex items-center justify-center text-on-surface-variant/70 transition-colors hover:text-on-surface"
        />
      }
    />
  );
}
