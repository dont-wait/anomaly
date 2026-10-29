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
import { explainLoanError, percentBps, vnd } from "@/features/loans/format";
import {
  Action,
  LoanLayout,
  Notice,
  Panel,
} from "@/features/loans/components/LoanLayout";

/** Read-only status is refreshed from API after every action; never infer success from clicks. */
export default function LoanApplicationsPage({
  applicationId,
}: {
  applicationId?: string;
}) {
  const { token } = useAuth();
  const [items, setItems] = useState<LoanApplication[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
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
    if (detail.status === "approved") {
      try {
        setOffer(await getLoanOffer(token, applicationId));
      } catch {
        setOffer(null);
      }
    } else {
      setOffer(detail.offer ?? null);
    }
  }, [applicationId, token]);

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
            setOffer(detail.offer ?? null);
          } else {
            const page = result as Awaited<ReturnType<typeof listLoans>>;
            setItems(page.items ?? []);
            setCursor(page.nextCursor);
          }
        })
        .catch((e: unknown) => {
          if (active) setError(explainLoanError(e));
        });
    return () => {
      active = false;
    };
  }, [applicationId, token]);

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
  return (
    <LoanLayout
      title={applicationId ? "Chi tiết hồ sơ vay" : "Danh sách hồ sơ vay"}
    >
      {error && <Notice message={error} error />}
      {message && <Notice message={message} />}
      {!applicationId && (
        <Panel title="Hồ sơ đã tạo">
          <Action onClick={() => navigate(routes.loanRegister)}>
            + Tạo hồ sơ vay
          </Action>
          {items.length === 0 && (
            <p className="text-sm">
              Chưa có hồ sơ (hoặc read model đang đồng bộ).
            </p>
          )}
          {items.map((app) => (
            <button
              type="button"
              key={app.id}
              onClick={() => navigate(loanApplication(app.id))}
              className="block w-full rounded-xl border p-3 text-left text-sm"
            >
              <strong>{app.applicationNo || app.id}</strong>
              <span className="float-right text-secondary">{app.status}</span>
              <span className="mt-1 block">
                {vnd(app.requestedAmount)} · {app.termMonths} tháng
              </span>
            </button>
          ))}
          {cursor && (
            <Action
              secondary
              disabled={busy}
              onClick={() => {
                if (!token) return;
                setBusy(true);
                void listLoans(token, cursor)
                  .then((page) => {
                    setItems((old) => [...old, ...page.items]);
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
            ← Danh sách hồ sơ
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
                  Trạng thái: <strong>{application.status}</strong>
                </p>
                <p className="text-xs text-on-surface-variant">
                  Sau khi nộp, admin phải thẩm định và ra quyết định. App không
                  tự phê duyệt hoặc giải ngân.
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
                <Panel title="Tải đủ 7 chứng từ">
                  {documentTypes.map((type) => {
                    const doc = application.documents?.find(
                      (d) =>
                        d.type === type && d.verificationStatus !== "rejected",
                    );
                    return (
                      <label
                        key={type}
                        className="block rounded-xl border p-3 text-xs"
                      >
                        <span className="font-semibold">
                          {documentLabels[type]}{" "}
                          {doc ? `✓ ${doc.verificationStatus}` : "(chưa có)"}
                        </span>
                        <input
                          type="file"
                          accept=".pdf,.png,.jpg,.jpeg,application/pdf,image/png,image/jpeg"
                          disabled={busy || !!doc}
                          onChange={(e) =>
                            void upload(type, e.target.files?.[0])
                          }
                          className="mt-2 block w-full text-xs"
                        />
                      </label>
                    );
                  })}
                  <Action
                    disabled={
                      busy ||
                      !documentTypes.every((type) =>
                        (application.documents ?? []).some(
                          (d) =>
                            d.type === type &&
                            d.verificationStatus !== "rejected",
                        ),
                      )
                    }
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
              {(application.documents?.length ?? 0) > 0 && (
                <Panel title="Chứng từ đã gửi">
                  {application.documents?.map((doc) => (
                    <button
                      type="button"
                      key={doc.documentId}
                      onClick={() => void download(doc.documentId)}
                      className="block w-full rounded-xl border p-2 text-left text-xs"
                    >
                      {documentLabels[doc.type] ?? doc.type} ·{" "}
                      {doc.verificationStatus} · Tải xuống
                    </button>
                  ))}
                </Panel>
              )}
              {application.status === "under_review" && (
                <Panel title="Kết quả tính theo Evidence">
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
                        "Đã tải Offer.",
                      );
                    }}
                  >
                    Xem Offer đã phát hành
                  </Action>
                  {offer && (
                    <>
                      <p className="text-sm">
                        Offer: <strong>{offer.status}</strong> ·{" "}
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
                                  "Đã chấp nhận Offer. Hãy xác nhận OTP.",
                                );
                            }}
                          >
                            Chấp nhận Offer
                          </Action>
                          <Action
                            secondary
                            disabled={busy}
                            onClick={() => {
                              if (
                                token &&
                                window.confirm(
                                  "Từ chối Offer sẽ kết thúc đề nghị này. Tiếp tục?",
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
                                  "Đã từ chối Offer.",
                                );
                            }}
                          >
                            Từ chối Offer
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
                            Tôi đã đọc và đồng ý các điều khoản Offer.
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
                        <Notice message="Bạn đã từ chối Offer. Hãy tạo hồ sơ mới nếu cần." />
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
