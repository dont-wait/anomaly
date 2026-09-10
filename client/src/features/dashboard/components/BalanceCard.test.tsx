import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BalanceCard } from "./BalanceCard";
import { mockAccount } from "@/mocks/dashboard";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("copies the full account number while keeping the displayed number masked", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard: { writeText } });
  render(<BalanceCard account={mockAccount} />);

  fireEvent.click(
    screen.getByRole("button", { name: "Sao chép số tài khoản" }),
  );

  expect(writeText).toHaveBeenCalledWith(mockAccount.accountNumber);
  expect(await screen.findByText("Đã sao chép số tài khoản")).toBeTruthy();
  expect(screen.queryByText(mockAccount.accountNumber)).toBeNull();
});

it.each(["denied", "unavailable"])(
  "reports a %s clipboard without an unhandled error",
  async (reason) => {
    vi.stubGlobal("navigator", {
      clipboard:
        reason === "denied"
          ? { writeText: vi.fn().mockRejectedValue(new Error("Denied")) }
          : undefined,
    });
    render(<BalanceCard account={mockAccount} />);

    fireEvent.click(
      screen.getByRole("button", { name: "Sao chép số tài khoản" }),
    );

    expect(
      await screen.findByText(
        "Không thể sao chép số tài khoản. Vui lòng thử lại.",
      ),
    ).toBeTruthy();
  },
);
