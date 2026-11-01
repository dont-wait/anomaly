import {
  faArrowLeft,
  faChevronRight,
  faFolderOpen,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { useCallback, useEffect, useState } from "react";
import { navigate, routes, loanApplication } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import {
  acceptLoanOffer,
  confirmLoanContract,
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
import { ApiError } from "@/shared/lib/http";
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

const missingDocumentTypes = (application: LoanApplication) =>
  documentTypes.filter(
    (type) =>
      !(application.documents ?? []).some(
        (doc) => doc.type === type && doc.verificationStatus !== "rejected",
      ),
  );

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

  const upload = async (type: DocumentType, file: File | undefined) => {
    if (!file || !token || !applicationId) return;
    if (
      file.size > 10 * 1024 * 1024 ||
      !["application/pdf", "image/png", "image/jpeg"].includes(file.type)
    ) {
      setError("Chỉ nhận PDF/JPEG/PNG tối đa 10 MiB.");
      return;
    }
    await run(
      () => uploadLoanDocument(token, applicationId, type, file),
      `Đã tải ${documentLabels[type]} lên hồ sơ.`,
    );
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
        <Panel title="Hồ sơ đã tạo">
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
            .map((item) => updatedApplications?.[item.id] ?? item)
            .map((app) => {
              const status = applicationStatusMeta[app.status];
              return (
                <button
                  type="button"
                  key={app.id}
                  onClick={() => navigate(loanApplication(app.id))}
                  className="block w-full rounded-2xl border border-[#e2e7ff] bg-white p-4 text-left shadow-sm active:bg-[#faf8ff]"
                >
                  <span className="flex items-start justify-between gap-3">
                    <span className="min-w-0">
                      <span className="block text-lg font-bold text-primary">
                        {vnd(app.requestedAmount)}
                      </span>
                      <span className="mt-0.5 block text-xs text-on-surface-variant">
                        Kỳ hạn {app.termMonths} tháng
                      </span>
                    </span>
                    <span
                      className={`shrink-0 rounded-full px-2.5 py-1 text-[11px] font-semibold ${status?.className ?? "bg-gray-100 text-gray-700"}`}
                    >
                      {applicationStatusLabel(app.status)}
                    </span>
                  </span>
                  <span className="mt-3 flex items-center justify-between gap-3 border-t border-[#f0f1f9] pt-2.5 text-xs text-on-surface-variant">
                    <span className="min-w-0 truncate">
                      <span>{app.applicationNo || app.id}</span>
                      {app.createdAt &&
                        ` · ${new Date(app.createdAt).toLocaleDateString("vi-VN")}`}
                    </span>
                    <FontAwesomeIcon icon={faChevronRight} />
                  </span>
                  {app.status === "draft" && (
                    <span className="mt-2 block text-xs font-semibold text-secondary">
                      {app.documents
                        ? `Đã có ${documentTypes.length - missingDocumentTypes(app).length}/${documentTypes.length} chứng từ · `
                        : ""}
                      Tiếp tục hoàn tất
                    </span>
                  )}
                </button>
              );
            })}
          {cursor && (
            <Action
              secondary
              disabled={busy}
              onClick={() => {
                if (!token) return;
                setBusy(true);
                void listLoans(token, cursor)
                  .then((page) => {
                    setItems((old) =>
                      appendUnique(old, page.items, (item) => item.id),
                    );
                    setCursor(page.nextCursor);
                  })
                  .catch((e: unknown) => setError(explainLoanError(e)))
                  .finally(() => setBusy(false));
              }}
            >
              Xem thêm hồ sơ
            </Action>
          )}
        </Panel>
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
                  Sau khi nộp, Chúng tôi phải thẩm định và ra quyết định. Quý khách vui lòng chờ kết quả trong 24h kể từ khi yêu cầu được gửi.
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
                  <p className="text-sm">
                    Hồ sơ chưa được gửi. Tải đủ chứng từ rồi gửi để ngân hàng
                    thẩm định.
                  </p>
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
                  {documentTypes.map((type) => {
                    const documents = application.documents ?? [];
                    const doc = documents.find(
                      (d) =>
                        d.type === type && d.verificationStatus !== "rejected",
                    );
                    return (
                      <LoanDocumentCard
                        key={type}
                        type={type}
                        document={doc}
                        rejected={documents.some((d) => d.type === type)}
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
                    );
                  })}
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
                    {application.documents?.map((doc) => (
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
