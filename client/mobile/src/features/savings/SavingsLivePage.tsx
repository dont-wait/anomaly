import { useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { AuthContext } from "@/features/auth/authContext";
import { ApiError } from "@/shared/lib/http";
import { getInfo, type AuthUser } from "@/features/auth/api/profile";
import { ProfileSidebar } from "@/shared/layout/ProfileSidebar";
import { navigate, routes } from "@/app/routes";
import { ArrowLeftIcon, ChevronRightIcon, SavingsIcon } from "@/shared/icons";
import { BottomNav } from "@/shared/layout/BottomNav";
import { Logo } from "@/shared/layout/Logo";
import { Pagination } from "@/shared/ui";
import {
  createSavingsQuote, createSavingsWithdrawalQuote, describeSavingsError,
  getSavingsContract, getSavingsHistory, getSavingsReceipt, getSavingsTerms, listSavingsContracts, listSavingsPackages,
  mayBeCommitted, openSavingsContract, withdrawSavings,
  type SavingsOpenResult, type SavingsPackage, type SavingsQuote,
  type SavingsHistoryItem, type SavingsRecord, type SavingsReceipt, type SavingsTermsSnapshot, type SavingsWithdrawalQuote,
  type SavingsWithdrawResult,
} from "./api";
import { clearPending, loadPending, savePending, type PendingSavingsCommand } from "./pending";
import {
  formatDate,
  formatVnd,
  parseAmount,
} from "./utils/format";
import type { SavingsContract, SavingsDraft, SavingsStatus } from "./model";
import "./savings.css";

type Screen =
  | "list"
  | "open"
  | "open-preview"
  | "open-success"
  | "detail"
  | "withdraw-preview"
  | "withdraw-success";
type Filter = "all" | SavingsStatus;

const labels: Record<SavingsStatus, string> = {
  active: "Đang gửi",
  matured: "Đã đáo hạn",
  settled: "Đã tất toán",
  locked: "Tạm khóa",
};

function Button({
  children,
  onClick,
  secondary = false,
  disabled = false,
}: {
  children: ReactNode;
  onClick: () => void;
  secondary?: boolean;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className={`savings-button ${secondary ? "savings-button-secondary" : ""}`}
    >
      {children}
    </button>
  );
}

function Section({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return <section className={`savings-panel ${className}`}>{children}</section>;
}

function Line({
  label,
  value,
  strong = false,
}: {
  label: string;
  value: ReactNode;
  strong?: boolean;
}) {
  return (
    <div className={`savings-line ${strong ? "savings-line-strong" : ""}`}>
      <span>{label}</span>
      <span>{value}</span>
    </div>
  );
}

function Badge({ status }: { status: SavingsStatus }) {
  return (
    <span className={`savings-status savings-status-${status}`}>
      {labels[status]}
    </span>
  );
}

function Head({
  title,
  onBack,
  subtitle,
}: {
  title: string;
  onBack: () => void;
  subtitle?: string;
}) {
  return (
    <header className="savings-topbar">
      <button
        type="button"
        className="savings-back"
        onClick={onBack}
        aria-label="Quay lại"
      >
        <ArrowLeftIcon className="h-5 w-5" />
      </button>
      <div className="savings-topbar-copy">
        <h1>{title}</h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
      <Logo className="savings-logo" />
    </header>
  );
}

function SuccessMark() {
  return (
    <div className="savings-success-mark" aria-hidden="true">
      <span>✓</span>
    </div>
  );
}

/** Never expose the internal account UUID as a bank account number in the UI. */
function paymentAccountLabel(accountId: string, currentId: string, accountNo?: string): string {
  if (accountId === currentId && accountNo) return `•••• ${accountNo.replace(/\s/g, "").slice(-4)}`;
  return "Tài khoản thanh toán đã xác thực";
}

function TermsDialog({ terms, onClose, onReviewed }: {
  terms: SavingsTermsSnapshot;
  onClose: () => void;
  onReviewed: () => void;
}) {
  return (
    <div className="savings-terms-overlay">
      <section
        role="dialog"
        aria-modal="true"
        aria-labelledby="savings-terms-title"
        className="savings-terms-dialog"
        onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}
      >
        <div className="savings-terms-header">
          <h2 id="savings-terms-title">{terms.title}</h2>
          <button type="button" aria-label="Đóng điều khoản" onClick={onClose}>×</button>
        </div>
        <div className="savings-terms-body">
          <p>{terms.introduction}</p>
          {terms.sections.map((part) => (
            <section key={part.title} className="savings-terms-part">
              <h3>{part.title}</h3>
              <p>{part.body}</p>
            </section>
          ))}
        </div>
        <div className="savings-terms-footer">
          <button type="button" className="savings-button" onClick={onReviewed}>Tôi đã đọc và hiểu</button>
          <button type="button" className="savings-button savings-button-secondary" onClick={onClose}>Đóng</button>
        </div>
      </section>
    </div>
  );
}

const SAVINGS_PAGE_SIZE = 5;

/** Convert the authoritative server record into the Stitch view model. */
function mapSavingsRecord(record: SavingsRecord): SavingsContract {
  const rate = Number(record.annualInterestRate);
  // Preview amounts on an un-settled contract are estimates only, never payout authority.
  const days = (Date.parse(record.maturityAt) - Date.parse(record.openedAt)) / 86_400_000;
  const estimatedInterest = Math.round(record.principal * rate * days / 36_500);
  return {
    id: record.savingsId, number: record.savingsNo, principal: record.principal,
    termMonths: record.termMonths, annualRate: rate, openedAt: record.openedAt,
    maturityAt: record.maturityAt, status: record.status,
    sourceAccount: record.sourceAccountId,
    interest: record.settlement?.interestAmount ?? estimatedInterest,
    settledAt: record.settlement?.settledAt,
  };
}

export default function SavingsLivePage() {
  const auth = useContext(AuthContext);
  const token = auth?.token ?? "";
  const actor = auth?.user?.id ?? "";
  const [profileOpen, setProfileOpen] = useState(false);
  const closeProfile = useCallback(() => setProfileOpen(false), []);
  const [screen, setScreen] = useState<Screen>("list");
  const [contracts, setContracts] = useState<SavingsContract[]>([]);
  const [packages, setPackages] = useState<SavingsPackage[]>([]);
  const [account, setAccount] = useState<AuthUser | null>(null);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const commandInFlight = useRef(false);
  const activeContext = useRef({ token, actor, selectedId: "" });
  const commandsGeneration = useRef(0);
  const latestWrite = useRef<{ id: string; status: SavingsStatus } | null>(null);
  const [summaryStale, setSummaryStale] = useState(false);

  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [pending, setPending] = useState<PendingSavingsCommand | null>(() => {
    try { return actor ? loadPending(actor) : null; } catch { return null; }
  });
  const [storageBlocked, setStorageBlocked] = useState(false);
  const [filter, setFilter] = useState<Filter>("all");
  const [savingsPage, setSavingsPage] = useState(1);
  const [selectedId, setSelectedId] = useState("");
  useEffect(() => { activeContext.current = { token, actor, selectedId }; }, [token, actor, selectedId]);
  const [amountText, setAmountText] = useState("10000000");
  const [termMonths, setTermMonths] = useState(12);
  const [accepted, setAccepted] = useState(false);
  const [draft, setDraft] = useState<SavingsDraft | null>(null);
  const [openQuote, setOpenQuote] = useState<SavingsQuote | null>(null);
  const [productTerms, setProductTerms] = useState<SavingsTermsSnapshot | null>(null);
  const [termsVisible, setTermsVisible] = useState(false);
  const [termsReviewed, setTermsReviewed] = useState(false);
  const [history, setHistory] = useState<SavingsHistoryItem[] | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState<string | null>(null);
  const [withdrawQuote, setWithdrawQuote] = useState<SavingsWithdrawalQuote | null>(null);
  const [receipt, setReceipt] = useState<SavingsReceipt | null>(null);
  const [lastWithdrawal, setLastWithdrawal] = useState<SavingsWithdrawResult | null>(null);
  const [withdrawAccepted, setWithdrawAccepted] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const refreshContracts = useCallback(async (replace = true, cursor?: string) => {
    if (!token || !actor) return;
    const generation = commandsGeneration.current;
    const page = await listSavingsContracts(token, cursor);
    if (activeContext.current.token !== token || activeContext.current.actor !== actor) return;
    const stale = commandsGeneration.current !== generation;
    setContracts((current) => {
      const incoming = (page.items ?? []).map(mapSavingsRecord);
      if (replace) {
        // Never erase a more recent successful write while the read model catches up.
        if (stale) {
          return [...current, ...incoming.filter((c) => !current.some((v) => v.id === c.id))];
        }
        return incoming;
      }
      return [...current, ...incoming.filter((c) => !current.some((v) => v.id === c.id))];
    });
    if (commandsGeneration.current === generation) setNextCursor(page.nextCursor);
    if (replace && !page.nextCursor && latestWrite.current &&
      (page.items ?? []).some((item) => item.savingsId === latestWrite.current?.id && item.status === latestWrite.current.status)) {
      setSummaryStale(false);
    }
    if (replace) setSavingsPage(1);
  }, [token, actor]);

  useEffect(() => {
    if (!token || !actor) return;
    const controller = new AbortController();
    const generation = commandsGeneration.current;
    // Independently display successful sections even if one API fails.
    void Promise.allSettled([
      getInfo(token, { signal: controller.signal }).then((profile) => {
        if (!controller.signal.aborted) setAccount(profile);
      }),
      listSavingsPackages(token, controller.signal).then((catalog) => {
        if (!controller.signal.aborted) setPackages(catalog.items ?? []);
      }),
      listSavingsContracts(token, undefined, controller.signal).then((page) => {
        if (controller.signal.aborted) return;
        const latest = commandsGeneration.current === generation;
        setContracts((current) => {
          const incoming = (page.items ?? []).map(mapSavingsRecord);
          return latest ? incoming :
            [...current, ...incoming.filter((c) => !current.some((v) => v.id === c.id))];
        });
        if (commandsGeneration.current === generation) setNextCursor(page.nextCursor);
      }),
    ]).then((results) => {
      if (controller.signal.aborted) return;
      const failed = results.find((result) => result.status === "rejected");
      setErrorMessage(failed?.status === "rejected" ? describeSavingsError(failed.reason) : null);
      setLoading(false);
    });
    void Promise.resolve().then(() => {
      if (controller.signal.aborted) return;
      try { setPending(loadPending(actor)); setStorageBlocked(false); }
      catch {
        setStorageBlocked(true);
        setErrorMessage("Không thể kiểm tra yêu cầu giao dịch trước đó. Tạm khóa giao dịch để tránh trùng tiền.");
      }
    });
    return () => controller.abort();
  }, [token, actor]);

  const selected = contracts.find((c) => c.id === selectedId) ?? null;
  const summaryIncomplete = Boolean(nextCursor) || summaryStale;
  const principalTotal = contracts.filter((c) => c.status !== "settled")
    .reduce((sum, c) => sum + c.principal, 0);
  const projectedInterest = contracts.filter((c) => c.status !== "settled")
    .reduce((sum, c) => sum + c.interest, 0);
  const count = (status: Filter) =>
    contracts.filter((c) => status === "all" || c.status === status).length;
  const visible = contracts.filter((c) => filter === "all" || c.status === filter);
  const amount = parseAmount(amountText);
  const savingsPackage = packages.find((item) => item.packageId === "savings_standard");
  const terms = savingsPackage?.termOptions ?? [];
  const chosenTerm = terms.find((option) => option.termMonths === termMonths);
  // This is a visual estimate only. The final amount comes from the server quote.
  const estimatedInterest = chosenTerm && Number.isSafeInteger(amount) && amount > 0
    ? Math.round(amount * Number(chosenTerm.annualInterestRate) * termMonths / 1_200)
    : null;
  const minDeposit = savingsPackage?.minimumDeposit ?? 1_000_000;
  const balance = account?.amount ?? 0;
  const sourceAccountId = account?.id ?? "";
  const sourceLabel = paymentAccountLabel(sourceAccountId, sourceAccountId, account?.accountNo);
  const canOpen = Boolean(savingsPackage && sourceAccountId) && Number.isSafeInteger(amount) &&
    amount >= minDeposit && amount <= balance &&
    terms.some((option) => option.termMonths === termMonths) &&
    !busy && !pending && !storageBlocked;

  const showContract = async (id: string) => {
    if (!token || busy) return;
    setBusy(true);
    setErrorMessage(null);
    try {
      const detail = await getSavingsContract(token, id);
      if (activeContext.current.token !== token || activeContext.current.actor !== actor) return;
      setContracts((current) => current.map((item) => item.id === id ? mapSavingsRecord(detail) : item));
      setSelectedId(id);
      setReceipt(null);
      setHistoryLoading(false);
      setHistory(null);
      setHistoryError(null);
      setScreen("detail");
    } catch (error) { setErrorMessage(describeSavingsError(error)); }
    finally { setBusy(false); }
  };
  const startOpen = () => {
    if (pending || storageBlocked) { setErrorMessage("Không thể tạo yêu cầu mới khi còn yêu cầu giao dịch chưa xác định hoặc lưu trữ không khả dụng."); return; }
    setAccepted(false);
    setProductTerms(null);
    setTermsVisible(false);
    setTermsReviewed(false);
    setFormError(null);
    setErrorMessage(null);
    setOpenQuote(null);
    setScreen("open");
  };
  const previewOpen = async (months: number) => {
    if (!token || !savingsPackage || !sourceAccountId || !canOpen) return;
    setBusy(true);
    setFormError(null);
    try {
      // Load the server-owned product conditions before starting the 5-minute quote TTL.
      const conditions = await getSavingsTerms(token);
      if (!conditions.version || !conditions.sections?.length) throw new Error("Điều kiện tiết kiệm chưa sẵn sàng.");
      const quote = await createSavingsQuote(token, {
        packageId: savingsPackage.packageId, sourceAccountId,
        amount, termMonths: months,
      });
      setProductTerms(conditions);
      setTermsVisible(false);
      setTermsReviewed(false);
      setOpenQuote(quote);
      setDraft({
        amount: quote.principal, termMonths: quote.termMonths,
        annualRate: Number(quote.annualInterestRate),
        openedAt: new Date().toISOString(),
        maturityAt: quote.expectedMaturityDate,
        interest: quote.estimatedInterest,
      });
      setAccepted(false);
      setScreen("open-preview");
    } catch (error) { setFormError(describeSavingsError(error)); }
    finally { setBusy(false); }
  };

  const keepPending = (operation: PendingSavingsCommand): boolean => {
    try {
      savePending(actor, operation);
      setPending(operation);
      return true;
    } catch {
      setStorageBlocked(true);
      setErrorMessage("Không thể lưu an toàn yêu cầu giao dịch. Không thực hiện chuyển tiền.");
      return false;
    }
  };
  const clearConfirmed = () => {
    try {
      clearPending(actor);
      setPending(null);
      setErrorMessage(null);
    } catch {
      // Success is authoritative, but a stale pending key must not enable another write.
      setStorageBlocked(true);
      setErrorMessage("Giao dịch đã hoàn thành. Bộ nhớ thiết bị chưa thể đồng bộ; không tạo yêu cầu mới.");
    }
  };
  const applyOpened = (result: SavingsOpenResult) => {
    const rate = Number(result.annualInterestRate);
    const days = (Date.parse(result.maturityAt) - Date.parse(result.openedAt)) / 86_400_000;
    const estimated = Math.round(result.principal * rate * days / 36_500);
    const item: SavingsContract = {
      id: result.savingsId, number: result.savingsNo,
      principal: result.principal, termMonths: result.termMonths,
      annualRate: rate, openedAt: result.openedAt, maturityAt: result.maturityAt,
      sourceAccount: result.sourceAccountId, status: result.status,
      interest: estimated,
    };
    commandsGeneration.current += 1;
    latestWrite.current = { id: result.savingsId, status: result.status };
    setSummaryStale(true);
    setNextCursor(null); // Old cursors are invalid after a bank write.
    setContracts((current) => [item, ...current.filter((c) => c.id !== item.id)]);
    setSelectedId(item.id);
    clearConfirmed();
    setScreen("open-success");
    void getInfo(token).then((profile) => {
      if (activeContext.current.token === token && activeContext.current.actor === actor) setAccount(profile);
    }).catch(() => {});
  };
  const applyWithdrawn = (result: SavingsWithdrawResult) => {
    commandsGeneration.current += 1;
    latestWrite.current = { id: result.savingsId, status: "settled" };
    setSummaryStale(true);
    setNextCursor(null);
    setLastWithdrawal(result);
    setContracts((current) => current.map((item) => item.id === result.savingsId ? {
      ...item, status: "settled" as const, interest: result.interestAmount,
      settledAt: result.settledAt,
    } : item));
    setSelectedId(result.savingsId);
    clearConfirmed();
    setScreen("withdraw-success");
    void getInfo(token).then((profile) => {
      if (activeContext.current.token === token && activeContext.current.actor === actor) setAccount(profile);
    }).catch(() => {});
  };
  const runOperation = async (operation: PendingSavingsCommand) => {
    if (commandInFlight.current || !token) return;
    commandInFlight.current = true;
    setBusy(true);
    setErrorMessage(null);
    if (!keepPending(operation)) {
      setBusy(false);
      commandInFlight.current = false;
      return;
    }
    try {
      if (operation.kind === "open") {
        const result = await openSavingsContract(token, {
          quoteId: operation.quoteId, sourceAccountId: operation.sourceAccountId,
          acceptedTermsVersion: operation.acceptedTermsVersion, confirm: true,
        }, operation.key);
        applyOpened(result);
      } else {
        const result = await withdrawSavings(token, operation.savingsId, operation.quoteId, operation.key);
        applyWithdrawn(result);
      }
    } catch (error) {
      setErrorMessage(describeSavingsError(error));
      if (error instanceof ApiError && error.hasCode("SAVINGS_TERMS_CHANGED")) {
        setProductTerms(null);
        setTermsReviewed(false);
        setAccepted(false);
        setOpenQuote(null);
        setScreen("open");
      }
      if (!mayBeCommitted(error)) {
        try {
          clearPending(actor);
          setPending(null);
        } catch { setStorageBlocked(true); }
      }
    } finally { setBusy(false); commandInFlight.current = false; }
  };
  const confirmOpen = () => {
    if (!termsReviewed || !productTerms || !accepted || !openQuote || !sourceAccountId || pending || storageBlocked) return;
    void runOperation({ kind: "open", key: crypto.randomUUID(), quoteId: openQuote.quoteId,
      sourceAccountId, acceptedTermsVersion: productTerms.version });
  };
  const beginWithdraw = async () => {
    if (!selected || selected.status !== "matured" || pending || busy || !token) return;
    setBusy(true);
    setErrorMessage(null);
    try {
      // Only the Backend may authorize maturity and compute the payout.
      const quote = await createSavingsWithdrawalQuote(token, selected.id);
      setWithdrawQuote(quote);
      setWithdrawAccepted(false);
      setScreen("withdraw-preview");
    } catch (error) { setErrorMessage(describeSavingsError(error)); }
    finally { setBusy(false); }
  };
  const confirmWithdraw = () => {
    if (!withdrawAccepted || !selected || selected.status !== "matured" || !withdrawQuote || pending || storageBlocked) return;
    void runOperation({ kind: "withdraw", key: crypto.randomUUID(), quoteId: withdrawQuote.quoteId, savingsId: selected.id });
  };
  const loadHistory = async () => {
    if (!selected || !token || historyLoading) return;
    const requestedId = selected.id;
    const requestedActor = actor;
    setHistoryLoading(true);
    setHistoryError(null);
    try {
      const response = await getSavingsHistory(token, requestedId);
      if (activeContext.current.token === token && activeContext.current.actor === requestedActor &&
        activeContext.current.selectedId === requestedId) setHistory(response.items ?? []);
    } catch (error) {
      if (activeContext.current.selectedId === requestedId) setHistoryError(describeSavingsError(error));
    } finally { if (activeContext.current.selectedId === requestedId) setHistoryLoading(false); }
  };
  const showReceipt = async () => {
    const id = lastWithdrawal?.savingsId ?? selected?.id;
    if (!id || !token || busy) return;
    setBusy(true);
    setErrorMessage(null);
    try {
      const [data, detail] = await Promise.all([
        getSavingsReceipt(token, id), getSavingsContract(token, id),
      ]);
      setContracts((current) => [mapSavingsRecord(detail), ...current.filter((item) => item.id !== id)]);
      setSelectedId(id);
      setReceipt(data);
      setScreen("detail");
    } catch (error) { setErrorMessage(describeSavingsError(error)); }
    finally { setBusy(false); }
  };
  return (
    <div className="savings-shell" aria-busy={busy}>
      {(errorMessage || !auth || !token) && <div className="savings-live-banner" role="alert">{errorMessage || "Vui lòng đăng nhập để xem sổ tiết kiệm thật."}</div>}
      {busy && <div className="savings-live-banner" role="status">Đang xử lý yêu cầu với ngân hàng…</div>}
      {pending && <div className="savings-live-banner" role="status">Yêu cầu giao dịch gần nhất đang được xác minh. Vui lòng không tạo giao dịch mới. <button type="button" disabled={busy} onClick={() => void runOperation(pending)}>Kiểm tra lại yêu cầu</button></div>}
      {storageBlocked && <div className="savings-live-banner" role="alert">Lưu trữ giao dịch không khả dụng. Không thể thực hiện giao dịch mới trên thiết bị này.</div>}
      {loading && <div className="savings-live-banner" role="status">Đang tải thông tin tiết kiệm…</div>}
      {screen === "list" ? (
        <>
          <Head
            title="Tiết kiệm của tôi"
            subtitle="Tích lũy hôm nay, vững vàng tương lai"
            onBack={() => navigate(routes.dashboard)}
          />
          <main className="savings-content">
            <section className="savings-hero">
              <div className="savings-hero-top">
                <span className="savings-hero-eyebrow">
                  <SavingsIcon className="h-5 w-5" /> Tiết kiệm có kỳ hạn
                </span>
                <span className="savings-hero-pill">◈ Chuẩn VND</span>
              </div>
              <strong className="savings-hero-amount">
                {summaryIncomplete ? "—" : formatVnd(principalTotal)}
              </strong>
              <p>
                {summaryIncomplete ? "Danh sách cần đồng bộ để tổng hợp đầy đủ" :
                  `${count("active") + count("matured") + count("locked")} sổ tiết kiệm chưa tất toán`}
              </p>
              <div className="savings-hero-metrics">
                <span>
                  Đang gửi <b>{summaryIncomplete ? "—" : `${count("active")} sổ`}</b>
                </span>
                <span>
                  Lãi ước tính (tham khảo) <b>{summaryIncomplete ? "—" : `+${formatVnd(projectedInterest)}`}</b>
                </span>
              </div>
            </section>
            <Button onClick={startOpen} disabled={loading || busy || !savingsPackage || !account || !!pending}>&nbsp; Gửi tiết kiệm mới</Button>
            <Button secondary disabled={busy || loading} onClick={() => { setBusy(true); void refreshContracts().catch((error: unknown) => setErrorMessage(describeSavingsError(error))).finally(() => setBusy(false)); }}>Làm mới danh sách sổ</Button>
            <div className="savings-section-title">
              <h2>Sổ tiết kiệm của bạn</h2>
              <span>{contracts.length} hợp đồng {nextCursor ? "(đã tải, còn trang sau)" : ""}</span>
            </div>
            <div className="savings-filters" aria-label="Lọc sổ tiết kiệm">
              {(["all", "active", "matured", "locked", "settled"] as const).map((key) => (
                <button
                  key={key}
                  type="button"
                  aria-pressed={filter === key}
                  onClick={() => {
                    setFilter(key);
                    setSavingsPage(1);
                  }}
                  className={filter === key ? "savings-filter-active" : ""}
                >
                  {key === "all" ? "Tất cả" : labels[key]} ({count(key)})
                </button>
              ))}
            </div>
            <div className="savings-contracts">
              {!loading && visible.length === 0 && (
                <Section>
                  <p className="savings-muted">
                    Chưa có sổ tiết kiệm nào ở trạng thái này.
                  </p>
                </Section>
              )}
              {visible
                .slice(
                  (savingsPage - 1) * SAVINGS_PAGE_SIZE,
                  savingsPage * SAVINGS_PAGE_SIZE,
                )
                .map((item) => (
                  <button
                    type="button"
                    key={item.id}
                    className="savings-contract"
                    onClick={() => showContract(item.id)}
                  >
                    <div className="savings-contract-head">
                      <span>#{item.number}</span>
                      <Badge status={item.status} />
                    </div>
                    <span className="savings-muted">Tiền gốc gửi</span>
                    <div className="savings-contract-amount">
                      {formatVnd(item.principal)}
                    </div>
                    <div className="savings-contract-meta">
                      <span>
                        Lãi suất <b>{item.annualRate}%/năm</b>
                      </span>
                      <span>
                        Kỳ hạn <b>{item.termMonths} tháng</b>
                      </span>
                    </div>
                    <div className="savings-progress">
                      <span
                        style={{
                          width: item.status === "active" ? "35%" : "100%",
                        }}
                      />
                    </div>
                    <div className="savings-contract-dates">
                      <span>
                        Ngày mở <b>{formatDate(item.openedAt)}</b>
                      </span>
                      <span>
                        Ngày đáo hạn <b>{formatDate(item.maturityAt)}</b>
                      </span>
                    </div>
                    <div className="savings-card-action">
                      {item.status === "matured"
                        ? "Sẵn sàng rút gốc & lãi"
                        : "Xem chi tiết sổ"}
                      <ChevronRightIcon className="h-4 w-4" />
                    </div>
                  </button>
                ))}
            </div>
            {nextCursor && <Button secondary disabled={busy} onClick={() => { setBusy(true); void refreshContracts(false, nextCursor).catch((error: unknown) => setErrorMessage(describeSavingsError(error))).finally(() => setBusy(false)); }}>Tải thêm sổ tiết kiệm</Button>}
            <Pagination
              page={savingsPage}
              pageCount={Math.ceil(visible.length / SAVINGS_PAGE_SIZE)}
              onPrevious={() => setSavingsPage((value) => value - 1)}
              onNext={() => setSavingsPage((value) => value + 1)}
            />
          </main>
          <BottomNav
            active={profileOpen ? "profile" : "savings"}
            onChange={(id) => {
              if (id === "home") navigate(routes.dashboard);
              if (id === "profile" && auth) setProfileOpen(true);
            }}
          />
          {auth && (
            <ProfileSidebar
              open={profileOpen}
              name={auth.user?.fullName || auth.user?.username || ""}
              token={auth.token}
              onClose={closeProfile}
              onLogout={() => {
                closeProfile();
                auth.logout();
              }}
            />
          )}
        </>
      ) : null}

      {screen === "open" && (
        <>
          <Head title="Mở sổ tiết kiệm" onBack={() => setScreen("list")} />
          <main className="savings-content savings-open-content">
            <section className="savings-banner" aria-label="Sản phẩm tiết kiệm">
              <span className="savings-overline">TÍCH LŨY</span>
              <h2>Tiết kiệm có kỳ hạn</h2>
              <p>Lãi suất cố định, an tâm với kế hoạch tương lai.</p>
            </section>

            <div className="savings-open-group">
              <div className="savings-section-title">
                <h2>Tài khoản trích tiền</h2>
                <span>Khả dụng</span>
              </div>
              <Section>
                <Line label="Tài khoản thanh toán" value={sourceLabel} />
                <Line label="Số dư thực tế" value={formatVnd(balance)} />
              </Section>
            </div>

            <div className="savings-open-group">
              <div className="savings-section-title">
                <h2>Số tiền gửi</h2>
                <span>Từ {formatVnd(minDeposit)}</span>
              </div>
              <Section>
                <label className="savings-open-field-label" htmlFor="savings-live-amount">
                  Nhập số tiền (VND)
                </label>
                <input
                  id="savings-live-amount"
                  className="savings-amount-input"
                  type="text"
                  inputMode="numeric"
                  value={amountText}
                  onChange={(event) => {
                    setAmountText(event.target.value.replace(/\D/g, ""));
                    setFormError(null);
                  }}
                />
                <div className="savings-amount-chips">
                  {[1_000_000, 5_000_000, 10_000_000].map((value) => (
                    <button
                      type="button"
                      key={value}
                      onClick={() => {
                        setAmountText(String(value));
                        setFormError(null);
                      }}
                    >
                      {formatVnd(value)}
                    </button>
                  ))}
                </div>
                {amount > balance && <p role="alert" className="savings-error">Số tiền vượt số dư khả dụng.</p>}
              </Section>
            </div>

            <div className="savings-open-group">
              <div className="savings-section-title">
                <h2>Chọn kỳ hạn</h2>
                <span>Lãi suất cố định</span>
              </div>
              <Section>
                <label className="savings-open-field-label" htmlFor="savings-live-term">Kỳ hạn gửi</label>
                <select
                  id="savings-live-term"
                  value={chosenTerm ? String(termMonths) : ""}
                  onChange={(event) => {
                    setTermMonths(Number(event.target.value));
                    setFormError(null);
                  }}
                >
                  {!chosenTerm && <option value="" disabled>Chọn kỳ hạn</option>}
                  {terms.map((option) => (
                    <option key={option.termMonths} value={option.termMonths}>
                      {option.termMonths} tháng – {option.annualInterestRate}%/năm
                    </option>
                  ))}
                </select>
                {terms.length === 0 && <p role="status" className="savings-hint">Chưa có kỳ hạn gửi khả dụng.</p>}
              </Section>
            </div>

            <Section className="savings-summary">
              <h3>Ước tính lợi nhuận</h3>
              <Line label="Tiền gốc" value={Number.isSafeInteger(amount) && amount > 0 ? formatVnd(amount) : "—"} />
              <Line label="Lãi suất năm" value={chosenTerm ? `${chosenTerm.annualInterestRate}%` : "—"} />
              <Line label="Tiền lãi dự kiến" value={estimatedInterest === null ? "—" : formatVnd(estimatedInterest)} />
              <p className="savings-open-estimate-note">Số tiền tham khảo. Lãi thực tế được xác định tại bước xác nhận.</p>
            </Section>

            {formError && <p role="alert" className="savings-error">{formError}</p>}
            <Button disabled={!canOpen} onClick={() => void previewOpen(termMonths)}>Tiếp tục →</Button>
          </main>
        </>
      )}

      {screen === "open-preview" && draft && (
        <>
          <Head
            title="Xác nhận gửi tiết kiệm"
            onBack={() => setScreen("open")}
          />
          <main className="savings-content">
            <section className="savings-amount-hero">
              <span>Số tiền gửi</span>
              <strong>{formatVnd(draft.amount)}</strong>
              <small>Tài khoản nguồn: {sourceLabel}</small>
            </section>
            <Section>
              <h3>Thông tin chi tiết</h3>
              <Line label="Sản phẩm" value="Tiết kiệm có kỳ hạn" />
              <Line label="Thời hạn" value={`${draft.termMonths} tháng`} />
              <Line
                label="Lãi suất cố định"
                value={`${draft.annualRate}%/năm`}
              />
              <Line label="Ngày bắt đầu" value={formatDate(draft.openedAt)} />
              <Line
                label="Ngày đáo hạn dự kiến"
                value={formatDate(draft.maturityAt)}
              />
              <Line
                label="Tiền lãi dự kiến"
                value={`+${formatVnd(draft.interest)}`}
              />
              <Line
                label="Tổng nhận khi đáo hạn"
                strong
                value={formatVnd(draft.amount + draft.interest)}
              />
            </Section>
            <Section className="savings-info">
              <h3>Lưu ý quan trọng</h3>
              <p>
                Tiền lãi hiển thị là số tiền dự kiến theo kỳ hạn đã chọn.
                Sổ được tất toán toàn bộ từ ngày đáo hạn; không rút trước hạn hoặc rút một phần. Thông tin dự tính có hiệu lực trong 5 phút.
              </p>
            </Section>
            <Section className="savings-info">
              <h3>Phiếu đăng ký và điều kiện sản phẩm</h3>
              <Line label="Chủ tài khoản" value={account?.fullName || account?.username || "Đã xác thực"} />
              <Line label="Tài khoản trích tiền" value={sourceLabel} />
              <Line label="Số tiền đăng ký" value={formatVnd(draft.amount)} />
              <Line label="Kỳ hạn và lãi suất" value={`${draft.termMonths} tháng · ${draft.annualRate}%/năm`} />
              <p>Vui lòng kiểm tra thông tin đăng ký. Hợp đồng tiết kiệm được ghi nhận khi giao dịch mở sổ hoàn tất.</p>
              <button type="button" className="savings-terms-link" disabled={!productTerms} onClick={() => setTermsVisible(true)}>
                Xem phiếu đăng ký & điều khoản tiết kiệm
              </button>
              {termsReviewed && <p role="status">Đã đọc điều kiện gửi tiết kiệm.</p>}
            </Section>
            <label className="savings-checkbox">
              <input
                type="checkbox"
                checked={accepted}
                disabled={!termsReviewed || !productTerms}
                onChange={(e) => setAccepted(e.target.checked)}
              />{" "}
              Tôi đã xem và đồng ý với điều kiện sản phẩm và thông tin gửi tiết kiệm ở trên.
            </label>
            <Button onClick={confirmOpen} disabled={!accepted || !termsReviewed || !productTerms || busy || !!pending}>
              Xác nhận gửi {formatVnd(draft.amount)}
            </Button>
          </main>
          {termsVisible && productTerms && (
            <TermsDialog terms={productTerms} onClose={() => setTermsVisible(false)} onReviewed={() => {
              setTermsReviewed(true);
              setTermsVisible(false);
            }} />
          )}
        </>
      )}

      {screen === "open-success" && selected && (
        <>
          <Head
            title="Kết quả gửi tiết kiệm"
            onBack={() => setScreen("list")}
          />
          <main className="savings-content">
            <SuccessMark />
            <h2 className="savings-success-title">Gửi tiết kiệm thành công!</h2>
            <p className="savings-center savings-muted">
              Sổ tiết kiệm đã được mở thành công. Thông tin hợp đồng được cập nhật trong danh sách sổ của bạn.
            </p>
            <section className="savings-amount-hero">
              <span>Số tiền gửi</span>
              <strong>{formatVnd(selected.principal)}</strong>
            </section>
            <Section>
              <h3>Thông tin sổ tiết kiệm</h3>
              <Line label="Mã sổ" value={selected.number} />
              <Line label="Tiền gốc" value={formatVnd(selected.principal)} />
              <Line label="Kỳ hạn" value={`${selected.termMonths} tháng`} />
              <Line label="Lãi suất" value={`${selected.annualRate}%/năm`} />
              <Line label="Đáo hạn" value={formatDate(selected.maturityAt)} />
              <Line
                label="Dự kiến nhận"
                strong
                value={formatVnd(selected.principal + selected.interest)}
              />
            </Section>
            <Button onClick={() => setScreen("detail")}>
              Xem sổ tiết kiệm
            </Button>
            <Button secondary onClick={() => setScreen("list")}>
              Về trang tiết kiệm
            </Button>
          </main>
        </>
      )}

      {screen === "detail" && selected && (
        <>
          <Head
            title="Chi tiết sổ tiết kiệm"
            onBack={() => setScreen("list")}
          />
          <main className="savings-content">
            <Section className="savings-detail-hero">
              <div className="savings-contract-head">
                <span>#{selected.number}</span>
                <Badge status={selected.status} />
              </div>
              <span className="savings-muted">Tiền gốc gửi</span>
              <div className="savings-detail-amount">
                {formatVnd(selected.principal)}
              </div>
              <div className="savings-contract-meta">
                <span>
                  Kỳ hạn <b>{selected.termMonths} tháng</b>
                </span>
                <span>
                  Lãi suất <b>{selected.annualRate}%/năm</b>
                </span>
              </div>
            </Section>
            <Section>
              <h3>Tiến độ kỳ hạn</h3>
              <div className="savings-progress">
                <span
                  style={{
                    width: selected.status === "active" ? "35%" : "100%",
                  }}
                />
              </div>
              <Line label="Ngày mở" value={formatDate(selected.openedAt)} />
              <Line label="Đáo hạn" value={formatDate(selected.maturityAt)} />
              <p className="savings-hint">
                {selected.status === "active"
                  ? "Sổ đang trong kỳ hạn gửi."
                  : selected.status === "matured"
                    ? "Sổ đã đến hạn, có thể xem trước tất toán."
                    : selected.status === "locked" ? "Sổ hiện đang tạm khóa." : "Sổ đã tất toán."}
              </p>
            </Section>
            <Section>
              <h3>Bảng chi tiết thanh toán</h3>
              <p className="savings-hint">Tiền lãi dự kiến theo thông tin sổ; số tiền thực nhận được xác định khi tất toán.</p>
              <Line
                label="Tổng gốc ban đầu"
                value={formatVnd(selected.principal)}
              />
              <Line
                label="Tiền lãi đến hạn"
                value={`+${formatVnd(selected.interest)}`}
              />
              <Line
                label="Tổng nhận khi đáo hạn"
                strong
                value={formatVnd(selected.principal + selected.interest)}
              />
              <Line label="Tài khoản nhận" value={paymentAccountLabel(selected.sourceAccount, sourceAccountId, account?.accountNo)} />
              {receipt && <>
                <Line label="Mã giao dịch tất toán" value={receipt.settlement.transactionId} />
                <Line label="Tổng tiền biên nhận" value={formatVnd(receipt.settlement.totalAmount)} />
              </>}
            </Section>
            <Section>
              <h3>Thông tin hợp đồng tiết kiệm</h3>
              <Line label="Mã sổ / hợp đồng" value={selected.number} />
              <Line label="Số tiền gửi" value={formatVnd(selected.principal)} />
              <Line label="Kỳ hạn" value={`${selected.termMonths} tháng`} />
              <Line label="Lãi suất tại ngày mở" value={`${selected.annualRate}%/năm`} />
              <Line label="Ngày hiệu lực" value={formatDate(selected.openedAt)} />
              <Line label="Ngày đáo hạn" value={formatDate(selected.maturityAt)} />
              <Line label="Tài khoản trích tiền" value={paymentAccountLabel(selected.sourceAccount, sourceAccountId, account?.accountNo)} />
            </Section>
            <Section>
              <h3>Lịch sử giao dịch của sổ</h3>
              <Button secondary disabled={historyLoading} onClick={() => void loadHistory()}>
                {historyLoading ? "Đang tải lịch sử…" : history ? "Tải lại lịch sử" : "Xem lịch sử giao dịch"}
              </Button>
              {historyError && <p className="savings-error" role="alert">{historyError}</p>}
              {history && history.length === 0 && <p className="savings-muted">Chưa có lịch sử giao dịch để hiển thị. Vui lòng thử tải lại.</p>}
              {history?.map((entry) => (
                <div key={entry.transactionId} className="savings-history-row">
                  <div>
                    <strong>{entry.direction.toLowerCase() === "debit" ? "Trích tiền mở sổ" : "Hoàn gốc và lãi"}</strong>
                    <small>{formatDate(entry.createdAt)}</small>
                  </div>
                  <strong>{entry.direction.toLowerCase() === "debit" ? "−" : "+"}{formatVnd(entry.amount)}</strong>
                </div>
              ))}
            </Section>
            <Section className="savings-info">
              <h3>Lưu ý tất toán</h3>
              <p>
                Chỉ tất toán toàn bộ khi đã đủ kỳ hạn, tiền về tài khoản nguồn.
                Điều kiện tất toán được kiểm tra khi bạn xác nhận yêu cầu.
              </p>
            </Section>
            {selected.status === "matured" ? (
              <Button disabled={busy || !!pending} onClick={() => void beginWithdraw()}
              >
                Rút tiền tiết kiệm
              </Button>
            ) : (
              <Button disabled onClick={() => {}}>
                {" "}
                {selected.status === "settled"
                  ? "Đã tất toán"
                  : selected.status === "locked" ? "Sổ đang tạm khóa" : "Chưa đến ngày đáo hạn"}{" "}
              </Button>
            )}
            <Button secondary onClick={() => setScreen("list")}>
              Về trang tiết kiệm
            </Button>
          </main>
        </>
      )}

      {screen === "withdraw-preview" &&
        selected &&
        selected.status === "matured" && (
          <>
            <Head
              title="Xác nhận rút tiết kiệm"
              onBack={() => setScreen("detail")}
            />
            <main className="savings-content">
              <section className="savings-amount-hero">
                <span>Tổng tiền nhận khi tất toán</span>
                <strong>{formatVnd(withdrawQuote?.totalPayout ?? 0)}</strong>
                <small>Toàn bộ gốc + lãi đến hạn</small>
              </section>
              <Section>
                <h3>Chi tiết tất toán</h3>
                <Line label="Mã sổ" value={selected.number} />
                <Line label="Tiền gốc" value={formatVnd(selected.principal)} />
                <Line
                  label="Tiền lãi"
                  value={`+${formatVnd(withdrawQuote?.interestAmount ?? 0)}`}
                />
                <Line label="Lãi suất" value={`${selected.annualRate}%/năm`} />
                <Line
                  label="Ngày đáo hạn"
                  value={formatDate(selected.maturityAt)}
                />
                <Line
                  label="Tổng cộng"
                  strong
                  value={formatVnd(withdrawQuote?.totalPayout ?? 0)}
                />
              </Section>
              <Section className="savings-account">
                <h3>Tài khoản nhận tiền</h3>
                <p>Tài khoản thanh toán</p>
                <strong>{paymentAccountLabel(withdrawQuote?.destinationAccountId ?? selected.sourceAccount, sourceAccountId, account?.accountNo)}</strong>
              </Section>
              <Section className="savings-info">
                <h3>Thông tin quan trọng</h3>
                <p>
                  Bạn chỉ có thể tất toán toàn bộ sổ từ ngày đáo hạn.
                  Thông tin kỳ hạn, tài khoản nhận và số tiền được kiểm tra lại khi xác nhận.
                </p>
              </Section>
              <label className="savings-checkbox">
                <input
                  type="checkbox"
                  checked={withdrawAccepted}
                  onChange={(e) => setWithdrawAccepted(e.target.checked)}
                />{" "}
                Tôi đồng ý tất toán toàn bộ sổ và nhận gốc, lãi về tài khoản thanh toán nguồn.
              </label>
              <Button onClick={confirmWithdraw} disabled={!withdrawAccepted || busy || !!pending || !withdrawQuote}>
                Xác nhận rút tiền
              </Button>
            </main>
          </>
        )}

      {screen === "withdraw-success" && lastWithdrawal && (
          <>
            <Head
              title="Kết quả rút tiết kiệm"
              onBack={() => setScreen("list")}
            />
            <main className="savings-content">
              <SuccessMark />
              <h2 className="savings-success-title">
                Rút tiết kiệm thành công!
              </h2>
              <p className="savings-center savings-muted">
                Sổ tiết kiệm đã được tất toán. Tiền gốc và lãi đã được ghi có vào tài khoản thanh toán nguồn.
              </p>
              <section className="savings-amount-hero">
                <span>Tổng tiền tất toán</span>
                <strong>
                  {formatVnd(lastWithdrawal.totalPayout)}
                </strong>
              </section>
              <Section>
                <h3>Thông tin tất toán</h3>
                <Line label="Mã sổ" value={lastWithdrawal.savingsNo} />
                <Line label="Trạng thái" value={<Badge status="settled" />} />
                <Line label="Tổng gốc" value={formatVnd(lastWithdrawal.principal)} />
                <Line
                  label="Tiền lãi"
                  value={`+${formatVnd(lastWithdrawal.interestAmount)}`}
                />
                <Line
                  label="Tổng tiền nhận"
                  strong
                  value={formatVnd(lastWithdrawal.totalPayout)}
                />
                <Line label="Tài khoản nhận" value={paymentAccountLabel(lastWithdrawal.destinationAccountId, sourceAccountId, account?.accountNo)} />
                <Line
                  label="Thời điểm"
                  value={formatDate(
                    lastWithdrawal.settledAt,
                  )}
                />
              </Section>
              <Button onClick={() => void showReceipt()}>Xem biên nhận</Button>
              <Button
                secondary
                onClick={() => {
                  setFilter("all");
                  setScreen("list");
                }}
              >
                Về trang tiết kiệm
              </Button>
            </main>
          </>
        )}
    </div>
  );
}
