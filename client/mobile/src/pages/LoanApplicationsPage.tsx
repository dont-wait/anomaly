import {
  faArrowLeft,
  faChevronRight,
  faFolderOpen,
  faTrash,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { useCallback, useEffect, useRef, useState } from "react";
import { navigate, routes, loanApplication } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import {
  acceptLoanOffer,
  attachLoanProfileDocuments,
  confirmLoanContract,
  deleteLoan,
  documentLabels,
  documentTypes,
  downloadLoanDocument,
  getApplicationQuote,
  getLoan,
  getLoanOffer,
  listLoans,
  rejectLoanOffer,
  requestLoanOTP,
  submitLoan,
  uploadLoanDocument,
  type DocumentType,
  type LoanApplication,
  type LoanOffer,
} from "@/features/loans/api";
import {
  applicationStatusLabel,
  applicationStatusMeta,
  statusLabel,
  explainLoanError,
  percentBps,
  vnd,
} from "@/features/loans/format";
import { uploadDocument } from "@/features/auth/api/profileMutations";
import { ApiError } from "@/shared/lib/http";
import { Pagination } from "@/shared/ui";
import {
  LoanDocumentCard,
  LoanDocumentRow,
} from "@/features/loans/components/LoanDocumentCard";
import {
  Action,
  LoanLayout,
  Notice,
  Panel,
} from "@/features/loans/components/LoanLayout";

/** Read-only status is refreshed from API after every action; never infer success from clicks. */
const appendUnique = <T,>(
  current: T[],
  incoming: T[],
  key: (item: T) => string,
) => {
  const seen = new Set(current.map(key));
  return [...current, ...incoming.filter((item) => !seen.has(key(item)))];
};

const PAGE_SIZE = 10;

const PROFILE_DOCUMENT_TYPES: DocumentType[] = [
  "IDENTITY_FRONT",
  "IDENTITY_BACK",
];

const missingDocumentTypes = (application: LoanApplication) =>
  documentTypes.filter(
    (type) =>
      !(application.documents ?? []).some(
        (doc) => doc.type === type && doc.verificationStatus !== "rejected",
      ),
  );

type DocumentState = "complete" | "needsUpdate" | "pending";

/** Chỉ đầy đủ khi cả 7 loại đã được duyệt; đã gửi đủ nhưng chưa duyệt thì đang chờ. */
const documentState = (application: LoanApplication): DocumentState => {
  if (missingDocumentTypes(application).length > 0) return "needsUpdate";
  const allVerified = documentTypes.every((type) =>
    (application.documents ?? []).some(
      (doc) => doc.type === type && doc.verificationStatus === "verified",
    ),
  );
  return allVerified ? "complete" : "pending";
};

const DOCUMENT_SUMMARY: Record<
  DocumentState,
  { text: string; className: string }
> = {
  complete: {
    text: "Chứng từ đầy đủ",
    className: "bg-emerald-50 text-emerald-700",
  },
  needsUpdate: {
    text: "Vui lòng cập nhật chứng từ",
    className: "bg-amber-50 text-amber-800",
  },
  pending: {
    text: "Đã gửi đủ chứng từ, đang chờ ngân hàng duyệt",
    className: "bg-blue-50 text-blue-700",
  },
};

function DocumentSummary({ application }: { application: LoanApplication }) {
  const summary = DOCUMENT_SUMMARY[documentState(application)];
  return (
    <p
      role="status"
      className={`rounded-xl px-3 py-2 text-sm font-semibold ${summary.className}`}
    >
      {summary.text}
    </p>
  );
}

export default function LoanApplicationsPage({
  applicationId,
  updatedApplications,
  onApplicationChange,
}: {
  applicationId?: string;
  updatedApplications?: Record<string, LoanApplication>;
  onApplicationChange?: (application: LoanApplication) => void;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<LoanApplication[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [listLoading, setListLoading] = useState(!applicationId);
  const [application, setApplication] = useState<LoanApplication | null>(null);
  const [offer, setOffer] = useState<LoanOffer | null>(null);
  const [appQuote, setAppQuote] = useState<{
    annualRateBps: number;
    monthlyIncome: number;
  } | null>(null);
  const [otp, setOtp] = useState("");
  const [acceptedTerms, setAcceptedTerms] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [uploadingType, setUploadingType] = useState<DocumentType | null>(null);

  const refresh = useCallback(async () => {
    if (!token) return;
    if (!applicationId) {
      const page = await listLoans(token);
      setItems(page.items ?? []);
      setCursor(page.nextCursor);
      return;
    }
    const detail = await getLoan(token, applicationId);
    setApplication(detail);
    onApplicationChange?.(detail);
    if (detail.status === "approved") {
      try {
        setOffer(await getLoanOffer(token, applicationId));
      } catch (e) {
        setOffer(null);
        // A missing offer (404) is a valid state; surface everything else.
        if (!(e instanceof ApiError && e.status === 404)) {
          setError(explainLoanError(e));
        }
      }
    } else {
      setOffer(detail.offer ?? null);
    }
  }, [applicationId, onApplicationChange, token]);

  useEffect(() => {
    let active = true;
    // Avoid stale network responses on fast route changes.
    if (token)
      void (applicationId ? getLoan(token, applicationId) : listLoans(token))
        .then((result) => {
          if (!active) return;
          if (applicationId) {
            const detail = result as LoanApplication;
            setApplication(detail);
            onApplicationChange?.(detail);
            setOffer(detail.offer ?? null);
          } else {
            const page = result as Awaited<ReturnType<typeof listLoans>>;
            setItems(page.items ?? []);
            setCursor(page.nextCursor);
          }
        })
        .catch((e: unknown) => {
          if (active) setError(explainLoanError(e));
        })
        .finally(() => {
          if (active) setListLoading(false);
        });
    return () => {
      active = false;
    };
  }, [applicationId, onApplicationChange, token]);

  // Chứng từ đã có trong profile thì chụp vào hồ sơ, khách không phải tải lại. Thất bại vẫn
  // tải tay được nên không báo lỗi.
  const attachedProfile = useRef<string | null>(null);
  useEffect(() => {
    if (
      !token ||
      application?.status !== "draft" ||
      attachedProfile.current === application.id ||
      documentTypes.every((type) =>
        (application.documents ?? []).some((doc) => doc.type === type),
      )
    )
      return;
    attachedProfile.current = application.id;
    attachLoanProfileDocuments(token, application.id)
      .then((updated) => {
        setApplication(updated);
        onApplicationChange?.(updated);
      })
      .catch(() => undefined);
  }, [application, onApplicationChange, token]);

  const run = async (fn: () => Promise<unknown>, success: string) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await fn();
      setMessage(success);
      await refresh();
    } catch (e) {
      setError(explainLoanError(e));
    } finally {
      setBusy(false);
    }
  };

  const pageCount = Math.ceil(items.length / PAGE_SIZE);
  const nextPage = async () => {
    if (page < pageCount) {
      setPage(page + 1);
      return;
    }
    if (!token || !cursor) return;
    setBusy(true);
    setError("");
    try {
      const result = await listLoans(token, cursor);
      const merged = appendUnique(items, result.items, (item) => item.id);
      setItems(merged);
      setCursor(result.nextCursor);
      if (merged.length > page * PAGE_SIZE) setPage(page + 1);
    } catch (e) {
      setError(explainLoanError(e));
    } finally {
      setBusy(false);
    }
  };
  const removeDraft = async (id: string) => {
    if (!token || !window.confirm("Xóa hồ sơ vay chưa gửi này?")) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await deleteLoan(token, id);
      const remaining = items.filter((item) => item.id !== id);
      setItems(remaining);
      setPage((value) =>
        Math.min(value, Math.max(Math.ceil(remaining.length / PAGE_SIZE), 1)),
      );
      setMessage("Đã xóa hồ sơ.");
    } catch (e) {
      setError(explainLoanError(e));
    } finally {
      setBusy(false);
    }
  };

  const upload = async (type: DocumentType, file: File | undefined) => {
    if (!file || !token || !applicationId) return;
    if (
      file.size > 10 * 1024 * 1024 ||
      !["application/pdf", "image/png", "image/jpeg"].includes(file.type)
    ) {
      setError("Chỉ nhận PDF/JPEG/PNG tối đa 10 MiB.");
      return;
    }
    await run(async () => {
      // CCCD đến từ KYC nên nộp thẳng vào hồ sơ. Loại khác lưu vào profile để
      // dùng lại cho hồ sơ sau, rồi chụp một bản vào hồ sơ này.
      if (PROFILE_DOCUMENT_TYPES.includes(type)) {
        await uploadLoanDocument(token, applicationId, type, file);
        return;
      }
      await uploadDocument(token, type, file);
      await attachLoanProfileDocuments(token, applicationId);
    }, `Đã tải ${documentLabels[type]} lên hồ sơ.`);
  };
  const download = async (documentId: string) => {
    if (!token || !applicationId) return;
    setError("");
    try {
      const file = await downloadLoanDocument(token, applicationId, documentId);
      const url = URL.createObjectURL(file);
      const link = document.createElement("a");
      link.href = url;
      link.download = `loan-document-${documentId}`;
      document.body.append(link);
      link.click();
      link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      setError(explainLoanError(e));
    }
  };
  const missingDocuments = application ? missingDocumentTypes(application) : [];
  return (
    <LoanLayout
      title={applicationId ? "Chi tiết hồ sơ vay" : "Danh sách hồ sơ vay"}
      overlay={Boolean(applicationId)}
    >
      {error && <Notice message={error} error />}
      {message && <Notice message={message} />}
      {!applicationId && (
        <>
          {listLoading && (
            <p
              role="status"
              className="py-6 text-center text-sm text-on-surface-variant"
            >
              Đang tải hồ sơ...
            </p>
          )}
          {!listLoading && !error && items.length === 0 && (
            <div className="py-8 text-center text-on-surface-variant">
              <FontAwesomeIcon icon={faFolderOpen} className="text-2xl" />
              <p className="mt-2 text-sm">Chưa có hồ sơ</p>
              <p className="mt-1 text-xs">
                Bấm "Tạo mới hồ sơ vay" ở trên để bắt đầu.
              </p>
            </div>
          )}
          {items
            .slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE)
            .map((item) => updatedApplications?.[item.id] ?? item)
            .map((app) => {
              const status = applicationStatusMeta[app.status];
              return (
                <div
                  key={app.id}
                  className="overflow-hidden rounded-2xl border border-[#e2e7ff] bg-white shadow-sm"
                >
                  <button
                    type="button"
                    onClick={() => navigate(loanApplication(app.id))}
                    className="block w-full p-3.5 text-left active:bg-[#faf8ff]"
                  >
                    <span className="flex items-center justify-between gap-3">
                      <strong className="min-w-0 truncate text-lg font-bold text-primary">
                        {vnd(app.requestedAmount)}
                      </strong>
                      <span
                        className={`shrink-0 rounded-full px-2.5 py-0.5 text-xs font-semibold ${status?.className ?? "bg-gray-100 text-gray-700"}`}
                      >
                        {applicationStatusLabel(app.status)}
                      </span>
                    </span>
                    <span className="mt-1 flex items-center justify-between gap-3 text-xs text-on-surface-variant">
                      <span>
                        {app.termMonths} tháng
                        {app.createdAt &&
                          ` · ${new Date(app.createdAt).toLocaleDateString("vi-VN")}`}
                      </span>
                      <FontAwesomeIcon icon={faChevronRight} />
                    </span>
                    <span className="mt-1 block truncate text-xs text-on-surface-variant/70">
                      {app.applicationNo || app.id}
                    </span>
                  </button>
                  {app.status === "draft" && (
                    <div className="flex items-center justify-between gap-2 border-t border-[#f0f1f9] pl-3.5">
                      <span className="min-w-0 truncate text-xs font-semibold text-secondary">
                        {app.documents
                          ? `Đã có ${documentTypes.length - missingDocumentTypes(app).length}/${documentTypes.length} chứng từ`
                          : "Tiếp tục hoàn tất"}
                      </span>
                      <button
                        type="button"
                        disabled={busy}
                        aria-label={`Xóa hồ sơ ${app.applicationNo || app.id}`}
                        onClick={() => void removeDraft(app.id)}
                        className="flex h-11 w-11 shrink-0 items-center justify-center text-error disabled:opacity-50"
                      >
                        <FontAwesomeIcon icon={faTrash} />
                      </button>
                    </div>
                  )}
                </div>
              );
            })}
          <Pagination
            page={page}
            pageCount={pageCount}
            hasMore={!!cursor}
            disabled={busy}
            onPrevious={() => setPage((value) => value - 1)}
            onNext={() => void nextPage()}
          />
        </>
      )}
      {applicationId && (
        <>
          <Action secondary onClick={() => navigate(routes.loanApplications)}>
            <FontAwesomeIcon icon={faArrowLeft} className="mr-1.5" />
            Danh sách hồ sơ
          </Action>
          {!application && <p className="text-sm">Đang tải hồ sơ...</p>}
          {application && (
            <>
              <Panel title="Thông tin hồ sơ">
                <p className="text-sm">
                  Mã: <strong>{application.applicationNo}</strong>
                </p>
                <p className="text-sm">
                  Số tiền: <strong>{vnd(application.requestedAmount)}</strong>
                </p>
                <p className="text-sm">
                  Kỳ hạn: {application.termMonths} tháng
                </p>
                <p className="text-sm">
                  Trạng thái:{" "}
                  <strong>{applicationStatusLabel(application.status)}</strong>
                </p>
                <p className="text-xs text-on-surface-variant italic">
                  Sau khi nộp, Chúng tôi phải thẩm định và ra quyết định. Quý
                  khách vui lòng chờ kết quả trong 24h kể từ khi yêu cầu được
                  gửi.
                </p>
                <Action
                  secondary
                  onClick={() =>
                    void run(async () => {
                      await refresh();
                    }, "Đã tải lại trạng thái.")
                  }
                >
                  Làm mới trạng thái
                </Action>
              </Panel>
              {application.status === "draft" && (
                <Panel title="Hoàn tất hồ sơ">
                  <DocumentSummary application={application} />
                  <label className="block text-xs font-semibold text-primary">
                    Đã có {documentTypes.length - missingDocuments.length}/
                    {documentTypes.length} chứng từ
                    <progress
                      className="mt-1.5 block h-2 w-full accent-[#6b38d4]"
                      max={documentTypes.length}
                      value={documentTypes.length - missingDocuments.length}
                    />
                  </label>
                  <p className="text-xs text-on-surface-variant">
                    Chụp trực tiếp hoặc chọn tệp PDF, JPEG, PNG, tối đa 10 MiB
                    mỗi tệp.
                  </p>
                  {missingDocuments.length > 0 && (
                    <p className="text-xs text-on-surface-variant">
                      Chụp trực tiếp hoặc chọn tệp PDF, JPEG, PNG, tối đa 10 MiB
                      mỗi tệp.
                    </p>
                  )}
                  {missingDocuments.map((type) => (
                    <LoanDocumentCard
                      key={type}
                      type={type}
                      rejected={(application.documents ?? []).some(
                        (d) => d.type === type,
                      )}
                      disabled={busy}
                      uploading={uploadingType === type}
                      onSelect={(file) => {
                        setUploadingType(type);
                        void upload(type, file).finally(() =>
                          setUploadingType(null),
                        );
                      }}
                      onDownload={(id) => void download(id)}
                    />
                  ))}
                  {missingDocuments.length > 0 && (
                    <p className="text-xs text-on-surface-variant">
                      Còn thiếu:{" "}
                      {missingDocuments
                        .map((type) => documentLabels[type])
                        .join(", ")}
                    </p>
                  )}
                  <Action
                    disabled={busy || missingDocuments.length > 0}
                    onClick={() => {
                      if (token)
                        void run(
                          () => submitLoan(token, applicationId),
                          "Hồ sơ đã nộp, đang chờ thẩm định.",
                        );
                    }}
                  >
                    Gửi hồ sơ thẩm định
                  </Action>
                </Panel>
              )}
              {application.status !== "draft" &&
                (application.documents?.length ?? 0) > 0 && (
                  <Panel title="Chứng từ đã gửi">
                    <DocumentSummary application={application} />
                    {documentState(application) !== "complete" &&
                      application.documents?.map((doc) => (
                        <LoanDocumentRow
                          key={doc.documentId}
                          document={doc}
                          onDownload={(id) => void download(id)}
                        />
                      ))}
                  </Panel>
                )}
              {application.status === "under_review" && (
                <Panel title="Kết quả sau thẩm định">
                  <Action
                    secondary
                    disabled={busy}
                    onClick={() => {
                      if (!token) return;
                      void run(async () => {
                        const result = await getApplicationQuote(
                          token,
                          applicationId,
                        );
                        setAppQuote(result);
                      }, "Đã lấy báo giá hồ sơ.");
                    }}
                  >
                    Xem báo giá sau thẩm định
                  </Action>
                  {appQuote && (
                    <p className="text-sm">
                      Thu nhập đã xác minh: {vnd(appQuote.monthlyIncome)} · Lãi
                      suất dự kiến: {percentBps(appQuote.annualRateBps)}
                    </p>
                  )}
                </Panel>
              )}
              {application.status === "approved" && (
                <Panel title="Đề nghị vay và hợp đồng">
                  <Action
                    secondary
                    disabled={busy}
                    onClick={() => {
                      if (!token) return;
                      void run(
                        async () =>
                          setOffer(await getLoanOffer(token, applicationId)),
                        "Đã tải đề nghị vay.",
                      );
                    }}
                  >
                    Xem đề nghị vay
                  </Action>
                  {offer && (
                    <>
                      <p className="text-sm">
                        Đề nghị vay:{" "}
                        <strong>{statusLabel(offer.status)}</strong> ·{" "}
                        {vnd(offer.amount)} · {offer.termMonths} tháng
                      </p>
                      <p className="text-sm">
                        Lãi suất hợp đồng đề xuất:{" "}
                        <strong>{percentBps(offer.annualRateBps)}</strong>
                      </p>
                      {offer.status === "issued" && (
                        <>
                          <Action
                            disabled={busy}
                            onClick={() => {
                              if (token)
                                void run(
                                  async () =>
                                    setOffer(
                                      await acceptLoanOffer(
                                        token,
                                        applicationId,
                                      ),
                                    ),
                                  "Đã chấp nhận đề nghị. Hãy xác nhận bằng mã OTP.",
                                );
                            }}
                          >
                            Chấp nhận đề nghị
                          </Action>
                          <Action
                            secondary
                            disabled={busy}
                            onClick={() => {
                              if (
                                token &&
                                window.confirm(
                                  "Từ chối sẽ kết thúc đề nghị này. Tiếp tục?",
                                )
                              )
                                void run(
                                  async () =>
                                    setOffer(
                                      await rejectLoanOffer(
                                        token,
                                        applicationId,
                                      ),
                                    ),
                                  "Đã từ chối đề nghị.",
                                );
                            }}
                          >
                            Từ chối đề nghị
                          </Action>
                        </>
                      )}
                      {offer.status === "accepted" && (
                        <>
                          <Action
                            disabled={busy}
                            onClick={() => {
                              if (token)
                                void run(
                                  () => requestLoanOTP(token, applicationId),
                                  "Đã yêu cầu mã OTP về email xác thực.",
                                );
                            }}
                          >
                            Gửi OTP xác nhận hợp đồng
                          </Action>
                          <label className="block text-sm">
                            Mã OTP từ email
                            <input
                              value={otp}
                              onChange={(e) => setOtp(e.target.value)}
                              autoComplete="one-time-code"
                              className="mt-1 w-full rounded-xl border p-3"
                            />
                          </label>
                          <label className="flex items-start gap-2 text-xs">
                            <input
                              type="checkbox"
                              checked={acceptedTerms}
                              onChange={(e) =>
                                setAcceptedTerms(e.target.checked)
                              }
                            />{" "}
                            Tôi đã đọc và đồng ý các điều khoản của đề nghị vay.
                          </label>
                          <Action
                            disabled={busy || !acceptedTerms || !otp.trim()}
                            onClick={() => {
                              if (token)
                                void run(async () => {
                                  setOffer(
                                    await confirmLoanContract(
                                      token,
                                      applicationId,
                                      otp.trim(),
                                    ),
                                  );
                                  setOtp("");
                                }, "Đã xác nhận hợp đồng bằng OTP. Chờ admin giải ngân.");
                            }}
                          >
                            Xác nhận hợp đồng
                          </Action>
                        </>
                      )}
                      {offer.status === "confirmed" && (
                        <Notice message="Hợp đồng đã xác nhận. Chờ nhân viên giải ngân." />
                      )}
                      {offer.status === "rejected" && (
                        <Notice message="Bạn đã từ chối đề nghị. Hãy tạo hồ sơ mới nếu cần." />
                      )}
                    </>
                  )}
                </Panel>
              )}
              {application.status === "disbursed" && (
                <Panel title="Khoản vay đã giải ngân">
                  <Notice message="Hợp đồng được đọc ở danh sách khoản vay. Nếu chưa xuất hiện, kiểm tra Loan Projector." />
                  <Action onClick={() => navigate(routes.loanContracts)}>
                    Xem hợp đồng và lịch trả nợ
                  </Action>
                </Panel>
              )}
            </>
          )}
        </>
      )}
    </LoanLayout>
  );
}
