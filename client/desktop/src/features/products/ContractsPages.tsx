import { useCallback, useEffect, useState } from "react";
import {
  explainProductError,
  getLoanContract,
  getSavingsContract,
  listLoanContracts,
  listSavingsContracts,
} from "./api";
import type {
  LoanContractDetail,
  LoanContractRow,
  SavingsContractDetail,
  SavingsContractRow,
} from "./model";
import {
  date,
  fieldInput,
  fieldLabel,
  panel,
  tableCell,
  tableHead,
  vnd,
} from "./format";
import { Banner, PageHeader, Pager } from "./ui";
import { AppliedPackage } from "./AppliedPackage";
import { CustomerSummary } from "@/features/loans/LoanReviewPage";

const SAVINGS_STATUS: Record<string, string> = {
  active: "Đang gửi",
  matured: "Đã đáo hạn",
  settled: "Đã tất toán",
  locked: "Tạm khóa",
};
const LOAN_STATUS: Record<string, string> = {
  active: "Đang vay",
  completed: "Đã tất toán",
};
const INSTALLMENT_STATUS: Record<string, string> = {
  upcoming: "Chưa đến hạn",
  due: "Đến hạn hôm nay",
  overdue: "Quá hạn",
  paid: "Đã thanh toán",
};

function usePaged<T>(
  load: (cursor?: string) => Promise<{ items: T[]; nextCursor: string | null }>,
) {
  // API chỉ có con trỏ đi tới, nên nhớ con trỏ các trang đã qua để quay lại.
  // Bộ lọc đổi sẽ tạo `load` mới, lúc đó tự về trang đầu.
  const [trail, setTrail] = useState({ load, cursors: [] as string[] });
  const cursors = trail.load === load ? trail.cursors : [];
  const current = cursors[cursors.length - 1] as string | undefined;
  const [result, setResult] = useState<{
    load: typeof load;
    cursor?: string;
    items: T[];
    next: string | null;
    error: string;
  } | null>(null);

  useEffect(() => {
    let active = true;
    load(current)
      .then((page) => {
        if (active)
          setResult({
            load,
            cursor: current,
            items: page.items,
            next: page.nextCursor,
            error: "",
          });
      })
      .catch((failure: unknown) => {
        if (active)
          setResult({
            load,
            cursor: current,
            items: [],
            next: null,
            error: explainProductError(failure),
          });
      });
    return () => {
      active = false;
    };
  }, [load, current]);

  const busy = result?.load !== load || result.cursor !== current;
  const next = result?.next ?? null;
  return {
    items: result?.items ?? [],
    busy,
    error: result?.error ?? "",
    page: cursors.length + 1,
    hasNext: Boolean(next),
    onNext: () => {
      if (next) setTrail({ load, cursors: [...cursors, next] });
    },
    onPrevious: () => setTrail({ load, cursors: cursors.slice(0, -1) }),
  };
}

const FRESHNESS =
  "Dữ liệu đọc từ bản chiếu nên có thể trễ vài giây so với giao dịch vừa ghi.";

export function SavingsContractsPage() {
  const [status, setStatus] = useState("");
  const [customerId, setCustomerId] = useState("");
  const [query, setQuery] = useState("");
  const load = useCallback(
    (cursor?: string) =>
      listSavingsContracts({ status, customerId, q: query, cursor }),
    [status, customerId, query],
  );
  const { items, busy, error, page, hasNext, onNext, onPrevious } =
    usePaged<SavingsContractRow>(load);
  const [detail, setDetail] = useState<SavingsContractDetail | null>(null);
  const [detailError, setDetailError] = useState("");

  const open = async (id: string) => {
    setDetailError("");
    try {
      setDetail(await getSavingsContract(id));
    } catch (failure) {
      setDetailError(explainProductError(failure));
    }
  };

  return (
    <div className="grid gap-3">
      <PageHeader title="Sổ tiết kiệm" description={FRESHNESS} />
      {error && <Banner message={error} error />}
      <section className={`${panel} grid gap-3 p-4`}>
        <div className="grid gap-3 md:grid-cols-3">
          <label className={fieldLabel}>
            Trạng thái
            <select
              className={fieldInput}
              value={status}
              onChange={(event) => setStatus(event.target.value)}
            >
              <option value="">Tất cả</option>
              <option value="active">Đang gửi</option>
              <option value="matured">Đã đáo hạn</option>
              <option value="settled">Đã tất toán</option>
            </select>
          </label>
          <label className={fieldLabel}>
            Mã sổ
            <input
              className={fieldInput}
              value={query}
              onChange={(event) => setQuery(event.target.value.trim())}
            />
          </label>
          <label className={fieldLabel}>
            Mã khách hàng
            <input
              className={fieldInput}
              value={customerId}
              onChange={(event) => setCustomerId(event.target.value.trim())}
            />
          </label>
        </div>
        <table className="w-full border-collapse">
          <thead>
            <tr>
              {[
                "Mã sổ",
                "Gói",
                "Tiền gốc",
                "Kỳ hạn",
                "Lãi",
                "Đáo hạn",
                "Trạng thái",
              ].map((label) => (
                <th key={label} className={tableHead}>
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((row) => (
              <tr key={row.savingsId}>
                <td className={tableCell}>
                  <button
                    type="button"
                    className="text-(--admin-primary) underline"
                    onClick={() => void open(row.savingsId)}
                  >
                    {row.savingsNo}
                  </button>
                </td>
                <td className={tableCell}>
                  {row.packageId} · v{row.packageVersion || "?"}
                </td>
                <td className={tableCell}>{vnd(row.principal)}</td>
                <td className={tableCell}>{row.termMonths} tháng</td>
                <td className={tableCell}>{row.annualInterestRate}%</td>
                <td className={tableCell}>{date(row.maturityAt)}</td>
                <td className={tableCell}>
                  {SAVINGS_STATUS[row.status] ?? "Đang cập nhật"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!busy && items.length === 0 && !error && (
          <p className="py-4 text-center text-[11px] text-(--admin-text-muted)">
            Không có sổ phù hợp.
          </p>
        )}
        <Pager
          page={page}
          hasNext={hasNext}
          disabled={busy}
          onPrevious={onPrevious}
          onNext={onNext}
        />
      </section>
      {detailError && <Banner message={detailError} error />}
      {detail && (
        <section
          className={`${panel} grid gap-2 p-4 text-[12px] text-(--admin-text)`}
        >
          <h2 className="text-[14px] font-semibold">Sổ {detail.savingsNo}</h2>
          <CustomerSummary accountId={detail.sourceAccountId} />
          <AppliedPackage
            kind="savings"
            code={detail.packageId}
            version={detail.packageVersion}
          />
          <p>
            Tiền gốc {vnd(detail.principal)} · {detail.termMonths} tháng ·{" "}
            {detail.annualInterestRate}%/năm
          </p>
          <p>
            Mở {date(detail.openedAt)} · Đáo hạn {date(detail.maturityAt)}
          </p>
          {detail.settlement && (
            <p>
              Tất toán {date(detail.settlement.settledAt)}: lãi{" "}
              {vnd(detail.settlement.interestAmount)}, tổng{" "}
              {vnd(detail.settlement.totalAmount)}
            </p>
          )}
          {detail.history.map((entry) => (
            <p key={entry.transactionId} className="text-[11px]">
              {date(entry.createdAt)} ·{" "}
              {entry.direction.toLowerCase() === "debit"
                ? "Trích tiền mở sổ"
                : "Hoàn gốc và lãi"}{" "}
              · {vnd(entry.amount)}
            </p>
          ))}
        </section>
      )}
    </div>
  );
}

export function LoanContractsAdminPage({
  overdue = false,
}: {
  overdue?: boolean;
}) {
  const [status, setStatus] = useState("");
  const [customerId, setCustomerId] = useState("");
  const load = useCallback(
    (cursor?: string) =>
      listLoanContracts({
        status: overdue ? "active" : status,
        customerId,
        overdue: overdue ? "true" : undefined,
        cursor,
      }),
    [overdue, status, customerId],
  );
  const { items, busy, error, page, hasNext, onNext, onPrevious } =
    usePaged<LoanContractRow>(load);
  const [detail, setDetail] = useState<LoanContractDetail | null>(null);
  const [detailError, setDetailError] = useState("");

  const open = async (id: string) => {
    setDetailError("");
    try {
      setDetail(await getLoanContract(id));
    } catch (failure) {
      setDetailError(explainProductError(failure));
    }
  };

  return (
    <div className="grid gap-3">
      <PageHeader
        title={overdue ? "Khoản vay quá hạn" : "Hợp đồng vay"}
        description={
          overdue
            ? `Khách vẫn trả được kỳ quá hạn trên app, theo đúng thứ tự và chưa tính phạt. ${FRESHNESS}`
            : FRESHNESS
        }
      />
      {error && <Banner message={error} error />}
      <section className={`${panel} grid gap-3 p-4`}>
        <div className="grid gap-3 md:grid-cols-2">
          {!overdue && (
            <label className={fieldLabel}>
              Trạng thái
              <select
                className={fieldInput}
                value={status}
                onChange={(event) => setStatus(event.target.value)}
              >
                <option value="">Tất cả</option>
                <option value="active">Đang vay</option>
                <option value="completed">Đã tất toán</option>
              </select>
            </label>
          )}
          <label className={fieldLabel}>
            Mã khách hàng
            <input
              className={fieldInput}
              value={customerId}
              onChange={(event) => setCustomerId(event.target.value.trim())}
            />
          </label>
        </div>
        <table className="w-full border-collapse">
          <thead>
            <tr>
              {[
                "Mã khoản vay",
                "Gói",
                "Dư nợ",
                "Đã trả",
                "Kỳ tới",
                "Quá hạn",
                "Trạng thái",
              ].map((label) => (
                <th key={label} className={tableHead}>
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((row) => (
              <tr key={row.loanId}>
                <td className={tableCell}>
                  <button
                    type="button"
                    className="max-w-40 truncate text-(--admin-primary) underline"
                    onClick={() => void open(row.loanId)}
                  >
                    {row.loanNo}
                  </button>
                </td>
                <td className={tableCell}>
                  {row.loanPackageId} · v{row.loanPackageVersion || "?"}
                </td>
                <td className={tableCell}>{vnd(row.outstandingPrincipal)}</td>
                <td className={tableCell}>
                  {row.paidInstallments}/{row.totalInstallments} kỳ
                </td>
                <td className={tableCell}>
                  {row.nextDueDate
                    ? `${date(row.nextDueDate)} · ${vnd(row.nextDueAmount)}`
                    : "—"}
                </td>
                <td className={tableCell}>
                  {row.overdueInstallments > 0
                    ? `${row.overdueInstallments} kỳ, từ ${date(row.oldestOverdueDate)}`
                    : "—"}
                </td>
                <td className={tableCell}>
                  {LOAN_STATUS[row.status] ?? "Đang cập nhật"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!busy && items.length === 0 && !error && (
          <p className="py-4 text-center text-[11px] text-(--admin-text-muted)">
            {overdue
              ? "Không có khoản vay quá hạn."
              : "Chưa có hợp đồng phù hợp."}
          </p>
        )}
        <Pager
          page={page}
          hasNext={hasNext}
          disabled={busy}
          onPrevious={onPrevious}
          onNext={onNext}
        />
      </section>
      {detailError && <Banner message={detailError} error />}
      {detail && (
        <section
          className={`${panel} grid gap-2 p-4 text-[12px] text-(--admin-text)`}
        >
          <h2 className="text-[14px] font-semibold">
            Khoản vay {detail.loanNo}
          </h2>
          <CustomerSummary accountId={detail.customerId} />
          <AppliedPackage
            kind="loan"
            code={detail.loanPackageId}
            version={detail.loanPackageVersion}
          />
          <p>
            Gốc {vnd(detail.principal)} · Dư nợ{" "}
            {vnd(detail.outstandingPrincipal)} · {detail.annualInterestRate}
            %/năm
          </p>
          <table className="w-full border-collapse">
            <thead>
              <tr>
                {["Kỳ", "Hạn", "Phải trả", "Trạng thái"].map((label) => (
                  <th key={label} className={tableHead}>
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {detail.repaymentSchedule.map((item) => (
                <tr key={item.installmentNo}>
                  <td className={tableCell}>{item.installmentNo}</td>
                  <td className={tableCell}>{date(item.dueDate)}</td>
                  <td className={tableCell}>{vnd(item.totalDue)}</td>
                  <td className={tableCell}>
                    {INSTALLMENT_STATUS[item.status] ?? "Đang cập nhật"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {detail.payments.map((payment) => (
            <p key={payment.paymentId} className="text-[11px]">
              {date(payment.paidAt)} · Kỳ {payment.installmentNo} ·{" "}
              {vnd(payment.amount)}
            </p>
          ))}
        </section>
      )}
    </div>
  );
}
