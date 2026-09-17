import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  useState,
  type Dispatch,
  type FormEvent,
  type SetStateAction,
} from "react";
import { OtpStep } from "./OtpStep";

afterEach(cleanup);

type Overrides = Partial<Parameters<typeof OtpStep>[0]>;

function renderStep(overrides: Overrides = {}) {
  const submit = vi.fn();
  const { otpCode: initialCode, ...rest } = overrides;
  function Harness() {
    const [otpCode, setOtpCode] = useState(initialCode ?? "");
    const handleChange: Dispatch<SetStateAction<string>> = (value) =>
      setOtpCode(typeof value === "function" ? value(otpCode) : value);
    return (
      <OtpStep
        email="a@example.com"
        otpCode={otpCode}
        setOtpCode={handleChange}
        otpBusy={false}
        otpErrorMsg=""
        otpErrorTick={0}
        progress=""
        resendIn={0}
        resendOtp={vi.fn()}
        heading={{ current: null }}
        submit={submit as (event?: FormEvent) => void}
        {...rest}
      />
    );
  }
  return { ...render(<Harness />), submit };
}

const box = (position: number) =>
  screen.getByLabelText(
    `Chữ số thứ ${position} của mã OTP`,
  ) as HTMLInputElement;
const boxValues = () =>
  [1, 2, 3, 4, 5, 6].map((position) => box(position).value);
const type = (position: number, value: string) =>
  fireEvent.change(box(position), {
    target: { value: box(position).value + value },
  });

it("renders six labelled boxes and the destination email", () => {
  renderStep();
  expect(screen.getByText("a@example.com")).toBeTruthy();
  expect(boxValues()).toEqual(["", "", "", "", "", ""]);
  expect(box(1).inputMode).toBe("numeric");
  expect(box(1).autocomplete).toBe("one-time-code");
  expect(box(2).autocomplete).toBe("off");
});

it("moves focus to the next box as digits are typed", () => {
  renderStep();
  box(1).focus();
  type(1, "1");
  expect(document.activeElement).toBe(box(2));
  type(2, "2");
  expect(document.activeElement).toBe(box(3));
  expect(boxValues()).toEqual(["1", "2", "", "", "", ""]);
});

it("ignores characters that are not digits", () => {
  renderStep();
  fireEvent.change(box(1), { target: { value: "a" } });
  expect(box(1).value).toBe("");
  expect(document.activeElement).not.toBe(box(2));
  type(1, "7");
  expect(box(1).value).toBe("7");
});

it("replaces the digit when typing over a filled box", () => {
  renderStep({ otpCode: "123456" });
  type(2, "9");
  expect(boxValues()).toEqual(["1", "9", "3", "4", "5", "6"]);
});

it("clears the current box on backspace, then steps back when it is empty", () => {
  renderStep({ otpCode: "123456" });
  box(6).focus();
  fireEvent.keyDown(box(6), { key: "Backspace" });
  expect(boxValues()).toEqual(["1", "2", "3", "4", "5", ""]);

  fireEvent.keyDown(box(6), { key: "Backspace" });
  expect(boxValues()).toEqual(["1", "2", "3", "4", "", ""]);
  expect(document.activeElement).toBe(box(5));
});

it("clears from a middle box onwards so the code never has a gap", () => {
  renderStep({ otpCode: "123456" });
  fireEvent.keyDown(box(3), { key: "Backspace" });
  expect(boxValues()).toEqual(["1", "2", "", "", "", ""]);
});

it("sends the caret to the box being filled when a later box is clicked", () => {
  renderStep({ otpCode: "12" });
  fireEvent.mouseDown(box(6));
  expect(document.activeElement).toBe(box(3));
});

it("lets the keyboard reach a later box instead of trapping focus", () => {
  renderStep({ otpCode: "12" });
  // Tab chỉ đặt focus, không kèm mousedown -> không bị kéo ngược về ô 3.
  box(6).focus();
  expect(document.activeElement).toBe(box(6));
});

it("appends to the first empty box when typing in a box reached by keyboard", () => {
  renderStep({ otpCode: "12" });
  box(6).focus();
  type(6, "9");
  expect(boxValues()).toEqual(["1", "2", "9", "", "", ""]);
  expect(document.activeElement).toBe(box(4));
});

it("spreads a code autofilled into a single box", () => {
  const { submit } = renderStep();
  fireEvent.change(box(1), { target: { value: "123456" } });
  expect(boxValues()).toEqual(["1", "2", "3", "4", "5", "6"]);
  expect(submit).toHaveBeenCalledTimes(1);
});

it("verifies again after a digit is corrected in place", () => {
  const { submit } = renderStep({ otpCode: "12345" });
  type(6, "6");
  expect(submit).toHaveBeenCalledTimes(1);

  // Mã sai: gõ đè ô cuối, độ dài vẫn là 6 nhưng nội dung đổi.
  type(6, "7");
  expect(boxValues()).toEqual(["1", "2", "3", "4", "5", "7"]);
  expect(submit).toHaveBeenCalledTimes(2);
});

it("spreads a pasted code across the boxes and verifies it", () => {
  const { submit } = renderStep();
  fireEvent.paste(box(1), {
    clipboardData: { getData: () => "12-34 56" },
  });
  expect(boxValues()).toEqual(["1", "2", "3", "4", "5", "6"]);
  expect(submit).toHaveBeenCalledTimes(1);
});

it("verifies automatically once the last box is filled", () => {
  const { submit } = renderStep({ otpCode: "12345" });
  expect(submit).not.toHaveBeenCalled();
  type(6, "6");
  expect(submit).toHaveBeenCalledTimes(1);
});

it("does not verify again while the code stays complete", () => {
  const { submit, rerender } = renderStep({ otpCode: "12345" });
  type(6, "6");
  expect(submit).toHaveBeenCalledTimes(1);
  rerender(<div />);
  expect(submit).toHaveBeenCalledTimes(1);
});

it("keeps the digits and focuses the last box when the code is rejected", () => {
  renderStep({ otpCode: "123456", otpErrorMsg: "Mã OTP không đúng." });
  expect(boxValues()).toEqual(["1", "2", "3", "4", "5", "6"]);
  expect(document.activeElement).toBe(box(6));
  expect(screen.getByRole("alert").textContent).toBe("Mã OTP không đúng.");
});

it("empties the boxes and focuses the first one when the code has expired", () => {
  renderStep({
    otpCode: "",
    otpErrorMsg: "Mã OTP đã hết hạn. Gửi lại mã mới.",
  });
  expect(boxValues()).toEqual(["", "", "", "", "", ""]);
  expect(document.activeElement).toBe(box(1));
});

it("disables every box while a request is in flight", () => {
  renderStep({ otpCode: "123456", otpBusy: true });
  expect(boxValues().length).toBe(6);
  for (const position of [1, 2, 3, 4, 5, 6])
    expect(box(position).disabled).toBe(true);
  expect(
    (screen.getByRole("button", { name: /Gửi lại/ }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

it("counts the resend cooldown down in the button label", () => {
  const resendOtp = vi.fn();
  renderStep({ resendIn: 25, resendOtp });
  const resend = screen.getByRole("button", {
    name: /Gửi lại/,
  }) as HTMLButtonElement;
  expect(resend.textContent).toContain("Gửi lại sau 25s");
  expect(resend.disabled).toBe(true);
  fireEvent.click(resend);
  expect(resendOtp).not.toHaveBeenCalled();
});
