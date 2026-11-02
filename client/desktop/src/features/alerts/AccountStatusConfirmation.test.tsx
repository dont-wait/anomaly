import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AccountStatusConfirmation } from "./AccountStatusConfirmation";

beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.setAttribute("open", "");
    },
  });
  Object.defineProperty(HTMLDialogElement.prototype, "close", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.removeAttribute("open");
    },
  });
});

afterEach(() => vi.restoreAllMocks());

it.each(["locked", "active"] as const)(
  "requires exact OK before confirming %s",
  async (status) => {
    const onConfirm = vi.fn().mockResolvedValue(undefined);
    render(
      <AccountStatusConfirmation
        accountNo="79548859179658"
        status={status}
        onConfirm={onConfirm}
        onCancel={vi.fn()}
      />,
    );
    const input = screen.getByRole("textbox", { name: /OK/ });
    const reason =
      status === "locked"
        ? screen.getByLabelText("Lý do khóa tài khoản")
        : null;
    const button = screen.getByRole("button", {
      name: status === "locked" ? "Khóa tài khoản" : "Mở khóa tài khoản",
    });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    for (const value of ["ok", "OK ", ""]) {
      fireEvent.change(input, { target: { value } });
      fireEvent.click(button);
      expect(onConfirm).not.toHaveBeenCalled();
    }
    fireEvent.change(input, { target: { value: "OK" } });
    if (reason)
      fireEvent.change(reason, { target: { value: "Đăng nhập bất thường" } });
    fireEvent.click(button);
    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  },
);

it("cancels without changing the account", () => {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  render(
    <AccountStatusConfirmation
      accountNo="79548859179658"
      status="locked"
      onConfirm={onConfirm}
      onCancel={onCancel}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Hủy" }));
  expect(onCancel).toHaveBeenCalledTimes(1);
  expect(onConfirm).not.toHaveBeenCalled();
});
