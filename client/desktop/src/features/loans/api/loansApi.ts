import {
  API_BASE_URL,
  readAdminSession,
} from "@/features/auth/api/adminSession";
import type {
  LoanCustomer,
  LoanApplication,
  LoanOffer,
  LoanPage,
  VerifiedEvidenceInput,
} from "../model";

const ROOT = "/api/loans";
const PATH = (id: string) => `${ROOT}/${encodeURIComponent(id)}`;
const TIMEOUT_MS = 15_000;

export class LoansApiError extends Error {
  readonly status: number;
  readonly codes: string[];

  constructor(message: string, status = 0, codes: string[] = []) {
    super(message);
    this.name = "LoansApiError";
    this.status = status;
    this.codes = codes;
  }

  hasCode(code: string): boolean {
    return this.codes.includes(code);
  }
}

interface ApiErrorBody {
  code?: unknown;
  title?: unknown;
  error?: unknown;
  errors?: unknown;
}

function errorCodes(body: unknown): string[] {
  if (!body || typeof body !== "object") return [];
  const record = body as ApiErrorBody;
  const codes: string[] = [];
  if (typeof record.code === "string" && record.code) codes.push(record.code);
  if (Array.isArray(record.errors)) {
    for (const item of record.errors) {
      if (
        item &&
        typeof item === "object" &&
        typeof (item as { code?: unknown }).code === "string"
      ) {
        codes.push((item as { code: string }).code);
      }
    }
  }
  return codes;
}

function errorMessage(status: number, body: unknown): string {
  if (body && typeof body === "object") {
    const record = body as ApiErrorBody;
    if (typeof record.title === "string" && record.title) return record.title;
    if (typeof record.error === "string" && record.error) return record.error;
    if (Array.isArray(record.errors)) {
      const detail = record.errors.find(
        (item) =>
          item &&
          typeof item === "object" &&
          typeof (item as { detail?: unknown }).detail === "string" &&
          (item as { detail: string }).detail,
      );
      if (detail) return (detail as { detail: string }).detail;
    }
  }
  return `Yêu cầu thất bại (mã ${status}).`;
}

export async function request<T>(
  path: string,
  init: RequestInit = {},
  timeoutMs = TIMEOUT_MS,
): Promise<T> {
  const session = readAdminSession();
  if (!session) {
    throw new LoansApiError("Phiên đăng nhập admin đã hết hạn.", 401);
  }

  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(`${API_BASE_URL.replace(/\/+$/, "")}${path}`, {
      ...init,
      headers: {
        ...(init.body === undefined
          ? {}
          : { "Content-Type": "application/json" }),
        Authorization: `Bearer ${session.accessToken}`,
        ...init.headers,
      },
      signal: controller.signal,
    });
    const body: unknown = await response.json().catch(() => undefined);
    if (!response.ok) {
      throw new LoansApiError(
        errorMessage(response.status, body),
        response.status,
        errorCodes(body),
      );
    }
    return body as T;
  } catch (error) {
    if (error instanceof LoansApiError) throw error;
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new LoansApiError("Yêu cầu quá lâu. Vui lòng thử lại.", 0);
    }
    throw new LoansApiError(
      "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.",
      0,
    );
  } finally {
    window.clearTimeout(timer);
  }
}

export async function getLoanCustomer(id: string): Promise<LoanCustomer> {
  const body = await request<{ data: LoanCustomer }>(
    `/api/accounts/${encodeURIComponent(id)}`,
  );
  return body.data;
}

export async function downloadLoanDocument(
  applicationId: string,
  documentId: string,
): Promise<Blob> {
  const session = readAdminSession();
  if (!session) {
    throw new LoansApiError("Phiên đăng nhập admin đã hết hạn.", 401);
  }
  let response: Response;
  try {
    response = await fetch(
      `${API_BASE_URL.replace(/\/+$/, "")}${PATH(applicationId)}/documents/${encodeURIComponent(documentId)}`,
      { headers: { Authorization: `Bearer ${session.accessToken}` } },
    );
  } catch {
    throw new LoansApiError(
      "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.",
      0,
    );
  }
  if (!response.ok) {
    throw new LoansApiError("Không tải được chứng từ.", response.status);
  }
  return response.blob();
}

export function listLoans(cursor?: string): Promise<LoanPage<LoanApplication>> {
  const query = new URLSearchParams({ limit: "20", all: "true" });
  if (cursor) query.set("cursor", cursor);
  return request<LoanPage<LoanApplication>>(`${ROOT}?${query.toString()}`);
}

export function getLoan(id: string): Promise<LoanApplication> {
  return request<LoanApplication>(PATH(id));
}

export function reviewLoanDocument(
  applicationId: string,
  documentId: string,
  decision: "verified" | "rejected",
  reason = "",
): Promise<LoanApplication> {
  return request<LoanApplication>(
    `${PATH(applicationId)}/documents/${encodeURIComponent(documentId)}/verification`,
    { method: "POST", body: JSON.stringify({ decision, reason }) },
  );
}

export function startLoanReview(id: string): Promise<LoanApplication> {
  return request<LoanApplication>(`${PATH(id)}/review`, { method: "POST" });
}

export function attestLoanEvidence(
  id: string,
  input: VerifiedEvidenceInput,
): Promise<unknown> {
  return request<unknown>(`${PATH(id)}/evidence`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function assessLoan(id: string): Promise<LoanApplication> {
  return request<LoanApplication>(`${PATH(id)}/assess`, { method: "POST" });
}

export function decideLoan(
  id: string,
  decision: "approved" | "rejected",
  reason: string,
): Promise<LoanApplication> {
  return request<LoanApplication>(`${PATH(id)}/decision`, {
    method: "POST",
    body: JSON.stringify({ decision, reason }),
  });
}

export function issueLoanOffer(id: string): Promise<LoanOffer> {
  return request<LoanOffer>(`${PATH(id)}/offer/issue`, { method: "POST" });
}

export function disburseLoan(id: string): Promise<LoanApplication> {
  return request<LoanApplication>(`${PATH(id)}/disburse`, { method: "POST" });
}
