import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { API_BASE_URL } from "@/shared/constants";
import { ApiError, requestApi } from "@/shared/lib/http";
import type { AuthUser, ProfileMedia } from "./profile";

const AUTH_HEADERS = (token: string) => ({ Authorization: `Bearer ${token}` });

export function updateCurrentProfile(
  token: string,
  input: { phone?: string; email?: string; address?: { line: string; provinceCode: string } },
) {
  return requestApi<AuthUser>(`${API_ENDPOINTS.AUTH.ME}/profile`, {
    method: "PATCH",
    body: input,
    token,
  });
}

export function changeCurrentPassword(
  token: string,
  input: { currentPassword: string; newPassword: string },
) {
  return requestApi<{ changed: boolean }>(`${API_ENDPOINTS.AUTH.ME}/password`, {
    method: "PUT",
    body: input,
    token,
  });
}

export async function uploadAvatar(token: string, file: File) {
  return uploadFile<ProfileMedia>(`${API_ENDPOINTS.AUTH.ME}/avatar`, token, { file });
}

export async function uploadDocument(token: string, type: string, file: File) {
  return uploadFile<ProfileMedia>(`${API_ENDPOINTS.AUTH.ME}/documents`, token, { type, file });
}

async function uploadFile<T>(path: string, token: string, fields: Record<string, string | File>): Promise<T> {
  const form = new FormData();
  for (const [name, value] of Object.entries(fields)) form.append(name, value);
  const response = await fetch(`${API_BASE_URL.replace(/\/+$/, "")}${path}`, {
    method: "POST",
    headers: AUTH_HEADERS(token),
    body: form,
  });
  let body: unknown;
  try { body = await response.json(); } catch { body = undefined; }
  if (!response.ok) {
    const message = body && typeof body === "object" && "message" in body && typeof body.message === "string"
      ? body.message
      : `Yêu cầu thất bại (mã ${response.status}).`;
    throw new ApiError(response.status, message, body);
  }
  if (!body || typeof body !== "object" || !("data" in body)) {
    throw new Error("Phản hồi tải tệp không hợp lệ.");
  }
  return body.data as T;
}
