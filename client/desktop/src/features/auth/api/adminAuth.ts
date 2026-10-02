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
    public readonly code: "INVALID_CREDENTIALS" | "STAFF_LOCKED",
  ) {
    super(message);
    this.name = "AdminAuthError";
  }
}

const DEMO_CCCD_NUMBER = "001234567890";
const DEMO_PASSWORD = "admin123";

export async function loginAdmin(
  input: AdminLoginInput,
): Promise<AdminAuthResponse> {
  await new Promise((resolve) => window.setTimeout(resolve, 450));

  if (
    input.cccdNumber !== DEMO_CCCD_NUMBER ||
    input.password !== DEMO_PASSWORD
  ) {
    throw new AdminAuthError(
      "Thông tin đăng nhập không chính xác. Vui lòng thử lại.",
      "INVALID_CREDENTIALS",
    );
  }

  return {
    accessToken: "mock-admin-access-token",
    expiresAt: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
    staff: {
      id: "staff_001",
      employeeCode: "EMP-0001",
      fullName: "Tran Thi B",
      roles: ["risk_officer"],
      permissions: [
        "alert.read",
        "alert.triage",
        "alert.escalate",
        "audit.read",
        "report.read",
      ],
    },
  };
}
