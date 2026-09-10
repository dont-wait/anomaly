import { ApiError, requestJson } from "./http";

export interface LoginInput {
    cccdNumber: string;
    password: string;
}

export interface LoginCredentials {
    cccdNumber: string;
    password: string;
}

export interface AuthUser {
    id: string;
    username: string;
    email: string;
    idCardFrontUrl: string;
    idCardBackUrl: string;
    liveVideoUrl: string;
    isVerify: boolean;
    amount: number;
}

export interface LoginResponse {
    token: string;
    expiresAt: string;
    user: AuthUser;
}

export const CCCD_LENGTH = 12;

export function normalizeCccd(value: string): string {
    return value.replace(/\s+/g, "");
}

export function isValidCccd(value: string): boolean {
    return new RegExp(`^\\d{${CCCD_LENGTH}}$`).test(normalizeCccd(value));
}

export function assertValidLoginInput(input: LoginInput): LoginCredentials {
    const cccdNumber = normalizeCccd(input.cccdNumber ?? "");
    if (!isValidCccd(cccdNumber)) {
        throw new Error("Số CCCD phải gồm đúng 12 chữ số.");
    }
    if (!input.password) {
        throw new Error("Vui lòng nhập mật khẩu.");
    }
    return { cccdNumber, password: input.password };
}

export async function login(
    input: LoginInput,
    options: { signal?: AbortSignal } = {},
): Promise<LoginResponse> {
    const credentials = assertValidLoginInput(input);
    const data = await requestJson<LoginResponse>("/api/auth/login", {
        method: "POST",
        body: credentials,
        signal: options.signal,
    });

    if (!data || typeof data.token !== "string" || !data.user) {
        throw new ApiError(0, "Phản hồi đăng nhập không hợp lệ.", data);
    }
    return data;
}

export async function getCurrentUser(
    token: string,
    options: { signal?: AbortSignal } = {},
): Promise<AuthUser> {
    if (!token) {
        throw new Error("Thiếu token xác thực.");
    }
    return requestJson<AuthUser>("/api/auth/me", {
        token,
        signal: options.signal,
    });
}

export function toLoginError(error: unknown): string {
    if (error instanceof ApiError) {
        switch (error.status) {
            case 0:
                return "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.";
            case 400:
                return "Thông tin đăng nhập không hợp lệ.";
            case 401:
            case 404:
                return "Số CCCD hoặc mật khẩu không đúng.";
            case 413:
                return "Dữ liệu gửi đi quá lớn. Vui lòng thử lại.";
            default:
                if (error.status >= 500) {
                    return "Máy chủ đang bận. Vui lòng thử lại sau.";
                }
                return `Đăng nhập thất bại (mã ${error.status}).`;
        }
    }
    if (error instanceof Error && error.message) return error.message;
    return "Đã xảy ra lỗi không xác định. Vui lòng thử lại.";
}
