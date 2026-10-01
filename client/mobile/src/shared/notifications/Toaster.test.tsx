import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Toaster } from "./Toaster";
import { toast, TOAST_DURATION_MS } from "./toast";

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  toast.dismiss();
  vi.useRealTimers();
});

it("automatically hides the notification after five seconds", () => {
  render(<Toaster />);
  act(() => toast.error("Không thể đăng nhập"));
  expect(screen.getByRole("alert").textContent).toContain("Không thể đăng nhập");
  act(() => vi.advanceTimersByTime(TOAST_DURATION_MS - 1));
  expect(screen.queryByRole("alert")).not.toBeNull();
  act(() => vi.advanceTimersByTime(1));
  expect(screen.queryByRole("alert")).toBeNull();
});

it("gives a replacement notification its full duration", () => {
  render(<Toaster />);
  act(() => toast.error("Lỗi đầu tiên"));
  act(() => vi.advanceTimersByTime(3000));
  act(() => toast.error("Lỗi mới"));
  act(() => vi.advanceTimersByTime(2000));
  expect(screen.getByRole("alert").textContent).toContain("Lỗi mới");
  act(() => vi.advanceTimersByTime(3000));
  expect(screen.queryByRole("alert")).toBeNull();
});

it("can be dismissed manually and clears its timeout", () => {
  render(<Toaster />);
  act(() => toast.error("Lỗi"));
  fireEvent.click(screen.getByRole("button", { name: "Đóng thông báo" }));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(vi.getTimerCount()).toBe(0);
});
