import { LoansApiError, request } from "@/features/loans/api/loansApi";
import type {
  CreatePackageInput,
  DraftInput,
  LoanContractDetail,
  LoanContractRow,
  LoanSimulation,
  PackageDetail,
  PackageKind,
  PackageSummary,
  PackageVersion,
  Page,
  SavingsContractDetail,
  SavingsContractRow,
  SavingsSimulation,
  TermsDocument,
} from "./model";

const ADMIN = "/api/admin";
const root = (kind: PackageKind) =>
  `${ADMIN}/${kind === "loan" ? "loan-packages" : "savings-packages"}`;
const item = (kind: PackageKind, code: string) =>
  `${root(kind)}/${encodeURIComponent(code)}`;

async function data<T>(path: string, init?: RequestInit): Promise<T> {
  return (await request<{ data: T }>(path, init)).data;
}

const send = <T>(path: string, method: string, body?: unknown, key?: string) =>
  data<T>(path, {
    method,
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    ...(key ? { headers: { "Idempotency-Key": key } } : {}),
  });

export const listPackages = async (kind: PackageKind) =>
  (await data<{ items: PackageSummary[] }>(root(kind))).items;
export const getPackage = (kind: PackageKind, code: string) =>
  data<PackageDetail>(item(kind, code));
export const createPackage = (kind: PackageKind, input: CreatePackageInput) =>
  send<PackageVersion>(root(kind), "POST", input);
export const deletePackage = (kind: PackageKind, code: string) =>
  send<null>(item(kind, code), "DELETE");
export const createDraft = (kind: PackageKind, code: string) =>
  send<PackageVersion>(`${item(kind, code)}/draft`, "POST");
export const updateDraft = (
  kind: PackageKind,
  code: string,
  input: DraftInput,
) => send<PackageVersion>(`${item(kind, code)}/draft`, "PATCH", input);
export const deleteDraft = (kind: PackageKind, code: string) =>
  send<null>(`${item(kind, code)}/draft`, "DELETE");
export const publishPackage = (
  kind: PackageKind,
  code: string,
  expectedVersion: number,
  key: string,
) =>
  send<PackageVersion>(
    `${item(kind, code)}/publish`,
    "POST",
    { expectedVersion },
    key,
  );
export const retirePackage = (
  kind: PackageKind,
  code: string,
  expectedVersion: number,
  reason: string,
  key: string,
) =>
  send<PackageVersion>(
    `${item(kind, code)}/retire`,
    "POST",
    { expectedVersion, reason },
    key,
  );

export const simulateLoan = (input: {
  amount: number;
  monthlyIncome: number;
  regionCode: string;
  termMonths: number;
}) => send<LoanSimulation>(`${ADMIN}/loan-simulations`, "POST", input);
export const simulateSavings = (input: {
  amount: number;
  annualRateBps: number;
  termMonths: number;
}) => send<SavingsSimulation>(`${ADMIN}/savings-simulations`, "POST", input);

export const listTerms = async () =>
  (await data<{ items: TermsDocument[] }>(`${ADMIN}/savings-terms`)).items;
export const saveTermsDraft = (draft: TermsDocument) =>
  send<TermsDocument>(`${ADMIN}/savings-terms/draft`, "PUT", draft);
export const deleteTermsDraft = () =>
  send<null>(`${ADMIN}/savings-terms/draft`, "DELETE");
export const publishTerms = (version: string) =>
  send<TermsDocument>(`${ADMIN}/savings-terms/publish`, "POST", { version });

const withQuery = (path: string, query: Record<string, string | undefined>) => {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query))
    if (value) params.set(key, value);
  const text = params.toString();
  return text ? `${path}?${text}` : path;
};

export const listSavingsContracts = (query: {
  status?: string;
  customerId?: string;
  q?: string;
  cursor?: string;
}) =>
  data<Page<SavingsContractRow>>(
    withQuery(`${ADMIN}/savings/contracts`, query),
  );
export const getSavingsContract = (id: string) =>
  data<SavingsContractDetail>(
    `${ADMIN}/savings/contracts/${encodeURIComponent(id)}`,
  );
export const listLoanContracts = (query: {
  status?: string;
  customerId?: string;
  overdue?: string;
  cursor?: string;
}) => data<Page<LoanContractRow>>(withQuery(`${ADMIN}/loans/contracts`, query));
export const getLoanContract = (id: string) =>
  data<LoanContractDetail>(
    `${ADMIN}/loans/contracts/${encodeURIComponent(id)}`,
  );

const ERRORS: Record<string, string> = {
  PACKAGE_VERSION_CONFLICT:
    "Gói vừa được người khác thay đổi. Hãy tải lại rồi kiểm tra trước khi thử lại.",
  DRAFT_ALREADY_EXISTS: "Gói đã có bản nháp.",
  NO_DRAFT: "Gói không còn bản nháp.",
  INVALID_PACKAGE: "Thông tin gói chưa hợp lệ. Vui lòng kiểm tra lại.",
  PACKAGE_NOT_FOUND: "Không tìm thấy gói.",
  PACKAGE_CODE_EXISTS: "Mã gói này đã được dùng. Hãy chọn mã khác.",
  PACKAGE_IN_USE:
    "Gói đã có hồ sơ hoặc hợp đồng nên không xóa được, chỉ có thể ngừng bán.",
  PACKAGE_ACTIVE: "Gói đang bán. Hãy ngừng bán trước khi xóa.",
};

export function explainProductError(error: unknown): string {
  if (error instanceof LoansApiError) {
    const known = error.codes.find((code) => ERRORS[code]);
    if (known) return ERRORS[known];
    if (error.status === 401)
      return "Phiên đăng nhập admin đã hết hạn. Vui lòng đăng nhập lại.";
    if (error.status === 403)
      return "Tài khoản hiện tại không có quyền thực hiện thao tác này.";
    if (error.status === 404) return "Không tìm thấy dữ liệu.";
    if (error.status === 0) return error.message;
    if (error.status === 503) return "Dữ liệu đang đồng bộ, hãy thử lại sau.";
    return "Không thực hiện được thao tác. Vui lòng thử lại sau.";
  }
  return "Có lỗi xảy ra. Vui lòng thử lại.";
}
