import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  AuthContext,
  type AuthContextValue,
} from "@/features/auth/authContext";
import { routes } from "@/app/routes";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import DashboardPage from "./DashboardPage";

const value = {
  status: AUTH_STATUS.AUTHENTICATED,
  token: "token",
  error: null,
  user: {
    id: "1",
    accountNo: "1234567890",
    username: "u",
    email: "a@example.com",
    currency: "VND",
    isVerify: true,
    amount: 0,
  },
  refreshProfile: vi.fn(async () => null),
  logout: vi.fn(),
} as unknown as AuthContextValue;

describe("Dashboard more services", () => {
  beforeEach(() => {
    window.location.hash = "";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        status: 200,
        text: async () =>
          JSON.stringify({ status: 200, message: "ok", data: { items: [] } }),
      })),
    );
  });

  it("opens a sheet with the loan service and goes to the loan list", () => {
    render(
      <AuthContext.Provider value={value}>
        <DashboardPage />
      </AuthContext.Provider>,
    );
    expect(screen.queryByRole("button", { name: "Vay vốn" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Xem thêm" }));
    fireEvent.click(screen.getByRole("button", { name: "Vay vốn" }));

    expect(window.location.hash).toBe(routes.loanApplications);
  });
});
