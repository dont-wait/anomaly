import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { transactionStore } from "@/features/transactions";
import { DEMO_OTP } from "@/features/transfer/api/transfer";
import { TransferFlow } from "./TransferFlow";

const source = {
  accountNo: "99999180105",
  ownerName: "Phạm Minh Hiếu",
  balance: 3_000_000,
};

afterEach(cleanup);

function renderFlow() {
  const onExit = vi.fn();
  render(
    <TransferFlow source={source} onExit={onExit} onViewHistory={vi.fn()} />,
  );
  return { onExit };
}

async function goToAmountStep() {
  fireEvent.change(screen.getByLabelText("Số tài khoản người nhận"), {
    target: { value: "99999180147" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
  await screen.findByRole("heading", { name: "Nhập số tiền" });
}

it("rejects invalid, own and unknown account numbers", async () => {
  renderFlow();
  const input = screen.getByLabelText("Số tài khoản người nhận");
  const next = screen.getByRole("button", { name: "Tiếp tục" });

  fireEvent.change(input, { target: { value: "123" } });
  fireEvent.click(next);
  expect(screen.getByRole("alert").textContent).toContain("8–19 chữ số");

  fireEvent.change(input, { target: { value: source.accountNo } });
  fireEvent.click(next);
  expect(screen.getByRole("alert").textContent).toContain("chính tài khoản");

  fireEvent.change(input, { target: { value: "11112222333" } });
  fireEvent.click(next);
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Không tìm thấy",
  );
});

it("blocks amounts above the available balance", async () => {
  renderFlow();
  await goToAmountStep();
  fireEvent.change(screen.getByLabelText("Số tiền chuyển"), {
    target: { value: "5000000" },
  });
  expect(screen.getByRole("alert").textContent).toContain("Số dư không đủ");
  expect(
    (screen.getByRole("button", { name: "Tiếp tục" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

it("transfers money end to end and records the transaction", async () => {
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: "Chuyển đến TRAN THI BICH" }),
  );
  await screen.findByRole("heading", { name: "Nhập số tiền" });
  expect(
    (screen.getByLabelText("Nội dung chuyển tiền") as HTMLTextAreaElement)
      .value,
  ).toBe("PHAM MINH HIEU chuyen tien");

  fireEvent.click(screen.getByRole("button", { name: "500.000" }));
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
  await screen.findByRole("heading", { name: "Xác nhận giao dịch" });
  fireEvent.click(screen.getByRole("button", { name: "Xác nhận chuyển tiền" }));

  const otp = await screen.findByLabelText("Mã OTP");
  fireEvent.change(otp, { target: { value: "000000" } });
  fireEvent.click(
    screen.getByRole("button", { name: "Xác thực và chuyển tiền" }),
  );
  expect((await screen.findByRole("alert")).textContent).toContain("còn 2 lần");

  fireEvent.change(otp, { target: { value: DEMO_OTP } });
  fireEvent.click(
    screen.getByRole("button", { name: "Xác thực và chuyển tiền" }),
  );
  await screen.findByRole("heading", { name: "Chuyển tiền thành công" });

  const latest = transactionStore.list()[0];
  expect(latest.amount).toBe(500_000);
  expect(latest.counterparty.accountNo).toBe("99999180147");
  expect(latest.balanceAfter).toBe(2_500_000);
});

it("cancels the transfer after too many wrong OTPs and allows retry", async () => {
  renderFlow();
  await goToAmountStep();
  fireEvent.change(screen.getByLabelText("Số tiền chuyển"), {
    target: { value: "10000" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Xác nhận chuyển tiền" }),
  );

  const otp = await screen.findByLabelText("Mã OTP");
  for (let i = 0; i < 3; i++) {
    fireEvent.change(otp, { target: { value: "999999" } });
    fireEvent.click(
      screen.getByRole("button", {
        name: /Xác thực và chuyển tiền|Đang xử lý/,
      }),
    );
    await screen
      .findByRole("button", { name: "Xác thực và chuyển tiền" })
      .catch(() => null);
  }
  await screen.findByRole("heading", { name: "Chuyển tiền thất bại" });
  fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
  await screen.findByRole("heading", { name: "Xác nhận giao dịch" });
});

it("goes back step by step, then exits from the first step", async () => {
  const { onExit } = renderFlow();
  await goToAmountStep();
  fireEvent.click(screen.getByRole("button", { name: "Quay lại bước trước" }));
  await screen.findByRole("heading", { name: "Chuyển đến ai?" });
  fireEvent.click(screen.getByRole("button", { name: "Về trang chủ" }));
  expect(onExit).toHaveBeenCalled();
});

async function transfer(amount: string) {
  fireEvent.click(
    await screen.findByRole("button", { name: "Chuyển đến TRAN THI BICH" }),
  );
  fireEvent.change(await screen.findByLabelText("Số tiền chuyển"), {
    target: { value: amount },
  });
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Xác nhận chuyển tiền" }),
  );
  fireEvent.change(await screen.findByLabelText("Mã OTP"), {
    target: { value: DEMO_OTP },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Xác thực và chuyển tiền" }),
  );
  await screen.findByRole("heading", { name: "Chuyển tiền thành công" });
}

it("deducts the balance between consecutive transfers", async () => {
  renderFlow();
  await transfer("2000000");
  expect(transactionStore.list()[0].balanceAfter).toBe(1_000_000);

  fireEvent.click(screen.getByRole("button", { name: "Giao dịch mới" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Chuyển đến TRAN THI BICH" }),
  );
  fireEvent.change(await screen.findByLabelText("Số tiền chuyển"), {
    target: { value: "2000000" },
  });
  expect(screen.getByRole("alert").textContent).toContain("Số dư không đủ");
  expect(screen.getByText("1.000.000₫")).toBeTruthy();
});

it("clears the minimum-amount error once the amount changes", async () => {
  renderFlow();
  await goToAmountStep();
  const input = screen.getByLabelText("Số tiền chuyển");
  fireEvent.change(input, { target: { value: "500" } });
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
  expect(screen.getByRole("alert").textContent).toContain("tối thiểu");

  fireEvent.click(screen.getByRole("button", { name: "100.000" }));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(input.getAttribute("aria-invalid")).toBe("false");
});
