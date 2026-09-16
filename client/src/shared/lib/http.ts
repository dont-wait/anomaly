import { API_BASE_URL } from "@/shared/constants";

export type HttpMethod = "GET" | "POST";

export interface RequestJsonOptions {
  method?: HttpMethod;
  body?: unknown;
  token?: string;
  signal?: AbortSignal;
  timeoutMs?: number;
  baseUrl?: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(status: number, message: string, body: unknown = undefined) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

const DEFAULT_TIMEOUT_MS = 15000;

function joinUrl(path: string, baseUrl = API_BASE_URL): string {
  const base = baseUrl.replace(/\/+$/, "");
  const suffix = path.startsWith("/") ? path : `/${path}`;
  return `${base}${suffix}`;
}

async function parseJsonSafe(response: Response): Promise<unknown> {
  try {
    const text = await response.text();
    if (!text) return undefined;
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

function errorMessage(status: number, body: unknown): string {
  if (
    body !== null &&
    typeof body === "object" &&
    "error" in body &&
    typeof (body as { error?: unknown }).error === "string"
  ) {
    return (body as { error: string }).error;
  }

  return `Yêu cầu thất bại (mã ${status}).`;
}

export async function requestJson<T>(
  path: string,
  options: RequestJsonOptions = {},
): Promise<T> {
  const {
    method = "GET",
    body,
    token,
    signal,
    timeoutMs = DEFAULT_TIMEOUT_MS,
    baseUrl = API_BASE_URL,
  } = options;
  const controller = new AbortController();
  const onExternalAbort = () => controller.abort();
  let timeout: ReturnType<typeof setTimeout> | undefined;
  let timedOut = false;

  try {
    if (signal?.aborted) {
      controller.abort();
    } else {
      signal?.addEventListener("abort", onExternalAbort, { once: true });
    }
    timeout = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, timeoutMs);

    const response = await fetch(joinUrl(path, baseUrl), {
      method,
      headers: {
        ...(body === undefined || body instanceof FormData
          ? {}
          : { "Content-Type": "application/json" }),
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body:
        body === undefined
          ? undefined
          : body instanceof FormData
            ? body
            : JSON.stringify(body),
      signal: controller.signal,
    });
    const data = await parseJsonSafe(response);

    if (!response.ok) {
      throw new ApiError(
        response.status,
        errorMessage(response.status, data),
        data,
      );
    }

    return data as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (error instanceof DOMException && error.name === "AbortError") {
      // Bên ngoài chủ động hủy (component unmount, effect cleanup, user điều hướng đi...)
      // -> không phải lỗi thật, ném lại nguyên bản AbortError để caller nhận diện và bỏ qua.
      if (signal?.aborted) {
        throw error;
      }
      // Request tự abort do vượt timeoutMs -> đây mới là lỗi thật, báo cho user.
      if (timedOut) {
        throw new ApiError(0, "Yêu cầu quá thời gian chờ. Vui lòng thử lại.");
      }
      // Trường hợp còn lại (hiếm): abort không rõ nguyên nhân.
      throw new ApiError(
        0,
        "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.",
      );
    }
    throw new ApiError(
      0,
      "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.",
    );
  } finally {
    if (timeout !== undefined) clearTimeout(timeout);
    signal?.removeEventListener("abort", onExternalAbort);
  }
}
