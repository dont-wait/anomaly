import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AuthProvider } from "./AuthProvider";
import { useAuth } from "./useAuth";
import type { AuthContextValue } from "./authContext";

const profile = (fullName: string) => ({
  id: "1",
  accountNo: "1234567890",
  username: "u",
  fullName,
  email: "a@example.com",
  currency: "VND",
  isVerify: true,
  amount: 0,
});

const envelope = (status: number, data: unknown) =>
  ({
    ok: status < 400,
    status,
    text: async () => JSON.stringify({ status, message: "ok", data }),
  }) as Response;

function setup() {
  let auth!: AuthContextValue;
  const Probe = () => {
    auth = useAuth();
    return <p>{auth.user?.fullName ?? "none"}</p>;
  };
  const tokenStore = {
    getToken: vi.fn(() => "token" as string | null),
    setToken: vi.fn(),
    clearToken: vi.fn(),
  };
  render(
    <AuthProvider tokenStore={tokenStore}>
      <Probe />
    </AuthProvider>,
  );
  return { auth: () => auth, tokenStore };
}

afterEach(() => vi.unstubAllGlobals());

describe("AuthProvider profile state", () => {
  it("ignores an older profile read that resolves after a newer one", async () => {
    const resolvers: Array<(response: Response) => void> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise((resolve) => resolvers.push(resolve))),
    );
    const { auth } = setup();
    await waitFor(() => expect(resolvers).toHaveLength(1));
    act(() => resolvers[0](envelope(200, profile("initial"))));
    await screen.findByText("initial");

    let older!: Promise<unknown>;
    let newer!: Promise<unknown>;
    act(() => {
      older = auth().refreshProfile();
      newer = auth().refreshProfile();
    });
    await waitFor(() => expect(resolvers).toHaveLength(3));
    await act(async () => {
      resolvers[2](envelope(200, profile("newer")));
      await newer;
      resolvers[1](envelope(200, profile("older")));
      await older;
    });

    expect(screen.getByText("newer")).toBeTruthy();
  });

  it("does not restore the profile when logout happens during a save", async () => {
    let resolveSave!: (response: Response) => void;
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init?: RequestInit) =>
        init?.method === "PATCH"
          ? new Promise((resolve) => (resolveSave = resolve))
          : Promise.resolve(envelope(200, profile("initial"))),
      ),
    );
    const { auth, tokenStore } = setup();
    await screen.findByText("initial");

    let save!: Promise<unknown>;
    act(() => {
      save = auth().updateProfile({ phone: "0912345678" });
    });
    await waitFor(() => expect(resolveSave).toBeTypeOf("function"));
    act(() => {
      tokenStore.getToken.mockReturnValue(null);
      auth().logout();
    });
    await act(async () => {
      resolveSave(envelope(200, profile("saved")));
      await save;
    });

    expect(screen.getByText("none")).toBeTruthy();
  });

  it("shows the pending email even when the refresh after sending OTPs fails", async () => {
    let refreshFails = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, init?: RequestInit) => {
        if (init?.method === "POST") return envelope(200, { pending: true });
        if (refreshFails) throw new Error("offline");
        return envelope(200, profile("initial"));
      }),
    );
    const { auth } = setup();
    await screen.findByText("initial");
    refreshFails = true;

    await act(async () => {
      await auth().startEmailChange({
        newEmail: "new@example.com",
        currentPassword: "secret",
      });
    });

    expect(auth().user?.pendingEmail).toBe("new@example.com");
    expect(auth().user?.email).toBe("a@example.com");
  });

  it("clears the token after a successful password change", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => envelope(200, profile("initial"))),
    );
    const { auth, tokenStore } = setup();
    await screen.findByText("initial");

    await act(() =>
      auth().changePassword({ currentPassword: "a", newPassword: "b" }),
    );

    expect(tokenStore.clearToken).toHaveBeenCalled();
    expect(screen.getByText("none")).toBeTruthy();
  });
});
