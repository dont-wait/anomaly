import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import SavingsPage from "./SavingsPage";

afterEach(() => cleanup());

describe("Stitch savings UI mock", () => {
  it("shows the savings list, filters and opens a detail screen", () => {
    render(<SavingsPage />);
    expect(screen.queryByText(/Bản xem trước giao diện/)).toBeNull();
    expect(screen.getByRole("heading", { name: "Tiết kiệm của tôi" })).toBeTruthy();
    expect(screen.getByText("#TK-2026-9921")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Đã đáo hạn \(1\)/ }));
    expect(screen.queryByText("#TK-2026-9921")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /#TK-2025-4108/ }));
    expect(screen.getByRole("heading", { name: "Chi tiết sổ tiết kiệm" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Rút tiền tiết kiệm" })).toBeTruthy();
  });

  it("requires consent for both mock deposit and mock withdrawal", () => {
    render(<SavingsPage />);
    fireEvent.click(screen.getByRole("button", { name: /Gửi tiết kiệm mới/ }));
    expect(screen.getByRole("heading", { name: "Mở sổ tiết kiệm" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Tiếp tục/ }));
    const confirm = screen.getByRole("button", { name: /Xác nhận gửi/ });
    expect(confirm.hasAttribute("disabled")).toBe(true);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(confirm);
    expect(screen.getByText("Gửi tiết kiệm thành công!")).toBeTruthy();
    expect(screen.getByText(/chưa có giao dịch ngân hàng/)).toBeTruthy();
  });

  it("blocks withdrawal of an active contract", () => {
    render(<SavingsPage />);
    fireEvent.click(screen.getByRole("button", { name: /#TK-2026-9921/ }));
    expect(screen.getByRole("button", { name: "Chưa đến ngày đáo hạn" }).hasAttribute("disabled")).toBe(true);
  });

  it("settles only a matured fixture, changing mock state without an API call", () => {
    render(<SavingsPage />);
    fireEvent.click(screen.getByRole("button", { name: /#TK-2025-4108/ }));
    fireEvent.click(screen.getByRole("button", { name: "Rút tiền tiết kiệm" }));
    const confirm = screen.getByRole("button", { name: "Xác nhận rút tiền" });
    expect(confirm.hasAttribute("disabled")).toBe(true);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(confirm);
    expect(screen.getByText("Rút tiết kiệm thành công!")).toBeTruthy();
    expect(screen.getByText(/tiền không được cộng vào tài khoản thật/)).toBeTruthy();
  });
});
