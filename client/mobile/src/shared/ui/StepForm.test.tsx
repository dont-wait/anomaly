import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StepForm } from "./StepForm";

afterEach(cleanup);

const setup = (canContinue: boolean) => {
  const onBack = vi.fn();
  const onNext = vi.fn();
  render(
    <StepForm
      title="Luồng"
      stepId="a"
      stepNumber={1}
      stepCount={2}
      heading="Bước đầu"
      canContinue={canContinue}
      onBack={onBack}
      onNext={onNext}
    >
      <input aria-label="Ô nhập" />
    </StepForm>,
  );
  return { onBack, onNext };
};

it("offers back and continue, and focuses the field", () => {
  const { onBack, onNext } = setup(true);
  expect(document.activeElement).toBe(screen.getByLabelText("Ô nhập"));

  fireEvent.click(screen.getByRole("button", { name: "Quay lại" }));
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));

  expect(onBack).toHaveBeenCalledTimes(1);
  expect(onNext).toHaveBeenCalledTimes(1);
});

it("does not continue on Enter while the step is invalid", () => {
  const { onNext } = setup(false);
  fireEvent.submit(screen.getByLabelText("Ô nhập").closest("form")!);
  expect(onNext).not.toHaveBeenCalled();
});
