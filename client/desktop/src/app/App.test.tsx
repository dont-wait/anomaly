import { StrictMode } from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import App from "./App";

vi.mock("react-chartjs-2", () => ({
  Line: (props: { "aria-label"?: string }) => <canvas data-testid="chart-canvas" aria-label={props["aria-label"]} />,
}));

const session = {
  accessToken: "admin-token",
  expiresAt: "2030-01-01T00:00:00Z",
};

const adminProfile = {
  id: "admin-id",
  accountNo: "ACC-ADMIN",
  username: "admin.staff",
  fullName: "Admin Staff",
  email: "admin@example.com",
  role: "admin",
};

describe("App admin route", () => {
  beforeEach(() => {
    localStorage.clear();
    window.location.hash = "#/admin/login";
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("redirects direct monitor access without a session", async () => {
    window.location.hash = "#/admin/monitor";
    render(<App />);

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/login");
    });
  });

  it("restores a verified admin session", async () => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            status: 200,
            message: "ok",
            data: adminProfile,
          }),
          { status: 200 },
        ),
    );
    vi.stubGlobal("fetch", fetchMock);
    window.location.hash = "#/admin/monitor";
    render(
      <StrictMode>
        <App />
      </StrictMode>,
    );

    expect(
      await screen.findByRole("heading", {
        name: /Giám sát rủi ro & bất thường hệ thống/i,
      }),
    ).toBeTruthy();
    expect(screen.getAllByText("Admin Staff")).toHaveLength(2);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("renders the shared shell for another protected route", async () => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              status: 200,
              message: "ok",
              data: adminProfile,
            }),
            { status: 200 },
          ),
      ),
    );
    window.location.hash = "#/admin/alerts";
    render(<App />);

    expect(
      await screen.findByText("Màn hình đang được triển khai"),
    ).toBeTruthy();
    expect(screen.getByRole("navigation", { name: "Menu admin" })).toBeTruthy();
  });

  it("clears the session and returns to login on logout", async () => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ status: 200, message: "ok", data: adminProfile }),
            { status: 200 },
          ),
      ),
    );
    window.location.hash = "#/admin/monitor";
    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: "Đăng xuất" }));

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/login");
      expect(localStorage.getItem("anomaly.admin.session")).toBeNull();
    });
  });

  it("rejects a session whose account is no longer admin", async () => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              status: 200,
              message: "ok",
              data: { ...adminProfile, role: "user" },
            }),
            { status: 200 },
          ),
      ),
    );
    window.location.hash = "#/admin/monitor";
    render(<App />);

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/login");
      expect(localStorage.getItem("anomaly.admin.session")).toBeNull();
    });
  });

  it.each([
    ["an incomplete profile", JSON.stringify({ data: { role: "admin" } })],
    ["invalid JSON", "not-json"],
  ])("rejects a successful /me response with %s", async (_case, body) => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(body, { status: 200 })),
    );
    window.location.hash = "#/admin/monitor";
    render(<App />);

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/login");
      expect(localStorage.getItem("anomaly.admin.session")).toBeNull();
    });
  });

  it("clears the session for a non-retryable /me error", async () => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 400 })),
    );
    window.location.hash = "#/admin/monitor";
    render(<App />);

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/login");
      expect(localStorage.getItem("anomaly.admin.session")).toBeNull();
    });
  });

  it("keeps the session and retries a transient server failure", async () => {
    vi.useFakeTimers();
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    const fetchMock = vi.fn(async () => new Response(null, { status: 503 }));
    vi.stubGlobal("fetch", fetchMock);
    window.location.hash = "#/admin/monitor";
    render(<App />);

    expect(fetchMock).toHaveBeenCalledOnce();
    await act(async () => {
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(3000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(window.location.hash).toBe("#/admin/monitor");
    expect(localStorage.getItem("anomaly.admin.session")).not.toBeNull();
  });

  it("clears a session whose account no longer exists", async () => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 404 })),
    );
    window.location.hash = "#/admin/monitor";
    render(<App />);

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/login");
      expect(localStorage.getItem("anomaly.admin.session")).toBeNull();
    });
  });
});
