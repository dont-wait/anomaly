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
import type { ProfileChangeItem } from "./ProfileChangesPage";

vi.mock("@/features/loans/api/loansApi", async (original) => ({
  ...(await original<object>()),
  request: vi.fn(),
}));

const item: ProfileChangeItem = {
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
    status: "PENDING",
    requestedAt: "2026-11-01T00:00:00Z",
  },
};

const pageData = (
  items: ProfileChangeItem[],
  page = 1,
  total = items.length,
) => ({
  data: { items, page, pageSize: 10, total, totalPages: Math.ceil(total / 10) },
});

beforeEach(() => {
  vi.mocked(request).mockReset();
  let reviewed = false;
  vi.mocked(request).mockImplementation(async (path: string) => {
    if (path.endsWith("/decision")) {
      reviewed = true;
      return {};
    }
    return pageData(reviewed ? [] : [item]);
  });
});
afterEach(cleanup);

it("shows current and proposed values, approves and reloads the page", async () => {
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
  expect(
    vi.mocked(request).mock.calls.filter(([path]) => path.includes("?")),
  ).toHaveLength(2);
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
  expect(request).toHaveBeenCalledWith(
    "/api/admin/profile-changes/acc-1/c1/decision",
    {
      method: "POST",
      body: JSON.stringify({ decision: "REJECTED", reason: "Sai tên" }),
    },
  );
});

it("filters status and searches, retaining filters when refreshing", async () => {
  render(<ProfileChangesPage />);
  expect(await screen.findByText("Tên mới")).toBeTruthy();

  fireEvent.change(screen.getByLabelText("Trạng thái yêu cầu"), {
    target: { value: "APPROVED" },
  });
  await waitFor(() => {
    expect(
      vi
        .mocked(request)
        .mock.calls.some(([path]) => path.includes("status=APPROVED")),
    ).toBe(true);
  });

  fireEvent.change(screen.getByLabelText("Tìm khách hàng"), {
    target: { value: "  Khách A  " },
  });
  fireEvent.click(screen.getByRole("button", { name: "Tìm kiếm" }));
  await waitFor(() => {
    expect(
      vi
        .mocked(request)
        .mock.calls.some(
          ([path]) =>
            path.includes("status=APPROVED") &&
            path.includes("search=Kh%C3%A1ch+A"),
        ),
    ).toBe(true);
  });
  const before = vi.mocked(request).mock.calls.length;
  fireEvent.click(screen.getByRole("button", { name: "Làm mới" }));
  await waitFor(() =>
    expect(vi.mocked(request).mock.calls.length).toBe(before + 1),
  );
  expect(vi.mocked(request).mock.lastCall?.[0]).toContain("status=APPROVED");
});

it("uses the server page and allows navigation", async () => {
  vi.mocked(request).mockImplementation(async (path: string) => {
    const page = Number(new URLSearchParams(path.split("?")[1]).get("page"));
    return pageData([item], page, 11);
  });
  render(<ProfileChangesPage />);
  expect(await screen.findByText("Tên mới")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Trang sau" }));
  await waitFor(() => {
    expect(
      vi.mocked(request).mock.calls.some(([path]) => path.includes("page=2")),
    ).toBe(true);
  });
  expect(await screen.findByText("11 yêu cầu · Trang 2/2")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Trang trước" }));
  await waitFor(() =>
    expect(vi.mocked(request).mock.lastCall?.[0]).toContain("page=1"),
  );
});

it("shows reviewed requests as read-only", async () => {
  vi.mocked(request).mockImplementation(async () =>
    pageData([
      {
        ...item,
        change: {
          ...item.change,
          status: "REJECTED",
          rejectionReason: "Không hợp lệ",
        },
      },
    ]),
  );
  render(<ProfileChangesPage />);
  expect(await screen.findByText("Đã từ chối")).toBeTruthy();
  expect(screen.getByText("Lý do: Không hợp lệ")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Duyệt" })).toBeNull();
});
