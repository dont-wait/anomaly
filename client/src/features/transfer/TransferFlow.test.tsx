import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { transactionStore } from "@/features/transactions";
import { clearBankCache } from "@/features/transfer/api/banks";
import { DEMO_OTP } from "@/features/transfer/api/transfer";
import { TransferFlow } from "./TransferFlow";

const source = {
  accountNo: "99999180105",
  ownerName: "Phạm Minh Hiếu",
  balance: 3_000_000,
};

const vietQrBanks = {
  code: "00",
  desc: "ok",
  data: [
    {
      code: "VCB",
      bin: "970436",
      shortName: "Vietcombank",
      name: "Ngân hàng TMCP Ngoại Thương Việt Nam",
      logo: "https://cdn.vietqr.io/img/VCB.png",
      transferSupported: 1,
    },
    {
      code: "TCB",
      bin: "970407",
      shortName: "Techcombank",
      name: "Ngân hàng TMCP Kỹ thương Việt Nam",
      logo: "https://cdn.vietqr.io/img/TCB.png",
      transferSupported: 1,
    },
    {
      code: "XYZ",
      bin: "000000",
      shortName: "NoTransfer",
      name: "Không hỗ trợ chuyển",
      logo: "",
      transferSupported: 0,
    },
  ],
};

function stubBanksApi(ok = true) {
  const fetchMock = vi.fn(async () =>
    ok
      ? new Response(JSON.stringify(vietQrBanks), { status: 200 })
      : Promise.reject(new TypeError("network down")),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

beforeEach(() => {
  clearBankCache();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderFlow() {
  const onExit = vi.fn();
  render(
    <TransferFlow source={source} onExit={onExit} onViewHistory={vi.fn()} />,
  );
  return { onExit };
}

async function enterAccount(accountNo: string) {
  const input = screen.getByLabelText("Số tài khoản");
  fireEvent.change(input, { target: { value: accountNo } });
  fireEvent.blur(input);
}

async function openOtpSheet(amount: string) {
  fireEvent.change(screen.getByLabelText("Số tiền chuyển"), {
    target: { value: amount },
  });
  fireEvent.click(screen.getByRole("button", { name: "Chuyển tiền" }));
  return screen.findByRole("dialog", { name: "Xác thực giao dịch" });
}

it("lists AnomalyBank first, then transfer-capable banks from VietQR", async () => {
  const fetchMock = stubBanksApi();
  renderFlow();
  expect(fetchMock).toHaveBeenCalledWith(
    "https://api.vietqr.io/v2/banks",
    expect.anything(),
  );

  const trigger = screen.getByRole("button", { name: /Ngân hàng AnomalyBank/ });
  fireEvent.click(trigger);
  const list = await screen.findByRole("listbox", {
    name: "Danh sách ngân hàng",
  });
  await waitFor(() =>
    expect(within(list).getAllByRole("option")).toHaveLength(3),
  );
  const names = within(list)
    .getAllByRole("option")
    .map((o) => o.textContent);
  expect(names[0]).toContain("AnomalyBank");
  expect(names.join()).not.toContain("NoTransfer");

  fireEvent.change(screen.getByRole("combobox"), {
    target: { value: "ngoại thương" },
  });
  expect(within(list).getAllByRole("option")).toHaveLength(1);
  fireEvent.click(within(list).getByRole("option", { name: /Vietcombank/ }));
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(
    screen.getByRole("button", { name: /Ngân hàng Vietcombank/ }),
  ).toBeTruthy();
});

it("keeps AnomalyBank usable when the bank list fails to load", async () => {
  stubBanksApi(false);
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: /Ngân hàng AnomalyBank/ }),
  );
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Không tải được",
  );
  expect(screen.getAllByRole("option")).toHaveLength(1);
});

it("validates account numbers inline and looks up the owner name", async () => {
  stubBanksApi();
  renderFlow();

  await enterAccount("123");
  expect(screen.getByRole("alert").textContent).toContain("6-19 chữ số");

  await enterAccount(source.accountNo);
  expect(screen.getByRole("alert").textContent).toContain("chính tài khoản");

  await enterAccount("11112222333");
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Không tìm thấy tài khoản tại AnomalyBank",
  );

  await enterAccount("99999180147");
  expect(await screen.findByText("TRAN THI BICH")).toBeTruthy();
});

it("looks up accounts at other banks after picking one", async () => {
  stubBanksApi();
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: /Ngân hàng AnomalyBank/ }),
  );
  const list = await screen.findByRole("listbox");
  fireEvent.click(
    await within(list).findByRole("option", { name: /Techcombank/ }),
  );
  await enterAccount("19036541234012");
  expect(await screen.findByText("LE THI MAI")).toBeTruthy();
});

it("blocks amounts above the balance", async () => {
  stubBanksApi();
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: /Chuyển đến TRAN THI BICH/ }),
  );
  fireEvent.change(screen.getByLabelText("Số tiền chuyển"), {
    target: { value: "5000000" },
  });
  expect(screen.getByRole("alert").textContent).toContain("Số dư không đủ");
  expect(
    (screen.getByRole("button", { name: "Chuyển tiền" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
});

it("shows the minimum-amount error on submit and clears it on change", async () => {
  stubBanksApi();
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: /Chuyển đến TRAN THI BICH/ }),
  );
  fireEvent.change(screen.getByLabelText("Số tiền chuyển"), {
    target: { value: "500" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Chuyển tiền" }));
  expect(screen.getByRole("alert").textContent).toContain("tối thiểu");
  expect(screen.queryByRole("dialog")).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "100.000" }));
  expect(screen.queryByRole("alert")).toBeNull();
});

it("confirms in a bottom sheet with OTP and shows the receipt there", async () => {
  stubBanksApi();
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: /Chuyển đến NGUYEN VAN AN/ }),
  );
  expect(
    screen.getByRole("button", { name: /Ngân hàng Vietcombank/ }),
  ).toBeTruthy();

  const sheet = await openOtpSheet("500000");
  expect(within(sheet).getByText("NGUYEN VAN AN")).toBeTruthy();
  expect(within(sheet).getByText("500.000₫")).toBeTruthy();

  const otp = within(sheet).getByLabelText("Mã OTP");
  fireEvent.change(otp, { target: { value: "000000" } });
  expect((await within(sheet).findByRole("alert")).textContent).toContain(
    "còn 2 lần",
  );
  expect((otp as HTMLInputElement).value).toBe("");

  fireEvent.change(otp, { target: { value: DEMO_OTP } });
  await screen.findByRole("dialog", { name: "Biên lai giao dịch" });
  expect(screen.getByText("Chuyển tiền thành công")).toBeTruthy();

  const latest = transactionStore.list()[0];
  expect(latest.amount).toBe(500_000);
  expect(latest.counterparty.bank).toBe("Vietcombank");
  expect(latest.balanceAfter).toBe(2_500_000);

  fireEvent.click(screen.getByRole("button", { name: "Giao dịch mới" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    (screen.getByLabelText("Số tài khoản") as HTMLInputElement).value,
  ).toBe("");
  expect(screen.getByText("2.500.000₫")).toBeTruthy();
});

it("cancels after too many wrong OTPs, then lets the user retry", async () => {
  stubBanksApi();
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: /Chuyển đến TRAN THI BICH/ }),
  );
  const sheet = await openOtpSheet("10000");
  const otp = within(sheet).getByLabelText("Mã OTP");
  for (const left of [2, 1]) {
    fireEvent.change(otp, { target: { value: "999999" } });
    await within(sheet).findByText(
      `Mã OTP không đúng. Bạn còn ${left} lần thử.`,
    );
  }
  fireEvent.change(otp, { target: { value: "999999" } });
  await screen.findByRole("dialog", { name: "Giao dịch chưa hoàn tất" });
  fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
  await screen.findByRole("dialog", { name: "Xác thực giao dịch" });
});

it("closes the sheet with Escape and keeps the form data", async () => {
  stubBanksApi();
  renderFlow();
  fireEvent.click(
    screen.getByRole("button", { name: /Chuyển đến TRAN THI BICH/ }),
  );
  const sheet = await openOtpSheet("10000");
  fireEvent.keyDown(sheet, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    (screen.getByLabelText("Số tài khoản") as HTMLInputElement).value,
  ).toBe("99999180147");
});

it("exits from the header back button", () => {
  stubBanksApi();
  const { onExit } = renderFlow();
  fireEvent.click(screen.getByRole("button", { name: "Về trang chủ" }));
  expect(onExit).toHaveBeenCalled();
});
