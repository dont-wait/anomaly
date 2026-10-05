import { StrictMode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import App from "./App";

const session = {
  accessToken: "admin-token",
  expiresAt: "2030-01-01T00:00:00Z",
};

describe("App admin route", () => {
  beforeEach(() => {
    localStorage.clear();
    window.location.hash = "#/admin/login";
  });

  afterEach(() => {
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
            data: { role: "admin" },
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
      await screen.findByText("Monitoring workspace sẵn sàng"),
    ).toBeTruthy();
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
              data: { role: "admin" },
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
              data: { role: "user" },
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

  it("keeps a valid session during a transient server failure", async () => {
    localStorage.setItem("anomaly.admin.session", JSON.stringify(session));
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 503 })),
    );
    window.location.hash = "#/admin/monitor";
    render(<App />);

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/monitor");
      expect(localStorage.getItem("anomaly.admin.session")).not.toBeNull();
    });
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
