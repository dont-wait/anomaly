import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AuthContext,
  type AuthContextValue,
} from "@/features/auth/authContext";
import { ApiError } from "@/shared/lib/http";
import { ProfileSidebar } from "./ProfileSidebar";

const user = {
  id: "1",
  accountNo: "1234567890",
  username: "u",
  fullName: "Nguyễn Văn A",
  email: "a@example.com",
  currency: "VND",
  isVerify: true,
  amount: 0,
  phone: "+84901234567",
  address: { line: "12 Nguyen Trai", provinceCode: "HCM" },
  identity: { number: "079203001234" },
};

const slots = [
  {
    type: "IDENTITY_FRONT",
    required: true,
    replaceable: false,
    document: {
      mediaId: "m1",
      type: "IDENTITY_FRONT",
      fileName: "front.jpg",
      contentType: "image/jpeg",
      sizeBytes: 10,
      uploadedAt: "2026-11-01T10:00:00Z",
      status: "ACTIVE",
    },
  },
  { type: "INCOME_PROOF", required: false, replaceable: true, document: null },
];

function auth(overrides: Partial<AuthContextValue> = {}): AuthContextValue {
  return {
    status: "authenticated",
    user,
    token: "token",
    error: null,
    login: vi.fn(),
    logout: vi.fn(),
    refreshProfile: vi.fn(async () => user),
    updateProfile: vi.fn(async () => user),
    changePassword: vi.fn(async () => undefined),
    startEmailChange: vi.fn(async () => user),
    confirmEmailChange: vi.fn(async () => user),
    uploadProfileAvatar: vi.fn(async () => user),
    uploadProfileDocument: vi.fn(async () => user),
    ...overrides,
  } as AuthContextValue;
}

function renderSidebar(value: AuthContextValue) {
  return render(
    <AuthContext.Provider value={value}>
      <ProfileSidebar
        open
        name="Nguyễn Văn A"
        token="token"
        onClose={vi.fn()}
        onLogout={vi.fn()}
      />
    </AuthContext.Provider>,
  );
}

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      status: 200,
      text: async () =>
        JSON.stringify({ status: 200, message: "ok", data: { slots } }),
    })),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ProfileSidebar", () => {
  it("masks account and identity numbers and lists backend slots", async () => {
    renderSidebar(auth());

    expect(screen.getByText("•••• 7890")).toBeTruthy();
    expect(screen.getByText("•••• 1234")).toBeTruthy();
    expect(screen.queryByText(/079203001234/)).toBeNull();
    expect(await screen.findByText("Chứng từ thu nhập")).toBeTruthy();
    expect(screen.queryByText("Chứng từ địa chỉ")).toBeNull();
  });

  it("shows a pending email separately from the active email", () => {
    renderSidebar(auth({ user: { ...user, pendingEmail: "new@example.com" } }));

    expect(screen.getByText("Email mới · chờ xác minh")).toBeTruthy();
    expect(screen.getByText("new@example.com")).toBeTruthy();
    expect(screen.getAllByText("a@example.com").length).toBeGreaterThan(0);
  });

  it("sends only changed nested address fields and no email", async () => {
    const value = auth();
    renderSidebar(value);
    fireEvent.click(screen.getByText("Cập nhật thông tin"));
    expect(screen.queryByLabelText("Email")).toBeNull();

    type("Tỉnh/thành", "HN");
    fireEvent.click(screen.getByText("Lưu thay đổi"));

    await screen.findByText("Đã cập nhật thông tin.");
    expect(value.updateProfile).toHaveBeenCalledWith({
      address: { provinceCode: "HN" },
    });
  });

  it("accepts a spaced phone number and sends it normalized", async () => {
    const value = auth();
    renderSidebar(value);
    fireEvent.click(screen.getByText("Cập nhật thông tin"));
    type("Số điện thoại", "0912 345 678");
    fireEvent.click(screen.getByText("Lưu thay đổi"));

    await screen.findByText("Đã cập nhật thông tin.");
    expect(value.updateProfile).toHaveBeenCalledWith({ phone: "0912345678" });
    expect(screen.queryByText("Lưu thay đổi")).toBeNull();
  });

  it("disables saving until something changes and masks verified identity", () => {
    renderSidebar(auth());
    fireEvent.click(screen.getByText("Cập nhật thông tin"));

    expect(screen.getByText<HTMLButtonElement>("Lưu thay đổi").disabled).toBe(
      true,
    );
    expect(screen.getByText("•••• 1234")).toBeTruthy();
    expect(screen.queryByLabelText("Họ và tên")).toBeNull();
  });

  it("reloads on conflict and keeps the customer's input", async () => {
    const value = auth({
      updateProfile: vi.fn(async () => {
        throw new ApiError(409, "conflict");
      }),
    });
    renderSidebar(value);
    fireEvent.click(screen.getByText("Cập nhật thông tin"));
    type("Tỉnh/thành", "HN");
    fireEvent.click(screen.getByText("Lưu thay đổi"));

    await screen.findByText(/vừa được thay đổi/);
    expect(value.refreshProfile).toHaveBeenCalled();
    expect(screen.getByLabelText<HTMLInputElement>("Tỉnh/thành").value).toBe(
      "HN",
    );
    expect(screen.queryByText("Đã cập nhật thông tin.")).toBeNull();
  });

  it("confirms a pending email with only the new address OTP", async () => {
    const value = auth({ user: { ...user, pendingEmail: "new@example.com" } });
    renderSidebar(value);
    fireEvent.click(screen.getByText("Cập nhật thông tin"));
    fireEvent.click(screen.getByText("Đổi email"));

    expect(screen.queryByLabelText("Mã gửi đến email hiện tại")).toBeNull();
    const submit = screen.getByText<HTMLButtonElement>("Xác minh email");
    expect(submit.disabled).toBe(true);
    type("Mã gửi đến email mới", "222222");
    fireEvent.click(submit);

    await waitFor(() =>
      expect(value.confirmEmailChange).toHaveBeenCalledWith({
        newEmailOtp: "222222",
      }),
    );
  });

  it("requires the current password to start an email change and clears it", async () => {
    const value = auth();
    renderSidebar(value);
    fireEvent.click(screen.getByText("Cập nhật thông tin"));
    fireEvent.click(screen.getByText("Đổi email"));

    const submit = screen.getByText<HTMLButtonElement>("Gửi mã xác minh");
    type("Email mới", "new@example.com");
    expect(submit.disabled).toBe(true);
    type("Mật khẩu hiện tại", "Current1!");
    fireEvent.click(submit);

    await waitFor(() =>
      expect(value.startEmailChange).toHaveBeenCalledWith({
        newEmail: "new@example.com",
        currentPassword: "Current1!",
      }),
    );
    await waitFor(() =>
      expect(
        screen.getByLabelText<HTMLInputElement>("Mật khẩu hiện tại").value,
      ).toBe(""),
    );
  });

  it("clears password fields after a rejected change without a success state", async () => {
    const value = auth({
      changePassword: vi.fn(async () => {
        throw new ApiError(400, "invalid");
      }),
    });
    renderSidebar(value);
    fireEvent.click(screen.getByText("Đổi mật khẩu"));
    type("Mật khẩu hiện tại", "Current1!");
    type("Mật khẩu mới", "Newpass1!");
    type("Xác nhận mật khẩu mới", "Newpass1!");
    fireEvent.click(screen.getByText("Cập nhật mật khẩu"));

    await screen.findByText(/Mật khẩu hiện tại không đúng/);
    expect(
      screen.getByLabelText<HTMLInputElement>("Mật khẩu hiện tại").value,
    ).toBe("");
    expect(value.logout).not.toHaveBeenCalled();
  });

  it("offers separate camera and file sources", () => {
    renderSidebar(auth());
    fireEvent.click(screen.getByRole("button", { name: "Đổi ảnh đại diện" }));

    const camera = screen.getByLabelText<HTMLInputElement>("Mở camera");
    const picker = screen.getByLabelText<HTMLInputElement>("Chọn tệp");
    expect(camera.getAttribute("capture")).toBe("user");
    expect(camera.getAttribute("accept")).toBe("image/*");
    expect(picker.hasAttribute("capture")).toBe(false);
  });

  it("does not show upload progress while downloading a document", async () => {
    const value = auth();
    let finishMedia!: () => void;
    vi.stubGlobal(
      "fetch",
      vi.fn((url: string) =>
        url.includes("/media/")
          ? new Promise((_, reject) => {
              finishMedia = () => reject(new Error("offline"));
            })
          : Promise.resolve({
              ok: true,
              status: 200,
              text: async () =>
                JSON.stringify({ status: 200, message: "ok", data: { slots } }),
            }),
      ),
    );
    renderSidebar(value);
    fireEvent.click(screen.getByText("Quản lý"));
    fireEvent.click(await screen.findByText("Tải lên"));
    fireEvent.click(screen.getByText("Tải xuống"));

    await waitFor(() => expect(finishMedia).toBeTypeOf("function"));
    expect(screen.queryByText(/Đang tải lên/)).toBeNull();
    finishMedia();
    await screen.findByRole("alert");
  });

  it("rejects an oversized avatar before upload", () => {
    const value = auth();
    renderSidebar(value);
    fireEvent.click(screen.getByRole("button", { name: "Đổi ảnh đại diện" }));
    const big = new File([new Uint8Array(5 * 1024 * 1024 + 1)], "a.png", {
      type: "image/png",
    });
    fireEvent.change(screen.getByLabelText("Chọn tệp"), {
      target: { files: [big] },
    });

    expect(screen.getByRole("alert").textContent).toContain("5 MiB");
    expect(screen.queryByText("Lưu ảnh")).toBeNull();
  });
});
