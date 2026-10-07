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
  kyc: "#/admin/kyc",
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
  routes.kyc,
  routes.reportsTransactions,
  routes.reportsRisk,
  routes.reportsPortfolio,
  routes.customers,
  routes.staff,
  routes.permissions,
] as const;

export type ProtectedRoute = (typeof protectedRoutes)[number];

export function isProtectedRoute(value: string): value is ProtectedRoute {
  return (protectedRoutes as readonly string[]).includes(value);
}

export function navigate(to: string) {
  window.location.hash = to;
}
