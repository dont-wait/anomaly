import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCheck,
  faCircleExclamation,
  faDownload,
  faEye,
  faFileInvoiceDollar,
  faShieldHalved,
  faXmark,
} from "@fortawesome/free-solid-svg-icons";
import {
  assessLoan,
  attestLoanEvidence,
  decideLoan,
  disburseLoan,
  downloadLoanDocument,
  getLoan,
  getLoanCustomer,
  getLoanEvidence,
  issueLoanOffer,
  listLoans,
  LoansApiError,
  rejectLoanReview,
  reviewLoanDocument,
  startLoanReview,
} from "./api/loansApi";
import { AppliedPackage } from "@/features/products/AppliedPackage";
import {
  assessmentLabels,
  assessmentReasonLabel,
  documentLabels,
  documentStatusLabels,
  explainLoanError,
  offerLabels,
  statusLabels,
  vnd,
} from "./format";
import type {
  LoanApplication,
  LoanCustomer,
  LoanDocument,
  VerifiedEvidenceInput,
} from "./model";

const panel =
  "rounded-(--admin-radius-md) border border-(--admin-border) bg-(--admin-surface)";
const fieldLabel =
  "grid gap-1 text-[11px] font-semibold text-(--admin-text-muted)";
const fieldInput =
  "h-9 w-full rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none focus:border-(--admin-primary)";
const mono = "font-(family-name:--admin-font-mono)";

function Button({
  children,
  onClick,
  disabled = false,
  variant = "primary",
  ariaLabel,
}: {
  children: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  variant?: "primary" | "secondary" | "danger";
  ariaLabel?: string;
}) {
  const palette =
    variant === "primary"
      ? "bg-(--admin-primary) text-white hover:bg-(--admin-primary-strong)"
      : variant === "danger"
        ? "border border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger) hover:bg-(--admin-surface-hover)"
        : "border border-(--admin-border) bg-(--admin-surface) text-(--admin-text) hover:border-(--admin-primary)";
  return (
    <button
      className={`h-9 rounded-sm px-3 text-[12px] font-semibold disabled:cursor-not-allowed disabled:opacity-50 ${palette}`}
      type="button"
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function Notice({
  message,
  error = false,
}: {
  message: string;
  error?: boolean;
}) {
  return (
    <div
      className={`flex items-center gap-2 rounded-sm border px-3 py-2 text-[11px] ${
        error
          ? "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)"
          : "border-(--admin-border) bg-(--admin-surface-subtle) text-(--admin-text-muted)"
      }`}
      role={error ? "alert" : "status"}
    >
      {error && (
        <FontAwesomeIcon icon={faCircleExclamation} aria-hidden="true" />
      )}
      {message}
    </div>
  );
}

const offerExpired = (application: LoanApplication) => {
  const offer = application.offer;
  return (
    !!offer?.expiresAt &&
    (offer.status === "issued" || offer.status === "accepted") &&
    Date.now() >= Date.parse(offer.expiresAt)
  );
};

function statusClass(status: string): string {
  if (status === "approved" || status === "disbursed")
    return "border-(--admin-success) bg-(--admin-success-soft) text-(--admin-success)";
  if (status === "rejected")
    return "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)";
  if (status === "under_review")
    return "border-(--admin-warning) bg-(--admin-warning-soft) text-(--admin-warning)";
  return "border-(--admin-border) bg-(--admin-surface-hover) text-(--admin-text-secondary)";
}

type TabId = "review" | "documents" | "customer";

// Mặc định chỉ hiện hồ sơ còn việc phải làm.
const QUEUE_NEEDS_ACTION = "submitted,under_review,approved";
const QUEUE_FILTERS = [
  { value: QUEUE_NEEDS_ACTION, label: "Cần xử lý" },
  { value: "submitted", label: "Chờ duyệt" },
  { value: "under_review", label: "Đang thẩm định" },
  { value: "approved", label: "Đã duyệt" },
  { value: "disbursed", label: "Đã giải ngân" },
  { value: "rejected", label: "Từ chối" },
  { value: "draft", label: "Bản nháp của khách" },
  { value: "", label: "Tất cả" },
];

const TABS: Array<{ id: TabId; label: string }> = [
  { id: "review", label: "Thẩm định" },
  { id: "documents", label: "Chứng từ" },
  { id: "customer", label: "Khách hàng" },
];
// Ảnh hoặc PDF giấy tờ thật luôn lớn hơn nhiều so với ngưỡng này.
const NEAR_EMPTY_BYTES = 1024;
const DOCUMENT_TYPES = Object.keys(documentLabels);
const DOCUMENT_COUNT = DOCUMENT_TYPES.length;

const formatDate = (value?: string) => {
  const date = new Date(value ?? "");
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleDateString("vi-VN");
};

// Khách có thể nộp lại chứng từ bị từ chối; bản nộp sau thay thế bản trước.
const latestDocument = (documents: LoanDocument[], type: string) =>
  [...documents].reverse().find((doc) => doc.type === type);

function documentStatusClass(status: string): string {
  if (status === "verified")
    return "border-(--admin-success) bg-(--admin-success-soft) text-(--admin-success)";
  if (status === "rejected")
    return "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)";
  return "border-(--admin-warning) bg-(--admin-warning-soft) text-(--admin-warning)";
}

function DocumentActions({
  document: doc,
  label,
  busy,
  onDecide,
}: {
  document: LoanDocument;
  label: string;
  busy: boolean;
  onDecide: (decision: "verified" | "rejected", reason?: string) => void;
}) {
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");

  if (rejecting) {
    return (
      <div className="grid w-full gap-2">
        <label className={fieldLabel}>
          Lý do từ chối {label}
          <input
            className={fieldInput}
            maxLength={500}
            value={reason}
            autoFocus
            onChange={(event) => setReason(event.target.value)}
          />
        </label>
        <div className="flex gap-2">
          <Button
            variant="danger"
            disabled={busy || !reason.trim()}
            onClick={() => {
              onDecide("rejected", reason.trim());
              setRejecting(false);
              setReason("");
            }}
          >
            Xác nhận từ chối
          </Button>
          <Button variant="secondary" onClick={() => setRejecting(false)}>
            Hủy
          </Button>
        </div>
      </div>
    );
  }
  return (
    <div className="grid max-w-64 grid-cols-2 gap-2">
      <Button
        ariaLabel={`Duyệt ${label}`}
        disabled={busy || doc.verificationStatus === "verified"}
        onClick={() => onDecide("verified")}
      >
        <FontAwesomeIcon icon={faCheck} className="mr-1.5" aria-hidden="true" />
        Duyệt
      </Button>
      <Button
        ariaLabel={`Từ chối ${label}`}
        variant="danger"
        disabled={busy || doc.verificationStatus === "rejected"}
        onClick={() => setRejecting(true)}
      >
        <FontAwesomeIcon icon={faXmark} className="mr-1.5" aria-hidden="true" />
        Từ chối
      </Button>
    </div>
  );
}

function DocumentPreview({
  applicationId,
  document: doc,
  label,
  actions,
  onClose,
}: {
  applicationId: string;
  document: LoanDocument;
  label: string;
  actions: ReactNode;
  onClose: () => void;
}) {
  const [file, setFile] = useState<{
    id: string;
    url?: string;
    error?: string;
  }>();
  const [attempt, setAttempt] = useState(0);
  const dialog = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;
    let url: string | undefined;
    downloadLoanDocument(applicationId, doc.documentId)
      .then((blob) => {
        if (!active) return;
        // Dùng loại file đã lưu để trình duyệt chọn đúng trình xem.
        url = URL.createObjectURL(new Blob([blob], { type: doc.mimeType }));
        setFile({ id: doc.documentId, url });
      })
      .catch((error: unknown) => {
        if (active)
          setFile({ id: doc.documentId, error: explainLoanError(error) });
      });
    return () => {
      active = false;
      if (url) URL.revokeObjectURL(url);
    };
  }, [applicationId, doc.documentId, doc.mimeType, attempt]);

  useEffect(() => {
    dialog.current?.focus();
    const close = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [onClose]);

  const current = file?.id === doc.documentId ? file : undefined;
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/70 p-4">
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-label={`Xem ${label}`}
        tabIndex={-1}
        className={`${panel} flex max-h-full w-full max-w-4xl flex-col gap-3 p-4 outline-none`}
      >
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-[14px] font-semibold text-(--admin-text)">
            {label}
          </h3>
          <button
            type="button"
            aria-label="Đóng xem trước"
            className="grid h-8 w-8 place-items-center rounded-sm text-(--admin-text) hover:bg-(--admin-surface-hover)"
            onClick={onClose}
          >
            <FontAwesomeIcon icon={faXmark} aria-hidden="true" />
          </button>
        </div>
        <div className="grid min-h-80 flex-1 place-items-center overflow-auto rounded-sm bg-(--admin-background)">
          {!current && (
            <p role="status" className="text-[12px] text-(--admin-text-muted)">
              Đang tải chứng từ...
            </p>
          )}
          {current?.error && (
            <div className="grid justify-items-center gap-2">
              <Notice message={current.error} error />
              <Button
                variant="secondary"
                onClick={() => {
                  setFile(undefined);
                  setAttempt((value) => value + 1);
                }}
              >
                Thử lại
              </Button>
            </div>
          )}
          {current?.url &&
            (doc.mimeType === "application/pdf" ? (
              <iframe
                title={label}
                src={current.url}
                className="h-[70vh] w-full"
              />
            ) : (
              <img
                src={current.url}
                alt={label}
                onError={() =>
                  setFile({
                    id: doc.documentId,
                    error:
                      "Không hiển thị được ảnh này. Hãy tải về để kiểm tra.",
                  })
                }
                className="max-h-[70vh] max-w-full object-contain"
              />
            ))}
        </div>
        {doc.sizeBytes < NEAR_EMPTY_BYTES && (
          <Notice
            error
            message={`Tệp gần như không có nội dung (${doc.sizeBytes} byte), nên có thể hiển thị trống. Không nên duyệt chứng từ này.`}
          />
        )}
        {actions}
      </div>
    </div>
  );
}

function DocumentsTab({
  application,
  onError,
  onChanged,
}: {
  application: LoanApplication;
  onError: (message: string) => void;
  onChanged: (application: LoanApplication) => void;
}) {
  const documents = application.documents ?? [];
  const [previewId, setPreviewId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const canReview = application.status === "under_review";
  const closePreview = useCallback(() => setPreviewId(null), []);
  const verified = DOCUMENT_TYPES.filter(
    (type) =>
      latestDocument(documents, type)?.verificationStatus === "verified",
  ).length;
  const previewed = documents.find((doc) => doc.documentId === previewId);

  const decide = async (
    doc: LoanDocument,
    decision: "verified" | "rejected",
    reason = "",
  ) => {
    setSaving(true);
    onError("");
    try {
      onChanged(
        await reviewLoanDocument(
          application.id,
          doc.documentId,
          decision,
          reason,
        ),
      );
    } catch (error) {
      onError(explainLoanError(error));
    } finally {
      setSaving(false);
    }
  };
  const open = async (documentId: string, type: string) => {
    onError("");
    try {
      const url = URL.createObjectURL(
        await downloadLoanDocument(application.id, documentId),
      );
      const link = document.createElement("a");
      link.href = url;
      link.download = documentLabels[type] ?? "chung-tu";
      document.body.append(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (error) {
      onError(explainLoanError(error));
    }
  };

  return (
    <div className="grid gap-2">
      <p className="text-[12px] text-(--admin-text-muted)">
        Đã xác minh {verified}/{DOCUMENT_COUNT} chứng từ.
        {!canReview && " Chỉ duyệt được khi hồ sơ đang thẩm định."}
      </p>
      {Object.entries(documentLabels).map(([type, label]) => {
        const doc = latestDocument(documents, type);
        return (
          <div
            key={type}
            className="grid gap-2 rounded-sm border border-(--admin-border) bg-(--admin-background) px-3 py-2"
          >
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <p className="text-[12px] font-semibold text-(--admin-text)">
                  {label}
                </p>
                <p className="text-[11px] text-(--admin-text-muted)">
                  {doc ? `Gửi ${formatDate(doc.uploadedAt)}` : "Khách chưa gửi"}
                </p>
              </div>
              {doc && (
                <div className="flex shrink-0 items-center gap-2">
                  <span
                    className={`rounded-sm border px-1.5 py-0.5 text-[10px] ${documentStatusClass(doc.verificationStatus)}`}
                  >
                    {documentStatusLabels[doc.verificationStatus] ??
                      "Đang cập nhật"}
                  </span>
                  <button
                    type="button"
                    aria-label={`Xem ${label}`}
                    className="grid h-8 w-8 place-items-center rounded-sm border border-(--admin-border) text-(--admin-text) hover:border-(--admin-primary)"
                    onClick={() => setPreviewId(doc.documentId)}
                  >
                    <FontAwesomeIcon icon={faEye} aria-hidden="true" />
                  </button>
                  <button
                    type="button"
                    aria-label={`Tải xuống ${label}`}
                    className="grid h-8 w-8 place-items-center rounded-sm border border-(--admin-border) text-(--admin-text) hover:border-(--admin-primary)"
                    onClick={() => void open(doc.documentId, doc.type)}
                  >
                    <FontAwesomeIcon icon={faDownload} aria-hidden="true" />
                  </button>
                </div>
              )}
            </div>
            {doc?.rejectionReason && (
              <p className="text-[11px] text-(--admin-danger)">
                Lý do: {doc.rejectionReason}
              </p>
            )}
            {doc && (
              <DocumentActions
                document={doc}
                label={label}
                busy={saving || !canReview}
                onDecide={(decision, reason) =>
                  void decide(doc, decision, reason)
                }
              />
            )}
          </div>
        );
      })}
      {previewed && (
        <DocumentPreview
          applicationId={application.id}
          document={previewed}
          label={documentLabels[previewed.type] ?? "Chứng từ"}
          onClose={closePreview}
          actions={
            <DocumentActions
              document={previewed}
              label={documentLabels[previewed.type] ?? "chứng từ"}
              busy={saving || !canReview}
              onDecide={(decision, reason) =>
                void decide(previewed, decision, reason)
              }
            />
          }
        />
      )}
    </div>
  );
}

export function CustomerSummary({ accountId }: { accountId: string }) {
  const [result, setResult] = useState<{
    id: string;
    customer?: LoanCustomer;
    error?: string;
  } | null>(null);

  useEffect(() => {
    let active = true;
    getLoanCustomer(accountId)
      .then((customer) => {
        if (active) setResult({ id: accountId, customer });
      })
      .catch((error: unknown) => {
        if (active)
          setResult({ id: accountId, error: explainLoanError(error) });
      });
    return () => {
      active = false;
    };
  }, [accountId]);

  const current = result?.id === accountId ? result : null;
  const customer = current?.customer;

  if (!current) {
    return (
      <p role="status" className="text-[12px] text-(--admin-text-muted)">
        Đang tải thông tin khách hàng...
      </p>
    );
  }
  if (!customer) return <Notice message={current.error ?? ""} error />;
  return (
    <dl className="grid grid-cols-2 gap-3 text-[12px]">
      {[
        ["Khách hàng", customer.username],
        ["Email", customer.email],
        ["Tài khoản", `•••• ${customer.accountNo.slice(-4)}`],
        ["Số dư", vnd(customer.amount)],
        ["Định danh", customer.isVerify ? "Đã xác minh" : "Chưa xác minh"],
      ].map(([label, value]) => (
        <div key={label}>
          <dt className="text-[11px] text-(--admin-text-muted)">{label}</dt>
          <dd className="mt-0.5 font-semibold break-words text-(--admin-text)">
            {value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

function CustomerTab({
  application,
  all,
  onOpen,
}: {
  application: LoanApplication;
  all: LoanApplication[];
  onOpen: (application: LoanApplication) => void;
}) {
  const accountId = application.customerId ?? application.disbursementAccountId;
  const others = all.filter(
    (item) =>
      item.id !== application.id &&
      (item.customerId ?? item.disbursementAccountId) === accountId,
  );

  return (
    <div className="grid gap-3">
      <CustomerSummary accountId={accountId} />
      <div className="grid gap-1.5 border-t border-(--admin-border) pt-3">
        <h3 className="text-[12px] font-semibold text-(--admin-text)">
          Hồ sơ khác của khách ({others.length})
        </h3>
        {others.map((item) => (
          <button
            key={item.id}
            type="button"
            className="flex items-center justify-between gap-2 rounded-sm border border-(--admin-border) px-3 py-2 text-left text-[11px] text-(--admin-text) hover:border-(--admin-primary)"
            onClick={() => onOpen(item)}
          >
            <span>
              {vnd(item.requestedAmount)} · {item.termMonths} tháng
            </span>
            <span
              className={`rounded-sm border px-1.5 py-0.5 text-[10px] ${statusClass(item.status)}`}
            >
              {statusLabels[item.status] ?? "Đang cập nhật"}
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}

export function LoanReviewPage() {
  const [items, setItems] = useState<LoanApplication[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState("");
  const [detail, setDetail] = useState<LoanApplication | null>(null);
  const [salaries, setSalaries] = useState<[number, number, number]>([0, 0, 0]);
  const [region, setRegion] = useState("KV01");
  const [debt, setDebt] = useState(0);
  const [insurance, setInsurance] = useState(0);
  const [history, setHistory] = useState("");
  const [financialVerified, setFinancialVerified] = useState(false);
  const [insuranceVerified, setInsuranceVerified] = useState(false);
  const [historyVerified, setHistoryVerified] = useState(false);
  const [evidenceSaved, setEvidenceSaved] = useState(false);
  const [reason, setReason] = useState("");
  const selectedRef = useRef("");
  const [tab, setTab] = useState<TabId>("review");
  const [queueFilter, setQueueFilter] = useState(QUEUE_NEEDS_ACTION);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const run = async (action: () => Promise<unknown>, success: string) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
      setMessage(success);
      const id = selectedId.trim();
      if (id) {
        const fresh = await getLoan(id);
        if (selectedRef.current === id) setDetail(fresh);
        setItems((old) => old.map((item) => (item.id === id ? fresh : item)));
      }
    } catch (error) {
      setError(explainLoanError(error));
    } finally {
      setBusy(false);
    }
  };

  const loadList = async (append: boolean, status = queueFilter) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const page = await listLoans(
        append ? (cursor ?? undefined) : undefined,
        status,
      );
      setItems((old) => {
        if (!append) return page.items;
        const seen = new Set(old.map((item) => item.id));
        return [...old, ...page.items.filter((item) => !seen.has(item.id))];
      });
      setCursor(page.nextCursor);
    } catch (error) {
      setError(explainLoanError(error));
    } finally {
      setBusy(false);
    }
  };

  // Mở trang là thấy hồ sơ ngay, không cần bấm tải.
  const loadListRef = useRef(loadList);
  useEffect(() => {
    void loadListRef.current(false);
  }, []);

  // Điền sẵn số khách khai để đối chiếu; phần chưa khai thì admin phải tự nhập.
  const selectApplication = (item: LoanApplication) => {
    const income = item.declared?.monthlyIncome ?? 0;
    selectedRef.current = item.id;
    setSelectedId(item.id);
    setDetail(item);
    setTab("review");
    setError("");
    setMessage("");
    setSalaries([income, income, income]);
    setDebt(item.declared?.existingMonthlyDebt ?? 0);
    setRegion(item.declared?.regionCode ?? "KV01");
    setInsurance(0);
    setHistory("");
    setFinancialVerified(false);
    setInsuranceVerified(false);
    setHistoryVerified(false);
    setEvidenceSaved(false);
    // Lý do thuộc về từng hồ sơ; giữ lại sẽ ghi nhầm vào quyết định của hồ sơ khác.
    setReason("");

    // Danh sách đọc từ projection có thể trễ; trạng thái thao tác lấy từ bản gốc.
    getLoan(item.id)
      .then((fresh) => {
        if (selectedRef.current !== item.id) return;
        setDetail(fresh);
        setItems((old) => old.map((row) => (row.id === item.id ? fresh : row)));
      })
      .catch((failure: unknown) => {
        if (selectedRef.current === item.id)
          setError(explainLoanError(failure));
      });
    if (item.status !== "under_review") return;
    getLoanEvidence(item.id)
      .then((saved) => {
        if (selectedRef.current !== item.id) return;
        setSalaries(saved.monthlySalaries);
        setRegion(saved.regionCode);
        setDebt(saved.existingMonthlyDebt);
        setInsurance(saved.insuranceMonths);
        setHistory(saved.repaymentHistory);
        setFinancialVerified(saved.financialDataVerified);
        setInsuranceVerified(saved.insuranceVerified);
        setHistoryVerified(saved.historyVerified);
        setEvidenceSaved(true);
      })
      .catch((failure: unknown) => {
        // 404 nghĩa là chưa đối soát, là trạng thái bình thường.
        if (
          selectedRef.current === item.id &&
          !(failure instanceof LoansApiError && failure.status === 404)
        )
          setError(explainLoanError(failure));
      });
  };
  const documentsReady = DOCUMENT_TYPES.every(
    (type) =>
      latestDocument(detail?.documents ?? [], type)?.verificationStatus ===
      "verified",
  );
  const allVerified =
    financialVerified && insuranceVerified && historyVerified && documentsReady;
  const assessed = !!detail?.assessedAt;

  const evidence: VerifiedEvidenceInput = {
    monthlySalaries: salaries,
    regionCode: region,
    existingMonthlyDebt: debt,
    insuranceMonths: insurance,
    repaymentHistory: history,
    insuranceVerified,
    historyVerified,
    financialDataVerified: financialVerified,
    documentsVerified: documentsReady,
    // Backend yêu cầu ít nhất 3 nguồn: dẫn chiếu tới các chứng từ admin đã đối chiếu.
    sources: (detail?.documents ?? []).map(
      (doc) => `loan-document:${doc.documentId}`,
    ),
  };

  return (
    <div className="grid gap-3">
      <section className="rounded-(--admin-radius-md) bg-(--admin-surface-subtle) p-4">
        <div className="flex items-start gap-3">
          <div className="grid h-9 w-9 shrink-0 place-items-center rounded-(--admin-radius-md) bg-(--admin-primary-soft) text-(--admin-primary)">
            <FontAwesomeIcon icon={faFileInvoiceDollar} aria-hidden="true" />
          </div>
          <div>
            <h1 className="text-[18px] font-bold uppercase tracking-[-0.02em] text-(--admin-text)">
              Thẩm định hồ sơ vay
            </h1>
            <p className="mt-1 text-[12px] leading-5 text-(--admin-text-muted)">
              Xem chứng từ, đối soát thông tin khách hàng và quyết định hồ sơ
              vay.
            </p>
          </div>
        </div>
      </section>

      {error && <Notice message={error} error />}
      {message && <Notice message={message} />}

      <div className="grid items-start gap-3 xl:grid-cols-[minmax(320px,0.9fr)_minmax(0,1.4fr)]">
        <section className={`${panel} grid gap-3 p-4`}>
          <div className="flex items-center justify-between gap-2">
            <h2 className="text-[14px] font-semibold text-(--admin-text)">
              Hồ sơ cần xử lý
            </h2>
            <Button
              disabled={busy}
              onClick={() => {
                void loadList(false);
              }}
            >
              Làm mới
            </Button>
          </div>

          <label className={fieldLabel}>
            Lọc hồ sơ
            <select
              className={fieldInput}
              value={queueFilter}
              onChange={(event) => {
                setQueueFilter(event.target.value);
                void loadList(false, event.target.value);
              }}
            >
              {QUEUE_FILTERS.map((filter) => (
                <option key={filter.label} value={filter.value}>
                  {filter.label}
                </option>
              ))}
            </select>
          </label>

          {items.length === 0 ? (
            <p className="rounded-sm border border-dashed border-(--admin-border) px-3 py-6 text-center text-[11px] text-(--admin-text-muted)">
              {busy ? "Đang tải hồ sơ..." : "Chưa có hồ sơ nào."}
            </p>
          ) : (
            <ul className="grid gap-1.5">
              {items.map((item) => {
                const active = item.id === selectedId;
                return (
                  <li key={item.id}>
                    <button
                      className={`grid w-full gap-1 rounded-sm border px-3 py-2 text-left transition-colors ${
                        active
                          ? "border-(--admin-primary) bg-(--admin-primary-soft)"
                          : "border-(--admin-border) bg-(--admin-background) hover:border-(--admin-primary)"
                      }`}
                      type="button"
                      onClick={() => {
                        selectApplication(item);
                      }}
                    >
                      <span className="flex items-center justify-between gap-2">
                        <strong className="text-[13px] text-(--admin-text)">
                          {vnd(item.requestedAmount)}
                        </strong>
                        <span
                          className={`rounded-sm border px-1.5 py-0.5 text-[10px] ${statusClass(item.status)}`}
                        >
                          {statusLabels[item.status] ?? "Đang cập nhật"}
                        </span>
                      </span>
                      <span className="flex items-center justify-between gap-2 text-[11px] text-(--admin-text-muted)">
                        <span>
                          {item.termMonths} tháng ·{" "}
                          {(item.documents ?? []).length}/{DOCUMENT_COUNT} chứng
                          từ
                        </span>
                        <span>{formatDate(item.createdAt)}</span>
                      </span>
                      <span
                        className={`${mono} truncate text-[10px] text-(--admin-text-muted)`}
                      >
                        {item.applicationNo}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}

          {cursor && (
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => {
                void loadList(true);
              }}
            >
              Xem thêm hồ sơ
            </Button>
          )}
        </section>

        {detail ? (
          <section className={`${panel} grid gap-4 p-4`}>
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div>
                <h2 className="text-[16px] font-bold text-(--admin-text)">
                  {vnd(detail.requestedAmount)} · {detail.termMonths} tháng
                </h2>
                <p
                  className={`${mono} mt-0.5 text-[10px] text-(--admin-text-muted)`}
                >
                  {detail.applicationNo}
                </p>
                <AppliedPackage
                  kind="loan"
                  code={detail.loanPackageId}
                  version={detail.loanPackageVersion}
                />
              </div>
              <span
                className={`rounded-sm border px-2 py-1 text-[11px] ${statusClass(detail.status)}`}
              >
                {statusLabels[detail.status] ?? "Đang cập nhật"}
              </span>
            </div>

            <div
              role="tablist"
              aria-label="Thông tin hồ sơ"
              className="flex gap-1 border-b border-(--admin-border)"
            >
              {TABS.map((item) => (
                <button
                  key={item.id}
                  type="button"
                  role="tab"
                  aria-selected={tab === item.id}
                  onClick={() => setTab(item.id)}
                  className={`-mb-px border-b-2 px-3 py-2 text-[12px] font-semibold ${
                    tab === item.id
                      ? "border-(--admin-primary) text-(--admin-text)"
                      : "border-transparent text-(--admin-text-muted) hover:text-(--admin-text)"
                  }`}
                >
                  {item.label}
                  {item.id === "documents" &&
                    ` (${(detail.documents ?? []).length}/${DOCUMENT_COUNT})`}
                </button>
              ))}
            </div>

            {tab === "documents" && (
              <DocumentsTab
                application={detail}
                onError={setError}
                onChanged={(updated) => {
                  setDetail(updated);
                  setItems((old) =>
                    old.map((item) =>
                      item.id === updated.id ? updated : item,
                    ),
                  );
                }}
              />
            )}
            {tab === "customer" && (
              <CustomerTab
                application={detail}
                all={items}
                onOpen={(item) => {
                  selectApplication(item);
                }}
              />
            )}
            {tab === "review" && (
              <>
                {assessed && (
                  <div className="grid gap-1 rounded-sm border border-(--admin-border) px-3 py-2">
                    <p className="text-[13px] font-bold text-(--admin-text)">
                      Điểm: {detail.assessmentScore ?? 0}
                    </p>
                    {(detail.assessmentReasons ?? []).map((item) => (
                      <p
                        key={item}
                        className="text-[11px] text-(--admin-text-muted)"
                      >
                        {assessmentReasonLabel(item)}
                      </p>
                    ))}
                  </div>
                )}
                <dl className="grid grid-cols-2 gap-2 text-[11px]">
                  <div>
                    <dt className="text-(--admin-text-muted)">
                      Kết quả chấm điểm
                    </dt>
                    <dd className="mt-0.5 font-semibold text-(--admin-text)">
                      {detail.assessmentDecision
                        ? (assessmentLabels[detail.assessmentDecision] ??
                          "Đang cập nhật")
                        : "Chưa chấm điểm"}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-(--admin-text-muted)">Đề nghị vay</dt>
                    <dd className="mt-0.5 font-semibold text-(--admin-text)">
                      {detail.offer
                        ? (offerLabels[detail.offer.status] ?? "Đang cập nhật")
                        : "Chưa gửi"}
                    </dd>
                  </div>
                </dl>
                {detail.status === "submitted" && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      void run(
                        () => startLoanReview(detail.id),
                        "Đã bắt đầu thẩm định.",
                      );
                    }}
                  >
                    Bắt đầu thẩm định
                  </Button>
                )}

                {detail.status === "under_review" && (
                  <div className="grid gap-3">
                    {detail.declared && !assessed && (
                      <p className="rounded-sm border border-(--admin-border) px-3 py-2 text-[11px] text-(--admin-text-muted)">
                        Khách khai: thu nhập{" "}
                        {vnd(detail.declared.monthlyIncome)}, trả nợ{" "}
                        {vnd(detail.declared.existingMonthlyDebt)}/tháng, mục
                        đích: {detail.declared.purpose}. Đối chiếu với chứng từ
                        rồi sửa nếu khác.
                      </p>
                    )}
                    {!detail.declared && !assessed && (
                      <p className="text-[11px] text-(--admin-text-muted)">
                        Hồ sơ cũ chưa có số khách khai, cần nhập từ chứng từ.
                      </p>
                    )}
                    {!assessed && (
                      <>
                        <div className="grid gap-3 md:grid-cols-2">
                          {salaries.map((salary, index) => (
                            <label key={index} className={fieldLabel}>
                              Lương tháng {index + 1} theo sao kê
                              <input
                                className={fieldInput}
                                inputMode="numeric"
                                value={
                                  salary
                                    ? new Intl.NumberFormat("vi-VN").format(
                                        salary,
                                      )
                                    : ""
                                }
                                onChange={(event) => {
                                  const next: [number, number, number] = [
                                    ...salaries,
                                  ];
                                  next[index] = Number(
                                    event.target.value.replace(/\D/g, ""),
                                  );
                                  setSalaries(next);
                                }}
                              />
                            </label>
                          ))}
                          <label className={fieldLabel}>
                            Khu vực
                            <select
                              className={fieldInput}
                              value={region}
                              onChange={(event) =>
                                setRegion(event.target.value)
                              }
                            >
                              <option value="KV01">HCM, Hà Nội, Cần Thơ</option>
                              <option value="KV02">
                                Huế, Đà Nẵng, Hải Phòng
                              </option>
                              <option value="KV03">Khu vực khác</option>
                            </select>
                          </label>
                          <label className={fieldLabel}>
                            Khoản trả nợ mỗi tháng
                            <input
                              className={fieldInput}
                              inputMode="numeric"
                              value={
                                debt
                                  ? new Intl.NumberFormat("vi-VN").format(debt)
                                  : ""
                              }
                              onChange={(event) =>
                                setDebt(
                                  Number(event.target.value.replace(/\D/g, "")),
                                )
                              }
                            />
                          </label>
                          <label className={fieldLabel}>
                            Số tháng đóng BHXH
                            <input
                              className={fieldInput}
                              type="number"
                              value={insurance}
                              onChange={(event) =>
                                setInsurance(Number(event.target.value))
                              }
                            />
                          </label>
                          <label className={fieldLabel}>
                            Lịch sử trả nợ
                            <select
                              className={fieldInput}
                              value={history}
                              onChange={(event) =>
                                setHistory(event.target.value)
                              }
                            >
                              <option value="">Chọn lịch sử trả nợ</option>
                              <option value="previously_late">
                                Từng trả chậm
                              </option>
                              <option value="on_time">Luôn trả đúng hạn</option>
                            </select>
                          </label>
                        </div>

                        <div className="grid gap-1.5">
                          {(
                            [
                              [
                                "Đã đối chiếu thu nhập và nghĩa vụ nợ",
                                financialVerified,
                                setFinancialVerified,
                              ],
                              [
                                "Đã đối chiếu thời gian đóng BHXH",
                                insuranceVerified,
                                setInsuranceVerified,
                              ],
                              [
                                "Đã đối chiếu lịch sử trả nợ",
                                historyVerified,
                                setHistoryVerified,
                              ],
                            ] as const
                          ).map(([label, checked, setChecked]) => (
                            <label
                              key={label}
                              className="flex items-start gap-2 text-[11px] leading-4 text-(--admin-text-muted)"
                            >
                              <input
                                className="mt-0.5"
                                type="checkbox"
                                checked={checked}
                                onChange={(event) =>
                                  setChecked(event.target.checked)
                                }
                              />
                              {label}
                            </label>
                          ))}
                        </div>

                        {!documentsReady && (
                          <p className="text-[11px] text-(--admin-warning)">
                            Cần duyệt đủ {DOCUMENT_COUNT} chứng từ ở tab Chứng
                            từ trước khi lưu kết quả đối soát.
                          </p>
                        )}
                        <div className="flex flex-wrap gap-2">
                          <Button
                            disabled={
                              busy ||
                              !allVerified ||
                              salaries.some((salary) => salary <= 0) ||
                              !history
                            }
                            onClick={() => {
                              void run(async () => {
                                await attestLoanEvidence(detail.id, evidence);
                                setEvidenceSaved(true);
                              }, "Đã lưu kết quả đối soát. Tiếp theo: chấm điểm.");
                            }}
                          >
                            Lưu kết quả đối soát
                          </Button>
                          <Button
                            variant="secondary"
                            disabled={busy || !evidenceSaved || !documentsReady}
                            onClick={() => {
                              void run(
                                () => assessLoan(detail.id),
                                "Đã chấm điểm hồ sơ.",
                              );
                            }}
                          >
                            Chấm điểm
                          </Button>
                        </div>
                      </>
                    )}
                    <label className={fieldLabel}>
                      {assessed ? "Lý do quyết định thủ công" : "Lý do từ chối"}{" "}
                      (khách hàng sẽ thấy)
                      <textarea
                        className={`${fieldInput} h-20 py-2`}
                        value={reason}
                        onChange={(event) => setReason(event.target.value)}
                      />
                    </label>

                    {!assessed && (
                      <div>
                        <Button
                          variant="danger"
                          disabled={busy || !reason.trim()}
                          onClick={() => {
                            if (window.confirm("Từ chối hồ sơ vay này?"))
                              void run(
                                () =>
                                  rejectLoanReview(detail.id, reason.trim()),
                                "Đã từ chối hồ sơ.",
                              );
                          }}
                        >
                          Từ chối hồ sơ
                        </Button>
                      </div>
                    )}

                    {detail.assessmentDecision === "manual_review" && (
                      <div className="flex flex-wrap gap-2">
                        <Button
                          disabled={busy || !reason.trim()}
                          onClick={() => {
                            void run(
                              () =>
                                decideLoan(
                                  detail.id,
                                  "approved",
                                  reason.trim(),
                                ),
                              "Đã phê duyệt thủ công.",
                            );
                          }}
                        >
                          Phê duyệt thủ công
                        </Button>
                        <Button
                          variant="danger"
                          disabled={busy || !reason.trim()}
                          onClick={() => {
                            if (window.confirm("Từ chối hồ sơ vay này?"))
                              void run(
                                () =>
                                  decideLoan(
                                    detail.id,
                                    "rejected",
                                    reason.trim(),
                                  ),
                                "Đã từ chối hồ sơ.",
                              );
                          }}
                        >
                          Từ chối hồ sơ
                        </Button>
                      </div>
                    )}
                  </div>
                )}

                {detail.status === "rejected" &&
                  detail.manualDecisionReason && (
                    <p className="text-[11px] text-(--admin-text-muted)">
                      Lý do từ chối: {detail.manualDecisionReason}
                    </p>
                  )}

                {detail.status === "approved" &&
                  detail.offer?.status === "rejected" && (
                    <Notice message="Khách đã từ chối đề nghị. Hồ sơ này đã kết thúc; khách cần tạo hồ sơ mới nếu muốn vay." />
                  )}
                {detail.status === "approved" && offerExpired(detail) && (
                  <Notice message="Đề nghị đã hết hạn trước khi khách xác nhận. Hồ sơ này đã kết thúc; khách cần tạo hồ sơ mới nếu muốn vay." />
                )}

                {detail.status === "approved" && (
                  <div className="flex flex-wrap items-center gap-2 border-t border-(--admin-border) pt-3">
                    {!detail.offer && (
                      <Button
                        disabled={busy}
                        onClick={() => {
                          void run(
                            () => issueLoanOffer(detail.id),
                            "Đã gửi đề nghị vay. Chờ khách chấp nhận và xác nhận OTP.",
                          );
                        }}
                      >
                        Gửi đề nghị vay
                      </Button>
                    )}
                    {detail.offer?.status === "confirmed" && (
                      <Button
                        disabled={busy}
                        onClick={() => {
                          if (
                            window.confirm(
                              "Giải ngân khoản vay đã xác nhận hợp đồng?",
                            )
                          )
                            void run(
                              () => disburseLoan(detail.id),
                              "Đã gửi yêu cầu giải ngân.",
                            );
                        }}
                      >
                        Giải ngân
                      </Button>
                    )}
                  </div>
                )}
              </>
            )}
          </section>
        ) : (
          <section
            className={`${panel} grid min-h-55 place-items-center p-6 text-center`}
          >
            <div className="grid max-w-90 gap-2">
              <FontAwesomeIcon
                className="mx-auto text-2xl text-(--admin-text-muted)"
                icon={faShieldHalved}
                aria-hidden="true"
              />
              <strong className="text-[14px] text-(--admin-text)">
                Chọn hồ sơ để thẩm định
              </strong>
              <span className="text-[12px] leading-5 text-(--admin-text-muted)">
                Chọn một hồ sơ trong danh sách để xem chi tiết.
              </span>
            </div>
          </section>
        )}
      </div>
    </div>
  );
}
