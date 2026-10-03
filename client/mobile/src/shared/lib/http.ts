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

export interface ApiSuccessResponse<T> {
  status: number;
  message: string;
  data: T;
}

export interface ApiErrorDetail {
  code?: string;
  field?: string;
  detail: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;
  readonly title: string;
  readonly errors: ApiErrorDetail[];

  constructor(
    status: number,
    message: string,
    body: unknown = undefined,
    errors: ApiErrorDetail[] = [],
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
    this.title = message;
    this.errors = errors;
  }

  hasCode(code: string): boolean {
    return this.errors.some((error) => error.code === code);
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

function errorTitle(status: number, body: unknown): string {
  if (
    body !== null &&
    typeof body === "object" &&
    "title" in body &&
    typeof (body as { title?: unknown }).title === "string"
  ) {
    return (body as { title: string }).title;
  }

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

function errorDetails(body: unknown): ApiErrorDetail[] {
  if (
    body === null ||
    typeof body !== "object" ||
    !Array.isArray((body as { errors?: unknown }).errors)
  ) {
    return [];
  }

  return (body as { errors: unknown[] }).errors.filter(
    (error): error is ApiErrorDetail =>
      error !== null &&
      typeof error === "object" &&
      typeof (error as { detail?: unknown }).detail === "string",
  );
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

  try {
    if (signal?.aborted) {
      controller.abort();
    } else {
      signal?.addEventListener("abort", onExternalAbort, { once: true });
    }
    timeout = setTimeout(() => controller.abort(), timeoutMs);

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
      const title = errorTitle(response.status, data);
      throw new ApiError(response.status, title, data, errorDetails(data));
    }

    return data as T;
  } catch (error) {
    if (error instanceof ApiError) throw error;
    if (error instanceof DOMException && error.name === "AbortError") {
      if (signal?.aborted) {
        throw new ApiError(0, "Yêu cầu đã bị hủy.");
      }
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

export async function requestApi<T>(
  path: string,
  options: RequestJsonOptions = {},
): Promise<T> {
  const response = await requestJson<ApiSuccessResponse<T>>(path, options);
  if (
    !response ||
    typeof response.status !== "number" ||
    typeof response.message !== "string" ||
    !Object.prototype.hasOwnProperty.call(response, "data")
  ) {
    throw new ApiError(0, "Phản hồi API không hợp lệ.", response);
  }
  return response.data;
}
