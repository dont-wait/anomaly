import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { isProtectedRoute, routes } from "@/app/routes";
import {
  AuthContext,
  type AuthContextValue,
} from "@/features/auth/authContext";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import { BottomNav } from "@/shared/layout/BottomNav";
import CardsPage from "./CardsPage";

vi.mock("@/shared/layout/ProfileSidebar", () => ({
  ProfileSidebar: () => null,
}));

const fetchMock = vi.fn();
const accountNo = "99999180147";
const auth = {
  status: AUTH_STATUS.AUTHENTICATED,
  token: "jwt",
  error: null,
  logout: vi.fn(),
  user: { accountNo, username: "bich", fullName: "Tran Thi Bich" },
} as unknown as AuthContextValue;

const button = (name: string | RegExp) => screen.getByRole("button", { name });
const mount = () =>
  render(
    <AuthContext.Provider value={auth}>
      <CardsPage />
    </AuthContext.Provider>,
  );

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  window.location.hash = routes.cards;
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows the default card from the account, with the card tab active", () => {
  mount();
  expect(screen.getByRole("region", { name: "Thẻ mặc định" })).toBeTruthy();
  expect(screen.getByText("Tran Thi Bich")).toBeTruthy();
  expect(screen.getByText("•••• 0147")).toBeTruthy();
  expect(screen.queryByText(accountNo)).toBeNull();
  expect(button("Thẻ").getAttribute("aria-current")).toBe("page");
});

it("walks the flow without creating a card or calling the API", () => {
  mount();
  fireEvent.click(button("Mở thẻ mới"));
  expect(screen.getByText("Bước 1/2")).toBeTruthy();
  expect(screen.queryByRole("radio")).toBeNull();
  expect(screen.getByRole("region", { name: "Thẻ mới" })).toBeTruthy();
  expect(screen.getByText("Tran Thi Bich")).toBeTruthy();
  expect(screen.getByText("•••• 0147")).toBeTruthy();
  expect(screen.queryByText(accountNo)).toBeNull();

  fireEvent.click(button("Tiếp tục"));
  expect(screen.getByText("Bước 2/2")).toBeTruthy();
  expect(screen.getByText(/Chưa có thẻ mới nào được tạo/)).toBeTruthy();
  expect(screen.queryByText(/thành công/i)).toBeNull();

  fireEvent.click(button("Về màn Thẻ"));
  expect(screen.getByRole("region", { name: "Thẻ mặc định" })).toBeTruthy();
  expect(fetchMock).not.toHaveBeenCalled();
});

it("goes back one step at a time", () => {
  mount();
  fireEvent.click(button("Mở thẻ mới"));
  fireEvent.click(button("Tiếp tục"));
  fireEvent.click(button("Quay lại"));
  expect(screen.getByText("Bước 1/2")).toBeTruthy();
  fireEvent.click(button("Quay lại"));
  expect(screen.getByRole("region", { name: "Thẻ mặc định" })).toBeTruthy();
});

it("navigates from the bottom bar", () => {
  mount();
  fireEvent.click(button("Tiết kiệm"));
  expect(window.location.hash).toBe(routes.savings);
  fireEvent.click(button("Quét mã QR"));
  expect(window.location.hash).toBe(routes.qrScan);
});

it("protects the cards route", () => {
  expect(isProtectedRoute(routes.cards)).toBe(true);
});

it("marks the active tab and still reports selections", () => {
  const onChange = vi.fn();
  const onQrScan = vi.fn();
  render(
    <BottomNav active="savings" onChange={onChange} onQrScan={onQrScan} />,
  );
  expect(button("Tiết kiệm").getAttribute("aria-current")).toBe("page");
  expect(button("Thẻ").getAttribute("aria-current")).toBeNull();
  fireEvent.click(button("Thẻ"));
  fireEvent.click(button("Quét mã QR"));
  expect(onChange).toHaveBeenCalledWith("card");
  expect(onQrScan).toHaveBeenCalledOnce();
});
