/** REST contract taken from server/internal/presentation/http/handler/loan.
 * These endpoints return direct JSON, not the auth API's {data} envelope.
 */
import { API_BASE_URL } from "@/shared/constants";
import { ApiError, requestJson } from "@/shared/lib/http";

const root = "/api/loans";
const path = (id: string) => `${root}/${encodeURIComponent(id)}`;
const params = (values: Record<string, string | number | undefined>) => {
  const query = new URLSearchParams();
  Object.entries(values).forEach(([key, value]) => {
    if (value !== undefined && value !== "") query.set(key, String(value));
  });
  const suffix = query.toString();
  return suffix ? `?${suffix}` : "";
};
const get = <T>(url: string, token: string, signal?: AbortSignal) =>
  requestJson<T>(url, { token, signal });
const post = <T>(url: string, token: string, body?: unknown, key?: string) =>
  requestJson<T>(url, {
    method: "POST",
    token,
    ...(body === undefined ? {} : { body }),
    ...(key ? { headers: { "Idempotency-Key": key } } : {}),
  });

export const documentTypes = [
  "IDENTITY_FRONT",
  "IDENTITY_BACK",
  "VNEID_PERSONAL",
  "VNEID_RESIDENCE",
  "VSSID_PARTICIPATION",
  "VSSID_DETAIL",
  "SALARY_STATEMENT_3_MONTHS",
] as const;
export type DocumentType = (typeof documentTypes)[number];
export const documentLabels: Record<DocumentType, string> = {
  IDENTITY_FRONT: "CCCD mặt trước",
  IDENTITY_BACK: "CCCD mặt sau",
  VNEID_PERSONAL: "VNeID - thông tin cá nhân",
  VNEID_RESIDENCE: "VNeID - cư trú",
  VSSID_PARTICIPATION: "VssID - tham gia BHXH",
  VSSID_DETAIL: "VssID - chi tiết BHXH",
  SALARY_STATEMENT_3_MONTHS: "Sao kê lương 3 tháng",
};
export interface LoanPackage {
  packageCode: string;
  name: string;
  minAmount: number;
  maxAmount: number;
  termMonths: number[];
}
export interface LoanPreQuoteInput {
  loanPackageId: string;
  requestedAmount: number;
  termMonths: number;
  monthlyIncome: number;
  regionCode: "KV01" | "KV02" | "KV03";
  existingMonthlyDebt: number;
}
export interface LoanQuote {
  loanPackageId: string;
  requestedAmount: number;
  termMonths: number;
  annualRateBps: number;
  estimatedMonthlyPayment: number;
  totalInterest: number;
  totalRepayment: number;
  affordability: {
    withinIncomeLimit: boolean;
    dtiStatus: string;
    dtiBps: number;
    incomeLimit: number;
  };
  provisionalSchedule: Array<{
    installmentNo: number;
    dueDate: string;
    principalAmount: number;
    interestAmount: number;
    totalPayment: number;
  }>;
  official: boolean;
}
export interface LoanDocument {
  documentId: string;
  type: DocumentType;
  verificationStatus: string;
  uploadedAt: string;
  mimeType: string;
  sizeBytes: number;
}
export type ApplicationStatus =
  | "draft"
  | "submitted"
  | "under_review"
  | "approved"
  | "rejected"
  | "disbursed";
export interface LoanOffer {
  status: "issued" | "accepted" | "rejected" | "confirmed";
  amount: number;
  annualRateBps: number;
  termMonths: number;
  issuedAt: string;
  acceptedAt?: string;
  confirmedAt?: string;
}
export interface LoanApplication {
  id: string;
  applicationNo: string;
  loanPackageId: string;
  disbursementAccountId: string;
  requestedAmount: number;
  termMonths: number;
  status: ApplicationStatus;
  documents: LoanDocument[];
  assessmentDecision?: string;
  assessmentScore?: number;
  annualRateBps?: number;
  createdAt: string;
  offer?: LoanOffer;
}
export interface Page<T> {
  items: T[];
  nextCursor: string | null;
  eventualConsistency?: boolean;
}
export interface Installment {
  installmentNo: number;
  dueDate: string;
  principalDue: number;
  interestDue: number;
  totalDue: number;
  status: string;
}
export interface LoanContract {
  loanId: string;
  loanNo: string;
  applicationId: string;
  principal: number;
  outstandingPrincipal: number;
  annualInterestRate: string;
  termMonths: number;
  status: string;
  paidInstallments: number;
  totalInstallments: number;
  nextInstallmentNo: number;
  nextDueDate?: string;
  nextDueAmount: number;
}
export interface LoanContractDetail extends LoanContract {
  repaymentMethod: string;
  repaymentSchedule: Installment[];
}
export interface RepaymentSchedule {
  loanId: string;
  outstandingPrincipal: number;
  status: string;
  nextInstallmentNo: number;
  items: Installment[];
}
export interface RepaymentQuote {
  quoteId: string;
  loanId: string;
  installmentNo: number;
  dueDate: string;
  principalAmount: number;
  interestAmount: number;
  penaltyAmount: number;
  totalAmount: number;
  quoteExpiresAt: string;
  sourceAccount: { accountId: string; availableBalance: number };
}
export interface LoanPayment {
  paymentId: string;
  transactionId: string;
  loanId: string;
  installmentNo: number;
  amount: number;
  principalAmount: number;
  interestAmount: number;
  penaltyAmount: number;
  status: string;
  paidAt: string;
}
export interface RepaymentResult extends LoanPayment {
  outstandingPrincipalAfter: number;
  loanStatus: string;
}
export interface VerifiedEvidenceInput {
  monthlySalaries: [number, number, number];
  regionCode: string;
  existingMonthlyDebt: number;
  insuranceMonths: number;
  repaymentHistory: string;
  insuranceVerified: boolean;
  historyVerified: boolean;
  financialDataVerified: boolean;
  documentsVerified: boolean;
  sources: string[];
}

// Product and pre-application quote (PR #115)
export const listLoanPackages = (token: string, signal?: AbortSignal) =>
  get<{ items: LoanPackage[] }>(`${root}/packages`, token, signal);
export const preQuoteLoan = (token: string, input: LoanPreQuoteInput) =>
  post<LoanQuote>(`${root}/quotes`, token, input);

// Application CRUD, supporting documents and underwriting
export const createLoan = (
  token: string,
  input: {
    loanPackageId: string;
    requestedAmount: number;
    termMonths: number;
    disbursementAccountId: string;
  },
  idempotencyKey: string,
) => post<LoanApplication>(root, token, input, idempotencyKey);
export const listLoans = (token: string, cursor?: string, all = false) =>
  get<Page<LoanApplication>>(
    `${root}${params({ limit: 20, cursor, ...(all ? { all: "true" } : {}) })}`,
    token,
  );
export const getLoan = (token: string, id: string) =>
  get<LoanApplication>(path(id), token);
export const submitLoan = (token: string, id: string) =>
  post<LoanApplication>(`${path(id)}/submit`, token);
export const getApplicationQuote = (token: string, id: string) =>
  get<{
    annualRateBps: number;
    monthlyIncome: number;
    official: boolean;
    provisionalSchedule: unknown[];
  }>(`${path(id)}/quote`, token);
export const uploadLoanDocument = (
  token: string,
  id: string,
  type: DocumentType,
  file: File,
) => {
  const form = new FormData();
  form.append("type", type);
  form.append("file", file);
  return post<LoanDocument>(`${path(id)}/documents`, token, form);
};
/** Document download is intentionally authenticated and never exposes storageKey. */
export async function downloadLoanDocument(
  token: string,
  id: string,
  documentId: string,
): Promise<Blob> {
  const response = await fetch(
    `${API_BASE_URL.replace(/\/+$/, "")}${path(id)}/documents/${encodeURIComponent(documentId)}`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  if (!response.ok)
    throw new ApiError(
      response.status,
      `Không tải được chứng từ (HTTP ${response.status}).`,
    );
  return response.blob();
}

// Offer and contract OTP (PR #116)
export const getLoanOffer = (token: string, id: string) =>
  get<LoanOffer>(`${path(id)}/offer`, token);
export const acceptLoanOffer = (token: string, id: string) =>
  post<LoanOffer>(`${path(id)}/offer/accept`, token);
export const rejectLoanOffer = (token: string, id: string) =>
  post<LoanOffer>(`${path(id)}/offer/reject`, token);
export const requestLoanOTP = (token: string, id: string) =>
  post<{ expiresAt?: string }>(`${path(id)}/contract/otp`, token);
export const confirmLoanContract = (token: string, id: string, otp: string) =>
  post<LoanOffer>(`${path(id)}/contract/confirm`, token, {
    otp,
    acceptTerms: true,
  });

// Read-side contract after disbursement (PR #117)
export const listLoanContracts = (token: string, cursor?: string) =>
  get<Page<LoanContract>>(
    `${root}/contracts${params({ limit: 20, cursor })}`,
    token,
  );
export const getLoanContract = (token: string, loanId: string) =>
  get<LoanContractDetail>(
    `${root}/contracts/detail/${encodeURIComponent(loanId)}`,
    token,
  );

// Repayment; backend enforces due date, installment order and ownership.
export const getRepaymentSchedule = (token: string, id: string) =>
  get<RepaymentSchedule>(`${path(id)}/repayment-schedule`, token);
export const quoteRepayment = (
  token: string,
  id: string,
  installmentNo: number,
  sourceAccountId: string,
) =>
  post<RepaymentQuote>(`${path(id)}/repayments/quote`, token, {
    installmentNo,
    sourceAccountId,
  });
export const confirmRepayment = (
  token: string,
  id: string,
  quoteId: string,
  sourceAccountId: string,
  idempotencyKey: string,
) =>
  post<RepaymentResult>(
    `${path(id)}/repayments`,
    token,
    { quoteId, sourceAccountId },
    idempotencyKey,
  );
export const listLoanPayments = (token: string, id: string, cursor?: string) =>
  get<Page<LoanPayment>>(
    `${path(id)}/payments${params({ limit: 20, cursor })}`,
    token,
  );
export const getLoanPayment = (token: string, id: string, paymentId: string) =>
  get<LoanPayment>(
    `${path(id)}/payments/${encodeURIComponent(paymentId)}`,
    token,
  );

// Admin-only endpoints. The backend is responsible for RBAC, not the mobile UI.
export const startLoanReview = (token: string, id: string) =>
  post<LoanApplication>(`${path(id)}/review`, token);
export const attestLoanEvidence = (
  token: string,
  id: string,
  input: VerifiedEvidenceInput,
) => post<unknown>(`${path(id)}/evidence`, token, input);
export const assessLoan = (token: string, id: string) =>
  post<LoanApplication>(`${path(id)}/assess`, token);
export const decideLoan = (
  token: string,
  id: string,
  decision: "approved" | "rejected",
  reason: string,
) => post<LoanApplication>(`${path(id)}/decision`, token, { decision, reason });
export const issueLoanOffer = (token: string, id: string) =>
  post<LoanOffer>(`${path(id)}/offer/issue`, token);
export const disburseLoan = (token: string, id: string) =>
  post<LoanApplication>(`${path(id)}/disburse`, token);
