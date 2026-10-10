import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TransactionDateRangePicker } from "./TransactionDateRangePicker";

afterEach(cleanup);

it("keeps the popup open and confirms a highlighted date range", () => {
  const onConfirm = vi.fn();
  render(
    <TransactionDateRangePicker
      value={{ from: "2026-10-01", to: "2026-10-10" }}
      minDate="2026-08-10"
      maxDate="2026-10-10"
      onConfirm={onConfirm}
    />,
  );

  fireEvent.click(screen.getByRole("button", { name: "Ngày bắt đầu" }));
  fireEvent.click(screen.getByRole("button", { name: "05/10/2026" }));
  fireEvent.click(screen.getByRole("button", { name: "10/10/2026" }));

  expect(screen.getByRole("dialog")).toBeTruthy();
  const middleDay = screen.getByRole("button", { name: "07/10/2026" });
  expect(middleDay.parentElement?.querySelector("span")?.className).toContain(
    "bg-secondary/10",
  );

  fireEvent.click(screen.getByRole("button", { name: "Xác nhận" }));

  expect(onConfirm).toHaveBeenCalledWith({
    from: "2026-10-05",
    to: "2026-10-10",
  });
  expect(screen.queryByRole("dialog")).toBeNull();
});
