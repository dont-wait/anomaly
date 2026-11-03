export const routes = {
  login: "#/admin/login",
  monitor: "#/admin/monitor",
  alerts: "#/admin/alerts",
  riskAccounts: "#/admin/risk-accounts",
  audit: "#/admin/audit/events",
  auditAccounts: "#/admin/audit/accounts",
  auditReplay: "#/admin/audit/replay",
  projectionFailures: "#/admin/audit/projections",
  loanPackages: "#/admin/loan-packages",
  loanApplications: "#/admin/loan-applications",
  overdueLoans: "#/admin/loans/overdue",
  loanContracts: "#/admin/loans/contracts",
  savingsPackages: "#/admin/savings-packages",
  savingsContracts: "#/admin/savings/contracts",
  kyc: "#/admin/kyc",
  profileChanges: "#/admin/profile-changes",
  reportsTransactions: "#/admin/reports/transactions",
  reportsRisk: "#/admin/reports/risk",
  reportsPortfolio: "#/admin/reports/portfolio",
  customers: "#/admin/customers",
  staff: "#/admin/staff",
  permissions: "#/admin/permissions",
} as const;

export const protectedRoutes = [
  routes.monitor,
  routes.alerts,
  routes.riskAccounts,
  routes.audit,
  routes.auditAccounts,
  routes.auditReplay,
  routes.projectionFailures,
  routes.loanPackages,
  routes.loanApplications,
  routes.overdueLoans,
  routes.loanContracts,
  routes.savingsPackages,
  routes.savingsContracts,
  routes.kyc,
  routes.profileChanges,
  routes.reportsTransactions,
  routes.reportsRisk,
  routes.reportsPortfolio,
  routes.customers,
  routes.staff,
  routes.permissions,
] as const;

export type ProtectedRoute = (typeof protectedRoutes)[number];

export function protectedRouteFromHash(value: string): ProtectedRoute | null {
  const path = value.split("?", 1)[0];
  return (protectedRoutes as readonly string[]).includes(path)
    ? (path as ProtectedRoute)
    : null;
}

export function isProtectedRoute(value: string): boolean {
  return protectedRouteFromHash(value) !== null;
}

export function navigate(to: string) {
  window.location.hash = to;
}
