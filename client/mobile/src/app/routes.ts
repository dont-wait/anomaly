export const routes = {
  login: "#/login",
  register: "#/register",
  dashboard: "#/dashboard",
  transfer: "#/transfer",
  transactions: "#/transactions",
} as const;

const TRANSACTION_DETAIL_PREFIX = `${routes.transactions}/`;

type TransactionDetailRoute = `${typeof routes.transactions}/${string}`;
export type AppRoute =
  (typeof routes)[keyof typeof routes] | TransactionDetailRoute;

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
];

export const isProtectedRoute = (hash: string) =>
  PROTECTED_ROUTES.includes(hash) || parseTransactionId(hash) !== null;

export function navigate(to: AppRoute) {
  window.location.hash = to;
}
