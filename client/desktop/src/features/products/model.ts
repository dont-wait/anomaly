export type PackageKind = "loan" | "savings";
export type PackageStatus = "DRAFT" | "ACTIVE" | "SUPERSEDED" | "RETIRED";

export interface TermOption {
  months: number;
  annualRateBps?: number;
}

export interface PackageVersion {
  kind: PackageKind;
  code: string;
  version: number;
  status: PackageStatus;
  name: string;
  description: string;
  minAmount: number;
  maxAmount?: number;
  terms: TermOption[];
  createdBy?: string;
  publishedBy?: string;
  publishedAt?: string;
  retiredBy?: string;
  retiredAt?: string;
  retireReason?: string;
}

export interface PackageSummary {
  kind: PackageKind;
  code: string;
  name: string;
  status: PackageStatus;
  activeVersion?: number;
  draftVersion?: number;
  minAmount: number;
  maxAmount?: number;
  terms: TermOption[];
}

export interface AuditEntry {
  action: string;
  version: string;
  actorId: string;
  at: string;
  reason?: string;
}

export interface PackageDetail extends PackageSummary {
  versions: PackageVersion[];
  usage: Record<string, number>;
  audit: AuditEntry[];
}

export interface DraftInput {
  name: string;
  description: string;
  minAmount: number;
  maxAmount: number;
  terms: TermOption[];
}

export interface CreatePackageInput extends DraftInput {
  code: string;
}

export interface TermsSection {
  title: string;
  body: string;
}

export interface TermsDocument {
  version: string;
  status: string;
  title: string;
  introduction: string;
  sections: TermsSection[];
}

export interface LoanSimulation {
  annualRateBps: number;
  monthlyPayment: number;
  totalInterest: number;
  totalPayment: number;
}

export interface SavingsSimulation {
  interest: number;
  total: number;
  maturityAt: string;
}

export interface Page<T> {
  items: T[];
  nextCursor: string | null;
}

export interface SavingsContractRow {
  savingsId: string;
  savingsNo: string;
  customerId: string;
  sourceAccountId: string;
  packageId: string;
  packageVersion: number;
  principal: number;
  termMonths: number;
  annualInterestRate: string;
  openedAt: string;
  maturityAt: string;
  status: string;
  settlement?: {
    transactionId: string;
    interestAmount: number;
    totalAmount: number;
    settledAt: string;
  } | null;
}

export interface SavingsHistoryRow {
  transactionId: string;
  direction: string;
  amount: number;
  createdAt: string;
}

export interface SavingsContractDetail extends SavingsContractRow {
  history: SavingsHistoryRow[];
}

export interface LoanContractRow {
  loanId: string;
  loanNo: string;
  customerId: string;
  loanPackageId: string;
  loanPackageVersion: number;
  principal: number;
  outstandingPrincipal: number;
  annualInterestRate: string;
  termMonths: number;
  status: string;
  paidInstallments: number;
  totalInstallments: number;
  nextDueDate?: string;
  nextDueAmount: number;
  overdueInstallments: number;
  oldestOverdueDate?: string;
}

export interface LoanInstallmentRow {
  installmentNo: number;
  dueDate: string;
  principalDue: number;
  interestDue: number;
  totalDue: number;
  status: string;
}

export interface LoanPaymentRow {
  paymentId: string;
  installmentNo: number;
  amount: number;
  status: string;
  paidAt: string;
}

export interface LoanContractDetail extends Omit<
  LoanContractRow,
  "overdueInstallments"
> {
  repaymentSchedule: LoanInstallmentRow[];
  payments: LoanPaymentRow[];
}
