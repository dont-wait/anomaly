import { API_BASE_URL, saveAdminSession } from "./adminSession";

export interface AdminStaff {
  id: string;
  employeeCode: string;
  fullName: string;
  roles: string[];
  permissions: string[];
}

export interface AdminAuthResponse {
  accessToken: string;
  expiresAt: string;
  staff: AdminStaff;
}

export interface AdminLoginInput {
  cccdNumber: string;
  password: string;
}

export class AdminAuthError extends Error {
  constructor(
    message: string,
    public readonly code:
      | "BAD_REQUEST"
      | "INVALID_CREDENTIALS"
      | "STAFF_LOCKED"
      | "FORBIDDEN"
      | "SERVER_ERROR"
      | "NETWORK_ERROR"
      | "INVALID_RESPONSE",
  ) {
    super(message);
    this.name = "AdminAuthError";
  }
}

interface ApiSuccess<T> {
  data: T;
}

interface ApiError {
  errors?: Array<{ code?: string; detail?: string }>;
}

interface AccountAuthData {
  token: string;
  expiresAt: string;
  user: {
    id: string;
    accountNo: string;
    fullName: string;
    role: string;
  };
}

function isAccountAuthData(value: unknown): value is AccountAuthData {
  if (!value || typeof value !== "object") return false;
  const data = value as Partial<AccountAuthData>;
  const user = data.user as Partial<AccountAuthData["user"]> | undefined;
  return (
    typeof data.token === "string" &&
    data.token !== "" &&
    typeof data.expiresAt === "string" &&
    Number.isFinite(Date.parse(data.expiresAt)) &&
    Date.parse(data.expiresAt) > Date.now() &&
    !!user &&
    typeof user.id === "string" &&
    user.id !== "" &&
    typeof user.accountNo === "string" &&
    user.accountNo !== "" &&
    typeof user.fullName === "string" &&
    user.fullName !== "" &&
    typeof user.role === "string"
  );
}

export async function loginAdmin(
  input: AdminLoginInput,
): Promise<AdminAuthResponse> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
  } catch {
    throw new AdminAuthError(
      "Không thể kết nối đến máy chủ. Vui lòng thử lại.",
      "NETWORK_ERROR",
    );
  }

  const body = (await response.json().catch(() => null)) as
    ApiSuccess<AccountAuthData> | ApiError | null;
  if (!response.ok) {
    const error = body as ApiError | null;
    const detail = error?.errors?.[0]?.detail;
    const code = error?.errors?.[0]?.code;
    if (response.status === 401) {
      throw new AdminAuthError(
        "Thông tin đăng nhập không chính xác. Vui lòng thử lại.",
        "INVALID_CREDENTIALS",
      );
    }
    if (response.status === 403 || code === "FORBIDDEN") {
      throw new AdminAuthError(
        "Tài khoản không có quyền truy cập khu vực quản trị.",
        "FORBIDDEN",
      );
    }
    if (response.status >= 500) {
      throw new AdminAuthError(
        "Máy chủ đang gặp sự cố. Vui lòng thử lại sau.",
        "SERVER_ERROR",
      );
    }
    throw new AdminAuthError(
      detail || "Yêu cầu đăng nhập không hợp lệ.",
      "BAD_REQUEST",
    );
  }

  const data =
    body && typeof body === "object" && "data" in body
      ? (body as ApiSuccess<AccountAuthData>).data
      : undefined;
  if (!isAccountAuthData(data)) {
    throw new AdminAuthError(
      "Phản hồi từ máy chủ không hợp lệ.",
      "INVALID_RESPONSE",
    );
  }
  if (data.user.role !== "admin") {
    throw new AdminAuthError(
      "Tài khoản không có quyền truy cập khu vực quản trị.",
      "FORBIDDEN",
    );
  }

  const result: AdminAuthResponse = {
    accessToken: data.token,
    expiresAt: data.expiresAt,
    staff: {
      id: data.user.id,
      employeeCode: data.user.accountNo,
      fullName: data.user.fullName,
      roles: [data.user.role],
      permissions: [],
    },
  };
  saveAdminSession({
    accessToken: result.accessToken,
    expiresAt: result.expiresAt,
  });
  return result;
}
