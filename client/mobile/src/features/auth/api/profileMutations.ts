import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { ApiError, joinUrl, requestApi, toApiError } from "@/shared/lib/http";
import type { AuthUser, ProfileDocumentSlot, ProfileMedia } from "./profile";

const UPLOAD_TIMEOUT_MS = 120_000;
const NETWORK_ERROR =
  "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.";

export type UploadProgress = (percent: number) => void;

export interface ProfileUpdateInput {
  phone?: string;
  address?: { line?: string; provinceCode?: string };
}

export function updateCurrentProfile(token: string, input: ProfileUpdateInput) {
  return requestApi<AuthUser>(API_ENDPOINTS.AUTH.PROFILE, {
    method: "PATCH",
    body: input,
    token,
  });
}

export function changeCurrentPassword(
  token: string,
  input: { currentPassword: string; newPassword: string },
) {
  return requestApi<unknown>(API_ENDPOINTS.AUTH.PASSWORD, {
    method: "PUT",
    body: input,
    token,
  });
}

export function startEmailChange(token: string, newEmail: string) {
  return requestApi<unknown>(API_ENDPOINTS.AUTH.EMAIL_CHANGE, {
    method: "POST",
    body: { newEmail },
    token,
  });
}

export function confirmEmailChange(
  token: string,
  input: { currentEmailOtp: string; newEmailOtp: string },
) {
  return requestApi<unknown>(API_ENDPOINTS.AUTH.EMAIL_CHANGE_CONFIRM, {
    method: "POST",
    body: input,
    token,
  });
}

export function getDocumentSlots(
  token: string,
  options: { signal?: AbortSignal } = {},
) {
  return requestApi<{ slots: ProfileDocumentSlot[] }>(
    API_ENDPOINTS.AUTH.PROFILE_DOCUMENTS,
    { token, signal: options.signal },
  ).then((response) => response.slots);
}

export function uploadAvatar(
  token: string,
  file: File,
  onProgress?: UploadProgress,
) {
  return uploadFile(
    API_ENDPOINTS.AUTH.PROFILE_AVATAR,
    token,
    { file },
    onProgress,
  );
}

export function uploadDocument(
  token: string,
  type: string,
  file: File,
  onProgress?: UploadProgress,
) {
  return uploadFile(
    API_ENDPOINTS.AUTH.PROFILE_DOCUMENTS,
    token,
    { type, file },
    onProgress,
  );
}

/** Tải media qua backend. Thành công trả raw bytes, lỗi trả JSON envelope. */
export async function fetchProfileMedia(
  token: string,
  mediaId: string,
  options: { signal?: AbortSignal } = {},
): Promise<Blob> {
  const controller = new AbortController();
  const abort = () => controller.abort();
  const timeout = setTimeout(abort, UPLOAD_TIMEOUT_MS);
  if (options.signal?.aborted) abort();
  options.signal?.addEventListener("abort", abort, { once: true });
  try {
    const response = await fetch(
      joinUrl(API_ENDPOINTS.AUTH.PROFILE_MEDIA(mediaId)),
      {
        headers: { Authorization: `Bearer ${token}` },
        signal: controller.signal,
      },
    );
    if (!response.ok) {
      const body: unknown = await response.json().catch(() => undefined);
      throw toApiError(response.status, body);
    }
    // Timeout vẫn có hiệu lực trong lúc nhận phần thân của tệp.
    return await response.blob();
  } catch (cause) {
    if (cause instanceof ApiError) throw cause;
    throw new ApiError(0, NETWORK_ERROR);
  } finally {
    clearTimeout(timeout);
    options.signal?.removeEventListener("abort", abort);
  }
}

// Dùng XHR vì fetch không báo được tiến độ upload.
function uploadFile(
  path: string,
  token: string,
  fields: Record<string, string | File>,
  onProgress?: UploadProgress,
): Promise<ProfileMedia> {
  const form = new FormData();
  for (const [name, value] of Object.entries(fields)) form.append(name, value);

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", joinUrl(path));
    xhr.timeout = UPLOAD_TIMEOUT_MS;
    xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) {
        onProgress?.(Math.round((event.loaded / event.total) * 100));
      }
    };
    xhr.onerror = xhr.ontimeout = () => reject(new ApiError(0, NETWORK_ERROR));
    xhr.onload = () => {
      let body: unknown;
      try {
        body = JSON.parse(xhr.responseText);
      } catch {
        body = undefined;
      }
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(toApiError(xhr.status, body));
      } else if (body && typeof body === "object" && "data" in body) {
        resolve(body.data as ProfileMedia);
      } else {
        reject(new ApiError(0, "Phản hồi tải tệp không hợp lệ.", body));
      }
    };
    xhr.send(form);
  });
}
