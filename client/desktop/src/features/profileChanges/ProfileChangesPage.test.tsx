import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { request } from "@/features/loans/api/loansApi";
import { ProfileChangesPage } from "./ProfileChangesPage";

vi.mock("@/features/loans/api/loansApi", async (original) => ({
  ...(await original<object>()),
  request: vi.fn(),
}));

const item = {
  accountId: "acc-1",
  username: "Khách A",
  current: {
    fullName: "Tên cũ",
    addressLine: "",
    provinceCode: "",
    issuedPlace: "",
    permanentAddress: "",
  },
  change: {
    id: "c1",
    fullName: "Tên mới",
    requestedAt: "2026-11-01T00:00:00Z",
  },
};

beforeEach(() => {
  vi.mocked(request).mockReset();
  vi.mocked(request).mockImplementation(async (path: string) =>
    path.endsWith("/decision") ? {} : { data: [item] },
  );
});
afterEach(cleanup);

it("shows current and proposed values and approves", async () => {
  render(<ProfileChangesPage />);
  expect(await screen.findByText("Tên mới")).toBeTruthy();
  expect(screen.getByText("Tên cũ")).toBeTruthy();
  expect(screen.queryByText("Nơi cấp CCCD")).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "Duyệt" }));

  await waitFor(() => expect(screen.queryByText("Tên mới")).toBeNull());
  expect(request).toHaveBeenCalledWith(
    "/api/admin/profile-changes/acc-1/c1/decision",
    {
      method: "POST",
      body: JSON.stringify({ decision: "APPROVED", reason: "" }),
    },
  );
});

it("requires a reason to reject", async () => {
  render(<ProfileChangesPage />);
  fireEvent.click(await screen.findByRole("button", { name: "Từ chối" }));
  const confirm = screen.getByRole<HTMLButtonElement>("button", {
    name: "Xác nhận từ chối",
  });
  expect(confirm.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Lý do từ chối"), {
    target: { value: "Sai tên" },
  });
  fireEvent.click(confirm);
  await waitFor(() => expect(screen.queryByText("Tên mới")).toBeNull());
});
