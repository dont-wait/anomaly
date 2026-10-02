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
      "INVALID_CREDENTIALS" | "STAFF_LOCKED" | "FORBIDDEN" | "NETWORK_ERROR",
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

const API_BASE_URL =
  import.meta.env.VITE_API_ENDPOINT || "http://localhost:8080";
const SESSION_STORAGE_KEY = "anomaly.admin.session";

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
    throw new AdminAuthError(
      code === "FORBIDDEN"
        ? "Tài khoản không có quyền truy cập khu vực quản trị."
        : detail || "Thông tin đăng nhập không chính xác. Vui lòng thử lại.",
      code === "FORBIDDEN" ? "FORBIDDEN" : "INVALID_CREDENTIALS",
    );
  }

  const data =
    body && typeof body === "object" && "data" in body
      ? (body as ApiSuccess<AccountAuthData>).data
      : undefined;
  if (!data?.token || !data.user || data.user.role !== "admin") {
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
  window.localStorage.setItem(
    SESSION_STORAGE_KEY,
    JSON.stringify({
      accessToken: result.accessToken,
      expiresAt: result.expiresAt,
    }),
  );
  return result;
}
