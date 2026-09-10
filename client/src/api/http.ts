import { API_BASE_URL } from "@/constants";

export type HttpMethod = "GET" | "POST";

export interface RequestJsonOptions {
    method?: HttpMethod;
    body?: unknown;
    token?: string;
    signal?: AbortSignal;
    timeoutMs?: number;
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

function joinUrl(path: string): string {
    const base = API_BASE_URL.replace(/\/+$/, "");
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

        const response = await fetch(joinUrl(path), {
            method,
            headers: {
                ...(body === undefined
                    ? {}
                    : { "Content-Type": "application/json" }),
                ...(token ? { Authorization: `Bearer ${token}` } : {}),
            },
            body: body === undefined ? undefined : JSON.stringify(body),
            signal: controller.signal,
        });
        const data = await parseJsonSafe(response);

        if (!response.ok) {
            throw new ApiError(response.status, errorMessage(response.status, data), data);
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
