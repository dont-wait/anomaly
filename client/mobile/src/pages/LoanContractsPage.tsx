import { faArrowLeft } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { useCallback, useEffect, useState } from "react";
import { navigate, routes, loanContract } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import {
  confirmRepayment,
  getLoanContract,
  getLoanPayment,
  getRepaymentSchedule,
  listLoanContracts,
  listLoanPayments,
  quoteRepayment,
  type LoanContract,
  type LoanContractDetail,
  type LoanPayment,
  type RepaymentQuote,
  type RepaymentSchedule,
  type RepaymentResult,
} from "@/features/loans/api";
import { explainLoanError, statusLabel, vnd } from "@/features/loans/format";
import {
  clearIdempotencyKey,
  getOrCreateIdempotencyKey,
} from "@/features/loans/idempotency";
import {
  Action,
  LoanLayout,
  Notice,
  Panel,
} from "@/features/loans/components/LoanLayout";

const appendUnique = <T,>(
  current: T[],
  incoming: T[],
  key: (item: T) => string,
) => {
  const seen = new Set(current.map(key));
  return [...current, ...incoming.filter((item) => !seen.has(key(item)))];
};

export default function LoanContractsPage({ loanId }: { loanId?: string }) {
  const { token, user } = useAuth();
  const [contracts, setContracts] = useState<LoanContract[]>([]);
  const [nextContractCursor, setNextContractCursor] = useState<string | null>(
    null,
  );
  const [contract, setContract] = useState<LoanContractDetail | null>(null);
  const [schedule, setSchedule] = useState<RepaymentSchedule | null>(null);
  const [payments, setPayments] = useState<LoanPayment[]>([]);
  const [nextPaymentCursor, setNextPaymentCursor] = useState<string | null>(
    null,
  );
  const [paymentDetail, setPaymentDetail] = useState<LoanPayment | null>(null);
  const sourceAccountId = user?.id ?? "";
  const [quote, setQuote] = useState<RepaymentQuote | null>(null);
  const [repaymentResult, setRepaymentResult] =
    useState<RepaymentResult | null>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    if (!token) return;
    if (!loanId) {
      const page = await listLoanContracts(token);
      setContracts(page.items ?? []);
      setNextContractCursor(page.nextCursor);
    } else {
      // Detail, schedule and payments are separate read models; don't merge guesses.
      const [detail, repaymentSchedule, paymentPage] = await Promise.all([
        getLoanContract(token, loanId),
        getRepaymentSchedule(token, loanId),
        listLoanPayments(token, loanId),
      ]);
      setContract(detail);
      setSchedule(repaymentSchedule);
      setPayments(paymentPage.items ?? []);
      setNextPaymentCursor(paymentPage.nextCursor);
    }
  }, [token, loanId]);

  useEffect(() => {
    let mounted = true;
    if (token)
      void (async () => {
        try {
          if (loanId) {
            const [detail, current, page] = await Promise.all([
              getLoanContract(token, loanId),
              getRepaymentSchedule(token, loanId),
              listLoanPayments(token, loanId),
            ]);
            if (mounted) {
              setContract(detail);
              setSchedule(current);
              setPayments(page.items ?? []);
              setNextPaymentCursor(page.nextCursor);
            }
          } else {
            const page = await listLoanContracts(token);
            if (mounted) {
              setContracts(page.items ?? []);
              setNextContractCursor(page.nextCursor);
            }
          }
        } catch (e) {
          if (mounted) setError(explainLoanError(e));
        }
      })();
    return () => {
      mounted = false;
    };
  }, [token, loanId]);

  const run = async (fn: () => Promise<unknown>, success: string) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await fn();
      setMessage(success);
    } catch (e) {
      setError(explainLoanError(e));
    } finally {
      setBusy(false);
    }
  };

  const getQuote = async () => {
    if (!token || !loanId || !schedule || !sourceAccountId.trim()) {
      setError("Thiếu thông tin khoản vay hoặc tài khoản thanh toán.");
      return;
    }
    setQuote(null);
    setRepaymentResult(null);
    await run(async () => {
      const next = await quoteRepayment(
        token,
        loanId,
        schedule.nextInstallmentNo,
        sourceAccountId.trim(),
      );
      setQuote(next);
    }, "Đã lấy báo giá trả nợ. Xem lại trước khi xác nhận.");
  };

  const confirm = async () => {
    if (!token || !loanId || !quote) return;
    if (Date.now() >= Date.parse(quote.quoteExpiresAt)) {
      setQuote(null);
      setError("Báo giá đã hết hạn. Vui lòng lấy báo giá mới.");
      return;
    }
    const fingerprint = `${quote.quoteId}:${sourceAccountId.trim()}`;
    const scope = `repay:${user?.id ?? "anonymous"}:${loanId}`;
    const idempotencyKey = getOrCreateIdempotencyKey(scope, fingerprint);
    await run(async () => {
      const result = await confirmRepayment(
        token,
        loanId,
        quote.quoteId,
        sourceAccountId.trim(),
        idempotencyKey,
      );
      clearIdempotencyKey(scope, fingerprint);
      setRepaymentResult(result);
      setQuote(null);
      // Read projection may lag; payment result is authoritative until refresh.
    }, "Giao dịch trả nợ đã được ghi nhận. Tải lại để kiểm tra lịch và dư nợ.");
  };

  /** After a successful payment the local schedule is stale until reloaded. */
  const reloadAfterRepayment = async () => {
    await run(async () => {
      await refresh();
      setRepaymentResult(null);
    }, "Đã tải lại dư nợ và lịch trả nợ.");
  };

  return (
    <LoanLayout title={loanId ? "Chi tiết khoản vay" : "Hợp đồng vay & trả nợ"}>
      {error && <Notice message={error} error />}
      {message && <Notice message={message} />}
      {!loanId && (
        <Panel title="Hợp đồng đã giải ngân">
          {contracts.length === 0 && (
            <p className="text-sm">
              Chưa có khoản vay giải ngân (hoặc projector chưa cập nhật).
            </p>
          )}
          {contracts.map((item) => (
            <button
              type="button"
              key={item.loanId}
              onClick={() => navigate(loanContract(item.loanId))}
              className="block w-full rounded-xl border p-3 text-left text-sm"
            >
              <strong>{item.loanNo}</strong>
              <span className="float-right text-secondary">
                {statusLabel(item.status)}
              </span>
              <span className="mt-1 block">
                Dư nợ {vnd(item.outstandingPrincipal)} · {item.paidInstallments}
                /{item.totalInstallments} kỳ
              </span>
            </button>
          ))}
          {nextContractCursor && (
            <Action
              secondary
              disabled={busy}
              onClick={() => {
                if (!token) return;
                void run(async () => {
                  const page = await listLoanContracts(
                    token,
                    nextContractCursor,
                  );
                  setContracts((old) =>
                    appendUnique(old, page.items, (item) => item.loanId),
                  );
                  setNextContractCursor(page.nextCursor);
                }, "Đã tải thêm hợp đồng.");
              }}
            >
              Xem thêm hợp đồng
            </Action>
          )}
          <Action
            secondary
            onClick={() => void run(refresh, "Đã làm mới danh sách.")}
          >
            Tải lại danh sách
          </Action>
        </Panel>
      )}
      {loanId && (
        <>
          <Action secondary onClick={() => navigate(routes.loanContracts)}>
            <FontAwesomeIcon icon={faArrowLeft} className="mr-1.5" />
            Danh sách hợp đồng
          </Action>
          {!contract && <Notice message="Đang tải dữ liệu hợp đồng..." />}
          {contract && (
            <Panel title="Thông tin hợp đồng">
              <p className="text-sm">
                Số hợp đồng: <strong>{contract.loanNo}</strong>
              </p>
              <p className="text-sm">Gốc vay: {vnd(contract.principal)}</p>
              <p className="text-sm">
                Dư nợ: <strong>{vnd(contract.outstandingPrincipal)}</strong>
              </p>
              <p className="text-sm">
                Lãi suất đã ký: {contract.annualInterestRate}%/năm
              </p>
              <p className="text-sm">
                Trạng thái: {statusLabel(contract.status)}
              </p>
              <p className="text-sm">
                Kỳ đã trả: {contract.paidInstallments}/
                {contract.totalInstallments}
              </p>
              <p className="text-sm">
                Kỳ kế tiếp: {contract.nextInstallmentNo || "Không có"} ·{" "}
                {contract.nextDueDate || "—"}
              </p>
              <Action
                secondary
                disabled={busy}
                onClick={() =>
                  void run(
                    refresh,
                    "Đã tải lại dữ liệu. Nếu Mongo projector trễ, kết quả có thể chưa cập nhật ngay.",
                  )
                }
              >
                Làm mới dư nợ / lịch sử
              </Action>
            </Panel>
          )}
          {schedule && (
            <Panel title="Lịch thanh toán">
              {schedule.items?.map((item) => (
                <div
                  key={item.installmentNo}
                  className="rounded-xl border p-3 text-xs"
                >
                  <div className="flex justify-between">
                    <strong>
                      Kỳ {item.installmentNo} · {item.dueDate}
                    </strong>
                    <span>{statusLabel(item.status)}</span>
                  </div>
                  <p className="mt-1">
                    Gốc {vnd(item.principalDue)} · Lãi {vnd(item.interestDue)}
                  </p>
                  <p className="font-semibold">Tổng: {vnd(item.totalDue)}</p>
                </div>
              ))}
            </Panel>
          )}
          {schedule && repaymentResult && (
            <Panel title="Kết quả trả nợ">
              <Notice
                message={`Đã thanh toán kỳ ${repaymentResult.installmentNo}: ${vnd(repaymentResult.amount)}. Mã: ${repaymentResult.paymentId}. Dư nợ sau trả: ${vnd(repaymentResult.outstandingPrincipalAfter)}.`}
              />
              <p className="text-xs text-on-surface-variant">
                Không thể trả lại kỳ vừa thanh toán. Tải lại để lấy kỳ kế tiếp.
              </p>
              <Action
                secondary
                disabled={busy}
                onClick={() => void reloadAfterRepayment()}
              >
                Tải lại dư nợ / lịch sử
              </Action>
            </Panel>
          )}
          {schedule &&
            !repaymentResult &&
            schedule.status !== "completed" &&
            schedule.nextInstallmentNo > 0 && (
              <Panel title="Thanh toán kỳ tới hạn">
                <p className="text-xs">
                  Chỉ cho phép trả đủ một kỳ đúng ngày đến hạn (theo giờ Việt
                  Nam). Hiện chưa hỗ trợ trả sớm hoặc trả trễ.
                </p>
                <p className="text-xs">
                  Trừ tiền từ tài khoản thanh toán{" "}
                  <strong>•••• {user?.accountNo?.slice(-4) ?? "----"}</strong>
                </p>
                <Action
                  disabled={busy || !sourceAccountId.trim()}
                  onClick={() => void getQuote()}
                >
                  Lấy báo giá kỳ {schedule.nextInstallmentNo}
                </Action>
                {quote && (
                  <div className="space-y-2 rounded-xl bg-surface-container p-3 text-sm">
                    <p>
                      Gốc: {vnd(quote.principalAmount)} · Lãi:{" "}
                      {vnd(quote.interestAmount)}
                    </p>
                    <p className="font-bold">
                      Phải trả: {vnd(quote.totalAmount)}
                    </p>
                    <p className="text-xs">
                      Hết hạn báo giá:{" "}
                      {new Date(quote.quoteExpiresAt).toLocaleString("vi-VN")}
                    </p>
                    <Action disabled={busy} onClick={() => void confirm()}>
                      Xác nhận trả {vnd(quote.totalAmount)}
                    </Action>
                  </div>
                )}
              </Panel>
            )}
          <Panel title="Lịch sử trả nợ">
            {payments.length === 0 && (
              <p className="text-xs">Chưa có lần thanh toán thành công.</p>
            )}
            {payments.map((p) => (
              <button
                type="button"
                key={p.paymentId}
                onClick={() => {
                  if (token)
                    void run(
                      async () =>
                        setPaymentDetail(
                          await getLoanPayment(token, loanId, p.paymentId),
                        ),
                      "Đã tải biên nhận.",
                    );
                }}
                className="block w-full rounded-xl border p-3 text-left text-xs"
              >
                Kỳ {p.installmentNo} · {vnd(p.amount)} · {statusLabel(p.status)}{" "}
                · Xem biên nhận
              </button>
            ))}
            {nextPaymentCursor && (
              <Action
                secondary
                disabled={busy}
                onClick={() => {
                  if (!token) return;
                  void run(async () => {
                    const page = await listLoanPayments(
                      token,
                      loanId,
                      nextPaymentCursor,
                    );
                    setPayments((old) =>
                      appendUnique(old, page.items, (item) => item.paymentId),
                    );
                    setNextPaymentCursor(page.nextCursor);
                  }, "Đã tải thêm lịch sử.");
                }}
              >
                Xem thêm thanh toán
              </Action>
            )}
            {paymentDetail && (
              <div className="rounded-xl bg-surface-container p-3 text-xs">
                <strong>Biên nhận kỳ {paymentDetail.installmentNo}</strong>
                <p>Thanh toán: {vnd(paymentDetail.amount)}</p>
                <p>
                  Gốc: {vnd(paymentDetail.principalAmount)} · Lãi:{" "}
                  {vnd(paymentDetail.interestAmount)}
                </p>
                <p>Mã giao dịch: {paymentDetail.transactionId}</p>
                <p>
                  Thời điểm:{" "}
                  {new Date(paymentDetail.paidAt).toLocaleString("vi-VN")}
                </p>
              </div>
            )}
          </Panel>
        </>
      )}
    </LoanLayout>
  );
}
