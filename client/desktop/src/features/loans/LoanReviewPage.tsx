import { useState, type ReactNode } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCircleExclamation,
  faFileInvoiceDollar,
  faShieldHalved,
} from "@fortawesome/free-solid-svg-icons";
import {
  assessLoan,
  attestLoanEvidence,
  decideLoan,
  disburseLoan,
  getLoan,
  issueLoanOffer,
  listLoans,
  startLoanReview,
} from "./api/loansApi";
import { explainLoanError, statusLabels, vnd } from "./format";
import type { LoanApplication, VerifiedEvidenceInput } from "./model";

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
}: {
  children: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  variant?: "primary" | "secondary" | "danger";
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

function statusClass(status: string): string {
  if (status === "approved" || status === "disbursed")
    return "border-(--admin-success) bg-(--admin-success-soft) text-(--admin-success)";
  if (status === "rejected")
    return "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)";
  if (status === "under_review")
    return "border-(--admin-warning) bg-(--admin-warning-soft) text-(--admin-warning)";
  return "border-(--admin-border) bg-(--admin-surface-hover) text-(--admin-text-secondary)";
}

export function LoanReviewPage() {
  const [items, setItems] = useState<LoanApplication[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState("");
  const [detail, setDetail] = useState<LoanApplication | null>(null);
  const [income, setIncome] = useState(20_000_000);
  const [region, setRegion] = useState("KV01");
  const [debt, setDebt] = useState(0);
  const [insurance, setInsurance] = useState(24);
  const [history, setHistory] = useState("previously_late");
  const [verified, setVerified] = useState(false);
  const [reason, setReason] = useState(
    "Đã xác minh hồ sơ test và phê duyệt thủ công",
  );
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
      if (id) setDetail(await getLoan(id));
    } catch (error) {
      setError(explainLoanError(error));
    } finally {
      setBusy(false);
    }
  };

  const loadList = async (append: boolean) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      const page = await listLoans(append ? (cursor ?? undefined) : undefined);
      setItems((old) => {
        if (!append) return page.items;
        const seen = new Set(old.map((item) => item.id));
        return [...old, ...page.items.filter((item) => !seen.has(item.id))];
      });
      setCursor(page.nextCursor);
      setMessage(
        append ? "Đã tải thêm hồ sơ." : "Đã tải danh sách hồ sơ cần thẩm định.",
      );
    } catch (error) {
      setError(explainLoanError(error));
    } finally {
      setBusy(false);
    }
  };

  const openDetail = (id: string) => {
    const normalized = id.trim();
    if (!normalized) return;
    setSelectedId(normalized);
    setDetail(null);
    void run(() => getLoan(normalized), "Đã tải chi tiết hồ sơ.");
  };

  const evidence: VerifiedEvidenceInput = {
    monthlySalaries: [income, income, income],
    regionCode: region,
    existingMonthlyDebt: debt,
    insuranceMonths: insurance,
    repaymentHistory: history,
    insuranceVerified: verified,
    historyVerified: verified,
    financialDataVerified: verified,
    documentsVerified: verified,
    sources: ["admin-console-manual-evidence"],
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
              Duyệt hồ sơ, ghi Evidence và giải ngân. Chỉ dùng với tài khoản
              admin và dữ liệu kiểm thử; không giả lập xác minh giấy tờ thật.
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
              Tải danh sách hồ sơ
            </Button>
          </div>

          {items.length === 0 ? (
            <p className="rounded-sm border border-dashed border-(--admin-border) px-3 py-6 text-center text-[11px] text-(--admin-text-muted)">
              Chưa tải hồ sơ nào.
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
                        setSelectedId(item.id);
                        setDetail(item);
                      }}
                    >
                      <span className="flex items-center justify-between gap-2">
                        <strong
                          className={`${mono} text-[12px] text-(--admin-text)`}
                        >
                          {item.applicationNo}
                        </strong>
                        <span
                          className={`rounded-sm border px-1.5 py-0.5 text-[10px] ${statusClass(item.status)}`}
                        >
                          {statusLabels[item.status] || item.status}
                        </span>
                      </span>
                      <span className="flex items-center justify-between gap-2 text-[11px] text-(--admin-text-muted)">
                        <span>{vnd(item.requestedAmount)}</span>
                        <span>{item.termMonths} tháng</span>
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

          <div className="grid gap-2 border-t border-(--admin-border) pt-3">
            <label className={fieldLabel} htmlFor="loan-application-id">
              Application ID
              <input
                id="loan-application-id"
                className={`${fieldInput} ${mono}`}
                value={selectedId}
                onChange={(event) => {
                  setSelectedId(event.target.value);
                  setDetail(null);
                }}
              />
            </label>
            <Button
              variant="secondary"
              disabled={busy || !selectedId.trim()}
              onClick={() => openDetail(selectedId)}
            >
              Mở hồ sơ
            </Button>
          </div>
        </section>

        {detail ? (
          <section className={`${panel} grid gap-4 p-4`}>
            <div className="flex flex-wrap items-start justify-between gap-2">
              <div>
                <h2
                  className={`${mono} text-[14px] font-semibold text-(--admin-text)`}
                >
                  {detail.applicationNo}
                </h2>
                <p
                  className={`${mono} mt-0.5 text-[10px] text-(--admin-text-muted)`}
                >
                  {detail.id}
                </p>
              </div>
              <span
                className={`rounded-sm border px-2 py-1 text-[11px] ${statusClass(detail.status)}`}
              >
                {statusLabels[detail.status] || detail.status}
              </span>
            </div>

            <dl className="grid grid-cols-2 gap-2 text-[11px] md:grid-cols-4">
              <div>
                <dt className="text-(--admin-text-muted)">Số tiền</dt>
                <dd className="mt-0.5 font-semibold text-(--admin-text)">
                  {vnd(detail.requestedAmount)}
                </dd>
              </div>
              <div>
                <dt className="text-(--admin-text-muted)">Kỳ hạn</dt>
                <dd className="mt-0.5 font-semibold text-(--admin-text)">
                  {detail.termMonths} tháng
                </dd>
              </div>
              <div>
                <dt className="text-(--admin-text-muted)">Kết quả assess</dt>
                <dd className="mt-0.5 font-semibold text-(--admin-text)">
                  {detail.assessmentDecision || "Chưa assess"}
                </dd>
              </div>
              <div>
                <dt className="text-(--admin-text-muted)">Offer</dt>
                <dd className="mt-0.5 font-semibold text-(--admin-text)">
                  {detail.offer?.status || "Chưa phát hành"}
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
                Bắt đầu review
              </Button>
            )}

            {detail.status === "under_review" && (
              <div className="grid gap-3">
                <div className="grid gap-3 md:grid-cols-2">
                  <label className={fieldLabel}>
                    Lương mỗi tháng (mẫu 3 tháng)
                    <input
                      className={fieldInput}
                      type="number"
                      value={income}
                      onChange={(event) =>
                        setIncome(Number(event.target.value))
                      }
                    />
                  </label>
                  <label className={fieldLabel}>
                    Khu vực
                    <select
                      className={fieldInput}
                      value={region}
                      onChange={(event) => setRegion(event.target.value)}
                    >
                      <option value="KV01">KV01</option>
                      <option value="KV02">KV02</option>
                      <option value="KV03">KV03</option>
                    </select>
                  </label>
                  <label className={fieldLabel}>
                    Nợ hàng tháng
                    <input
                      className={fieldInput}
                      type="number"
                      value={debt}
                      onChange={(event) => setDebt(Number(event.target.value))}
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
                      onChange={(event) => setHistory(event.target.value)}
                    >
                      <option value="previously_late">
                        previously_late (manual review)
                      </option>
                      <option value="on_time">on_time</option>
                    </select>
                  </label>
                </div>

                <label className="flex items-start gap-2 text-[11px] leading-4 text-(--admin-text-muted)">
                  <input
                    className="mt-0.5"
                    type="checkbox"
                    checked={verified}
                    onChange={(event) => setVerified(event.target.checked)}
                  />
                  Tôi đã xác minh Evidence trên môi trường TEST (không tích hợp
                  đối soát thật).
                </label>

                <div className="flex flex-wrap gap-2">
                  <Button
                    disabled={busy || !verified}
                    onClick={() => {
                      void run(
                        () => attestLoanEvidence(detail.id, evidence),
                        "Đã lưu Evidence. Tiếp theo: chấm điểm.",
                      );
                    }}
                  >
                    Ghi Evidence đã kiểm tra
                  </Button>
                  <Button
                    variant="secondary"
                    disabled={busy}
                    onClick={() => {
                      void run(
                        () => assessLoan(detail.id),
                        "Đã chạy assessment. Nếu manual_review, cần quyết định thủ công.",
                      );
                    }}
                  >
                    Chấm điểm / Assess
                  </Button>
                </div>

                <label className={fieldLabel}>
                  Lý do quyết định thủ công
                  <textarea
                    className={`${fieldInput} h-20 py-2`}
                    value={reason}
                    onChange={(event) => setReason(event.target.value)}
                  />
                </label>

                {detail.assessmentDecision === "manual_review" && (
                  <div className="flex flex-wrap gap-2">
                    <Button
                      disabled={busy || !reason.trim()}
                      onClick={() => {
                        void run(
                          () =>
                            decideLoan(detail.id, "approved", reason.trim()),
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
                              decideLoan(detail.id, "rejected", reason.trim()),
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

            {detail.status === "approved" && (
              <div className="flex flex-wrap items-center gap-2 border-t border-(--admin-border) pt-3">
                {!detail.offer && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      void run(
                        () => issueLoanOffer(detail.id),
                        "Đã phát hành Offer. Khách hàng cần accept + OTP.",
                      );
                    }}
                  >
                    Phát hành Offer
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
                          "Đã gọi giải ngân. Kiểm tra contract projection.",
                        );
                    }}
                  >
                    Giải ngân
                  </Button>
                )}
              </div>
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
                Tải danh sách hoặc nhập Application ID để mở hồ sơ.
              </span>
            </div>
          </section>
        )}
      </div>
    </div>
  );
}
