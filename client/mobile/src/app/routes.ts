export const routes = {
  login: "#/login",
  register: "#/register",
  dashboard: "#/dashboard",
  transfer: "#/transfer",
  transactions: "#/transactions",
  loanRegister: "#/loan/register",
  loanApplications: "#/loan/applications",
  loanContracts: "#/loan/contracts",
  loanAdmin: "#/loan/admin",
} as const;

const LOAN_APPLICATION_PREFIX = `${routes.loanApplications}/`;
const LOAN_CONTRACT_PREFIX = `${routes.loanContracts}/`;
export const loanApplication = (id: string) =>
  `${LOAN_APPLICATION_PREFIX}${encodeURIComponent(id)}` as const;
export const loanContract = (id: string) =>
  `${LOAN_CONTRACT_PREFIX}${encodeURIComponent(id)}` as const;
function parseId(hash: string, prefix: string): string | null {
  if (!hash.startsWith(prefix)) return null;
  const id = hash.slice(prefix.length);
  if (!id) return null;
  try {
    return decodeURIComponent(id);
  } catch {
    return null;
  }
}
export const parseLoanApplicationId = (hash: string) =>
  parseId(hash, LOAN_APPLICATION_PREFIX);
export const parseLoanContractId = (hash: string) =>
  parseId(hash, LOAN_CONTRACT_PREFIX);

const TRANSACTION_DETAIL_PREFIX = `${routes.transactions}/`;

type TransactionDetailRoute = `${typeof routes.transactions}/${string}`;
export type AppRoute =
  | (typeof routes)[keyof typeof routes]
  | TransactionDetailRoute
  | ReturnType<typeof loanApplication>
  | ReturnType<typeof loanContract>;

export const transactionDetailRoute = (id: string): TransactionDetailRoute =>
  `${routes.transactions}/${encodeURIComponent(id)}`;

/** Trả về id giao dịch nếu hash là `#/transactions/:id`, ngược lại `null`. */
export function parseTransactionId(hash: string): string | null {
  if (!hash.startsWith(TRANSACTION_DETAIL_PREFIX)) return null;
  const id = hash.slice(TRANSACTION_DETAIL_PREFIX.length);
  if (!id) return null;
  try {
    return decodeURIComponent(id);
  } catch {
    // Hash bị sửa tay thành chuỗi percent-encoding lỗi, vd `%E0`
    return null;
  }
}

const PROTECTED_ROUTES: readonly string[] = [
  routes.dashboard,
  routes.transfer,
  routes.transactions,
  routes.loanRegister,
  routes.loanApplications,
  routes.loanContracts,
  routes.loanAdmin,
];

export const isProtectedRoute = (hash: string) =>
  PROTECTED_ROUTES.includes(hash) ||
  parseTransactionId(hash) !== null ||
  parseLoanApplicationId(hash) !== null ||
  parseLoanContractId(hash) !== null;

export function navigate(to: AppRoute) {
  window.location.hash = to;
}
