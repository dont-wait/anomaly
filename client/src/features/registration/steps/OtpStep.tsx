import {
  useEffect,
  useRef,
  type ClipboardEvent,
  type KeyboardEvent,
  type MouseEvent,
} from "react";
import type { RegistrationFlowState } from "../useRegistrationFlow";
import { OTP_LENGTH } from "../model";
import { Notice, Next, StepHeading } from "../components/StepPrimitives";
import { faEnvelope, faRotateRight } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

const digitsOnly = (value: string) => value.replace(/\D/g, "");

export function OtpStep({
  email,
  otpCode,
  setOtpCode,
  otpBusy,
  otpErrorMsg,
  otpErrorTick,
  progress,
  resendIn,
  resendOtp,
  heading,
  submit,
}: Pick<
  RegistrationFlowState,
  | "email"
  | "otpCode"
  | "setOtpCode"
  | "otpBusy"
  | "otpErrorMsg"
  | "otpErrorTick"
  | "progress"
  | "resendIn"
  | "resendOtp"
  | "heading"
  | "submit"
>) {
  // Các ô chỉ là cách hiển thị otpCode, không giữ state riêng: ô thứ i là ký
  // tự thứ i của mã. Không cho phép ô trống nằm giữa (xem handleFocus) nên
  // mã luôn là một chuỗi liền mạch tính từ ô đầu.
  const digits = Array.from(
    { length: OTP_LENGTH },
    (_, index) => otpCode[index] ?? "",
  );
  const boxes = useRef<(HTMLInputElement | null)[]>([]);
  const lastSubmitted = useRef("");
  // Mã vừa ghi, đọc được ngay trong cùng một lượt xử lý sự kiện (state của
  // flow chỉ cập nhật ở lần render kế tiếp).
  const codeRef = useRef(otpCode);
  useEffect(() => {
    codeRef.current = otpCode;
  });

  // Ô cuối vừa đầy thì tự xác thực. Mốc so sánh là nội dung mã đã gửi, nên
  // sửa đè một chữ số sau khi sai vẫn kích hoạt xác thực lại, còn mã y
  // nguyên thì không gửi lần hai.
  useEffect(() => {
    if (otpCode.length < OTP_LENGTH) {
      lastSubmitted.current = "";
      return;
    }
    if (otpBusy || lastSubmitted.current === otpCode) return;
    lastSubmitted.current = otpCode;
    submit();
  }, [otpCode, otpBusy, submit]);

  // Sau khi báo lỗi: mã hết hạn đã bị flow xoá nên con trỏ về ô đầu, mã sai
  // thì giữ nguyên số và đưa con trỏ về ô trống đầu tiên để user sửa.
  useEffect(() => {
    if (!otpErrorMsg) return;
    boxes.current[Math.min(codeRef.current.length, OTP_LENGTH - 1)]?.focus();
    // Dep theo counter để hai lần lỗi giống hệt nhau vẫn đưa lại con trỏ.
  }, [otpErrorMsg, otpErrorTick]);

  function focusAt(index: number) {
    boxes.current[Math.min(Math.max(index, 0), OTP_LENGTH - 1)]?.focus();
  }
  function commitCode(next: string) {
    codeRef.current = next;
    setOtpCode(next);
  }
  // Ô xa hơn phần mã đang nhập dở thì ghi vào vị trí trống đầu tiên, nhờ đó
  // mã không bao giờ thủng lỗ ở giữa mà vẫn không cần chặn focus.
  function writeAt(index: number) {
    return Math.min(index, otpCode.length);
  }
  function fillFrom(index: number, incoming: string) {
    const at = writeAt(index);
    const filled = (otpCode.slice(0, at) + incoming).slice(0, OTP_LENGTH);
    commitCode(filled);
    focusAt(filled.length);
  }
  // Chuột bấm vào ô xa thì kéo con trỏ về ô đang nhập dở. Chỉ chặn ở chuột:
  // Tab vẫn đi tự do ra nút Gửi lại / Xác thực khi mã chưa đủ số.
  function handleMouseDown(index: number, event: MouseEvent<HTMLInputElement>) {
    if (index <= otpCode.length) return;
    event.preventDefault();
    focusAt(otpCode.length);
  }
  function handleChange(index: number, raw: string) {
    const incoming = digitsOnly(raw);
    // Một phím gõ chỉ làm giá trị dài thêm đúng một ký tự; dài hơn thế nghĩa
    // là cả mã được đổ vào một ô (autofill one-time-code) nên trải ra các ô.
    if (raw.length > digits[index].length + 1) {
      if (incoming) fillFrom(index, incoming);
      return;
    }
    const at = writeAt(index);
    // Gõ đè lên ô đã có số cho raw hai ký tự; ký tự số cuối là ký tự vừa gõ.
    const typed = incoming.slice(-1);
    if (!typed) {
      commitCode(otpCode.slice(0, at));
      return;
    }
    commitCode(
      (otpCode.slice(0, at) + typed + otpCode.slice(at + 1)).slice(
        0,
        OTP_LENGTH,
      ),
    );
    focusAt(at + 1);
  }
  function handleKeyDown(
    index: number,
    event: KeyboardEvent<HTMLInputElement>,
  ) {
    if (event.key !== "Backspace") return;
    event.preventDefault();
    const at = writeAt(index);
    if (digits[at]) {
      commitCode(otpCode.slice(0, at));
      focusAt(at);
      return;
    }
    if (at === 0) return;
    commitCode(otpCode.slice(0, at - 1));
    focusAt(at - 1);
  }
  function handlePaste(index: number, event: ClipboardEvent<HTMLInputElement>) {
    const pasted = digitsOnly(event.clipboardData.getData("text"));
    if (!pasted) return;
    event.preventDefault();
    fillFrom(index, pasted);
  }

  return (
    <>
      <StepHeading
        headingRef={heading}
        title="Xác thực email"
        icon={faEnvelope}
      >
        Mã gồm 6 số đã được gửi tới <strong>{email}</strong>
      </StepHeading>
      <form className="form-card" onSubmit={submit}>
        <fieldset disabled={otpBusy}>
          <label className="field otp-label" htmlFor="otp-digit-1">
            Mã OTP
          </label>
          <div className="otp-boxes">
            {digits.map((digit, index) => (
              <input
                key={index}
                id={`otp-digit-${index + 1}`}
                ref={(node) => {
                  boxes.current[index] = node;
                }}
                className="otp-box"
                type="text"
                disabled={otpBusy}
                inputMode="numeric"
                pattern="[0-9]*"
                autoComplete={index === 0 ? "one-time-code" : "off"}
                autoCapitalize="off"
                aria-label={`Chữ số thứ ${index + 1} của mã OTP`}
                value={digit}
                onMouseDown={(event) => handleMouseDown(index, event)}
                onChange={(event) => handleChange(index, event.target.value)}
                onKeyDown={(event) => handleKeyDown(index, event)}
                onPaste={(event) => handlePaste(index, event)}
              />
            ))}
          </div>
          {progress && (
            <p role="status" className="field-hint centered">
              {progress}
            </p>
          )}
          {otpErrorMsg && (
            <p role="alert" className="error-text">
              {otpErrorMsg}
            </p>
          )}
          <p className="resend">
            Không nhận được mã?{" "}
            <button
              type="button"
              className="text-link"
              disabled={otpBusy || resendIn > 0}
              onClick={() => void resendOtp()}
            >
              <FontAwesomeIcon icon={faRotateRight} />{" "}
              {resendIn > 0 ? `Gửi lại sau ${resendIn}s` : "Gửi lại"}
            </button>
          </p>
          <Notice>
            Mã có hiệu lực 60 giây. Nhập sai quá 5 lần mã sẽ bị hủy.
          </Notice>
          <Next disabled={otpBusy || otpCode.length !== OTP_LENGTH}>
            Xác thực
          </Next>
        </fieldset>
      </form>
    </>
  );
}
