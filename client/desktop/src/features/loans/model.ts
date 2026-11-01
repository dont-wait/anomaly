export type ApplicationStatus =
  | "draft"
  | "submitted"
  | "under_review"
  | "approved"
  | "rejected"
  | "disbursed";

export interface LoanDocument {
  documentId: string;
  type: string;
  verificationStatus: string;
  rejectionReason?: string;
  uploadedAt: string;
  mimeType: string;
  sizeBytes: number;
}

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
  customerId?: string;
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

export interface LoanPage<T> {
  items: T[];
  nextCursor: string | null;
  eventualConsistency?: boolean;
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

export interface LoanCustomer {
  id: string;
  accountNo: string;
  username: string;
  email: string;
  isVerify: boolean;
  amount: number;
}
