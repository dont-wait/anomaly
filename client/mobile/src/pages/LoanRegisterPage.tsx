import { useEffect, useRef, useState } from "react";
import {
  ArrowLeftIcon,
  ChevronDownIcon,
  CheckIcon,
  ShieldIcon,
  UserIcon,
} from "@/shared/icons";
import { Logo } from "@/shared/layout/Logo";
import { navigate, routes, loanApplication } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import { useLocalStorage } from "@/shared/hooks";
import {
  createLoan,
  listLoanPackages,
  preQuoteLoan,
  type LoanPackage,
  type LoanQuote,
} from "@/features/loans/api";
import { explainLoanError, percentBps, vnd } from "@/features/loans/format";
import {
  clearIdempotencyKey,
  getOrCreateIdempotencyKey,
} from "@/features/loans/idempotency";
import { SelectSheet } from "@/shared/ui";

const AMOUNTS = [30_000_000, 50_000_000, 100_000_000, 200_000_000] as const;
const TENURES = [6, 12, 24, 36] as const;
const PURPOSES = [
  "Tiêu dùng cá nhân",
  "Mua xe",
  "Mua nhà / Cải tạo tổ ấm",
  "Học tập & Phát triển kỹ năng",
  "Chi phí y tế & Chăm sóc sức khỏe",
  "Khác (Cung cấp chi tiết)",
];
const DEFAULT_AMOUNT = 50_000_000;
const DEFAULT_TENURE = 12;

interface LoanDraft {
  amount: number;
  tenure: number;
  purpose: string;
  details: string;
}

const DEFAULT_DRAFT: LoanDraft = {
  amount: DEFAULT_AMOUNT,
  tenure: DEFAULT_TENURE,
  purpose: PURPOSES[0],
  details: "",
};

interface SaveMessage {
  tone: "success" | "error";
  text: string;
}

const formatMoney = (value: number) =>
  new Intl.NumberFormat("vi-VN").format(value);

const LoanRegisterPage = () => {
  const { user, token } = useAuth();
  const [draft, setDraft] = useLocalStorage<LoanDraft>(
    `anomaly:loan-draft:${user?.id ?? "anonymous"}`,
    DEFAULT_DRAFT,
  );
  const [saveMessage, setSaveMessage] = useState<SaveMessage | null>(null);
  const [purposeOpen, setPurposeOpen] = useState(false);
  const [packages, setPackages] = useState<LoanPackage[]>([]);
  const [packageCode, setPackageCode] = useState("");
  const [loadingPackages, setLoadingPackages] = useState(true);
  const [income, setIncome] = useState(20_000_000);
  const [region, setRegion] = useState<"KV01" | "KV02" | "KV03">("KV01");
  const [debt, setDebt] = useState(0);
  const sourceAccountId = user?.id ?? "";
  const [quote, setQuote] = useState<LoanQuote | null>(null);
  const [busy, setBusy] = useState(false);
  const quotedFingerprint = useRef<string | null>(null);
  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    void listLoanPackages(token, controller.signal)
      .then(({ items }) => {
        setPackages(items ?? []);
        setPackageCode((current) => current || items?.[0]?.packageCode || "");
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted)
          setSaveMessage({ tone: "error", text: explainLoanError(error) });
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingPackages(false);
      });
    return () => controller.abort();
  }, [token]);

  // Ignore stale or hand-edited values from an older stored draft.
  const storedDraft: Partial<LoanDraft> =
    draft && typeof draft === "object" ? draft : DEFAULT_DRAFT;
  const amount =
    Number.isSafeInteger(storedDraft.amount) && (storedDraft.amount ?? -1) >= 0
      ? storedDraft.amount!
      : DEFAULT_AMOUNT;
  const tenure =
    Number.isSafeInteger(storedDraft.tenure) && (storedDraft.tenure ?? 0) > 0
      ? storedDraft.tenure!
      : DEFAULT_TENURE;
  const purpose = PURPOSES.includes(storedDraft.purpose ?? "")
    ? (storedDraft.purpose as string)
    : PURPOSES[0];
  const details =
    typeof storedDraft.details === "string" ? storedDraft.details : "";

  const quoteFingerprint = JSON.stringify({
    packageCode,
    amount,
    tenure,
    income,
    region,
    debt,
  });
  const selectedPackage = packages.find(
    (item) => item.packageCode === packageCode,
  );
  const withinPackage =
    !!selectedPackage &&
    amount >= selectedPackage.minAmount &&
    amount <= selectedPackage.maxAmount &&
    selectedPackage.termMonths.includes(tenure);
  const updateDraft = (patch: Partial<LoanDraft>) => {
    setQuote(null);
    quotedFingerprint.current = null;
    setDraft((previous) => ({
      ...DEFAULT_DRAFT,
      ...(previous ?? DEFAULT_DRAFT),
      ...patch,
    }));
    setSaveMessage(null);
  };

  const handleSaveDraft = () => {
    if (purpose.startsWith("Khác") && !details.trim()) {
      setSaveMessage({
        tone: "error",
        text: "Vui lòng nhập mục đích vay cụ thể trước khi lưu bản nháp.",
      });
      return;
    }
    setSaveMessage({
      tone: "success",
      text: "Đã lưu bản nháp trên thiết bị này. Hồ sơ chưa được gửi lên hệ thống.",
    });
  };

  const handleQuote = async () => {
    if (!token || !withinPackage) {
      setSaveMessage({
        tone: "error",
        text: "Chọn số tiền và kỳ hạn hợp lệ trong gói vay đang hoạt động.",
      });
      return;
    }
    setBusy(true);
    setQuote(null);
    const requestedFingerprint = quoteFingerprint;
    quotedFingerprint.current = requestedFingerprint;
    try {
      const result = await preQuoteLoan(token, {
        loanPackageId: packageCode,
        requestedAmount: amount,
        termMonths: tenure,
        monthlyIncome: income,
        regionCode: region,
        existingMonthlyDebt: debt,
      });
      if (quotedFingerprint.current === requestedFingerprint) {
        setQuote(result);
        setSaveMessage(null);
      }
    } catch (error) {
      setSaveMessage({ tone: "error", text: explainLoanError(error) });
    } finally {
      setBusy(false);
    }
  };

  const handleCreate = async () => {
    if (
      !token ||
      !withinPackage ||
      !quote ||
      quotedFingerprint.current !== quoteFingerprint ||
      quote.loanPackageId !== packageCode ||
      quote.requestedAmount !== amount ||
      quote.termMonths !== tenure
    ) {
      setSaveMessage({
        tone: "error",
        text: "Bạn cần xem báo giá API mới nhất trước khi tạo hồ sơ.",
      });
      return;
    }
    if (!sourceAccountId.trim()) {
      setSaveMessage({
        tone: "error",
        text: "Vui lòng xác nhận ID tài khoản nhận giải ngân.",
      });
      return;
    }
    if (purpose.startsWith("Khác") && !details.trim()) {
      setSaveMessage({
        tone: "error",
        text: "Vui lòng nhập mục đích vay cụ thể.",
      });
      return;
    }
    const payload = {
      loanPackageId: packageCode,
      requestedAmount: amount,
      termMonths: tenure,
      disbursementAccountId: sourceAccountId.trim(),
    };
    const fingerprint = JSON.stringify(payload);
    const scope = `create-loan:${user?.id ?? "anonymous"}`;
    const idempotencyKey = getOrCreateIdempotencyKey(scope, fingerprint);
    setBusy(true);
    try {
      const result = await createLoan(token, payload, idempotencyKey);
      clearIdempotencyKey(scope, fingerprint);
      navigate(loanApplication(result.id));
    } catch (error) {
      setSaveMessage({ tone: "error", text: explainLoanError(error) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto flex min-h-screen max-w-md flex-col bg-background text-on-surface">
      <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-primary/5 bg-background/90 px-4 backdrop-blur-xl">
        <div className="flex min-w-0 items-center gap-2">
          <button
            type="button"
            aria-label="Quay lại"
            onClick={() => navigate(routes.dashboard)}
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-primary active:bg-surface-container"
          >
            <ArrowLeftIcon />
          </button>
          <Logo className="!h-8" />
          <div className="min-w-0 border-l border-outline-variant pl-2">
            <p className="truncate text-[10px] font-bold uppercase tracking-widest text-secondary">
              AnomalyBank
            </p>
            <h1 className="truncate text-sm font-bold text-primary">
              Đăng ký vay vốn
            </h1>
          </div>
        </div>
        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-white">
          <UserIcon className="h-4 w-4" />
        </div>
      </header>

      <main className="flex-1 space-y-4 overflow-y-auto px-4 pb-8 pt-4">
        <section className="rounded-2xl bg-white p-4 shadow-sm">
          <div className="flex items-center justify-between gap-2">
            <div className="flex items-center gap-2 text-primary">
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-secondary text-xs font-bold text-white">
                1
              </span>
              <h2 className="text-sm font-bold">Thông tin khoản vay</h2>
            </div>
            <span className="rounded-full bg-secondary-container px-3 py-1 text-[10px] font-semibold text-primary">
              Bước 1 / 3
            </span>
          </div>
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-surface-container">
            <div className="h-full w-1/3 rounded-full bg-gradient-to-r from-primary to-secondary" />
          </div>
          <p className="mt-2 text-xs text-on-surface-variant">
            Hồ sơ sẽ tự động lưu dưới dạng <strong>Bản nháp</strong> trên thiết
            bị này.
          </p>
        </section>

        <section className="relative overflow-hidden rounded-2xl bg-gradient-to-br from-primary-container via-primary to-secondary p-5 text-white shadow-md">
          <div className="absolute -bottom-10 -right-5 h-32 w-32 rounded-full bg-secondary-container/30 blur-2xl" />
          <p className="relative inline-flex rounded-full bg-white/15 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider">
            ✦ Gói vay đặc quyền
          </p>
          <h2 className="relative mt-3 text-xl font-bold">
            Vay vốn cá nhân theo lương
          </h2>
          <p className="relative mt-2 text-xs text-white/80">
            Lãi suất và khoản trả góp do ngân hàng tính qua API. Kết quả chính
            thức phụ thuộc vào thẩm định và Offer.
          </p>
        </section>

        <section className="space-y-3 rounded-2xl bg-white p-4 shadow-sm">
          <div className="block text-sm font-bold text-primary">
            Gói vay hiện có
          </div>
          <SelectSheet
            id="loan-package"
            label="Gói vay hiện có"
            disabled={loadingPackages || busy}
            value={packageCode}
            onChange={(value) => {
              setPackageCode(value);
              setQuote(null);
              quotedFingerprint.current = null;
            }}
            placeholder="Chưa có gói vay khả dụng"
            options={packages.map((p) => ({ value: p.packageCode, label: p.name }))}
          />
          {!loadingPackages && packages.length === 0 && (
            <p role="alert" className="text-xs text-error">
              Chưa có gói vay ACTIVE trên backend. Không thể tạo hồ sơ.
            </p>
          )}
        </section>
        <section className="space-y-4 rounded-2xl bg-white p-4 shadow-sm">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-bold text-primary">
              Số tiền đề nghị vay
            </h2>
            <span className="text-[10px] text-on-surface-variant">
              {selectedPackage
                ? `Tối đa: ${vnd(selectedPackage.maxAmount)}`
                : "Theo gói vay"}
            </span>
          </div>
          <div className="flex items-center rounded-2xl bg-surface-container-low px-4 py-2">
            <output
              aria-label="Số tiền đề nghị vay"
              className="flex-1 text-3xl font-bold tracking-tight text-primary"
            >
              {formatMoney(amount)}
            </output>
            <span className="text-xl font-bold text-secondary">đ</span>
          </div>
          <label className="block text-xs font-semibold text-primary">
            Hoặc nhập số tiền theo hạn mức gói vay
            <input
              type="number"
              min={1}
              step={1000000}
              value={amount === 0 ? "" : amount}
              onChange={(e) =>
                updateDraft({
                  amount: e.target.value === "" ? 0 : Number(e.target.value),
                })
              }
              className="mt-2 w-full rounded-xl border border-outline-variant p-3 text-sm"
            />
          </label>
          <div
            className="grid grid-cols-4 gap-2"
            aria-label="Gợi ý số tiền"
            role="radiogroup"
          >
            {AMOUNTS.map((value) => (
              <button
                key={value}
                type="button"
                role="radio"
                aria-checked={amount === value}
                disabled={
                  !!selectedPackage &&
                  (value < selectedPackage.minAmount ||
                    value > selectedPackage.maxAmount)
                }
                onClick={() => updateDraft({ amount: value })}
                className={`rounded-xl disabled:opacity-35 py-2 text-xs font-semibold transition-transform active:scale-95 ${amount === value ? "bg-secondary text-white shadow-md shadow-secondary/25" : "bg-surface-container text-on-surface-variant"}`}
              >
                {value / 1_000_000} Tr
              </button>
            ))}
          </div>
        </section>

        <section className="space-y-4 rounded-2xl bg-white p-4 shadow-sm">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-bold text-primary">
              Kỳ hạn vay mong muốn
            </h2>
            <span className="rounded-full bg-secondary-container px-2.5 py-1 text-[10px] font-semibold text-primary">
              {tenure} Tháng
            </span>
          </div>
          <div
            className="grid grid-cols-4 gap-2"
            role="radiogroup"
            aria-label="Kỳ hạn vay"
          >
            {Array.from(
              new Set<number>([
                ...TENURES,
                ...(selectedPackage?.termMonths ?? []),
              ]),
            )
              .sort((a, b) => a - b)
              .map((value) => (
                <button
                  key={value}
                  type="button"
                  role="radio"
                  aria-checked={tenure === value}
                  disabled={
                    !!selectedPackage &&
                    !selectedPackage.termMonths.includes(value)
                  }
                  onClick={() => updateDraft({ tenure: value })}
                  className={`rounded-xl disabled:opacity-35 py-2 text-sm font-bold ${tenure === value ? "bg-secondary text-white shadow-md shadow-secondary/25" : "bg-surface-container text-on-surface-variant"}`}
                >
                  <span className="block">{value}</span>
                  <span className="text-[10px] font-normal">Tháng</span>
                </button>
              ))}
          </div>
        </section>

        <section className="space-y-3 rounded-2xl bg-white p-4 shadow-sm">
          <div>
            <div className="flex items-center justify-between">
              <h2 className="text-sm font-bold text-primary">
                Mục đích vay <span className="text-error">*</span>
              </h2>
              <span className="text-[10px] font-medium text-secondary">
                Bắt buộc
              </span>
            </div>
            <p className="mt-1 text-xs text-on-surface-variant">
              Giúp AnomalyBank áp dụng chính sách ưu đãi phù hợp nhất.
            </p>
          </div>
          <button
            type="button"
            aria-expanded={purposeOpen}
            onClick={() => setPurposeOpen((open) => !open)}
            className="flex w-full items-center justify-between rounded-2xl bg-surface-container-low px-4 py-3 text-left text-sm font-semibold text-primary"
          >
            <span className="truncate">{purpose}</span>
            <ChevronDownIcon
              className={`h-5 w-5 text-secondary transition-transform ${purposeOpen ? "rotate-180" : ""}`}
            />
          </button>
          {purposeOpen && (
            <div
              role="listbox"
              aria-label="Mục đích vay"
              className="space-y-1 rounded-2xl border border-outline-variant/30 bg-white p-1 shadow-lg"
            >
              {PURPOSES.map((item) => (
                <button
                  key={item}
                  type="button"
                  role="option"
                  aria-selected={purpose === item}
                  onClick={() => {
                    updateDraft({ purpose: item });
                    setPurposeOpen(false);
                  }}
                  className={`flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-left text-xs ${purpose === item ? "bg-secondary-container font-semibold text-primary" : "text-on-surface-variant hover:bg-surface-container-low"}`}
                >
                  <span>{item}</span>
                  {purpose === item && (
                    <CheckIcon className="h-4 w-4 text-secondary" />
                  )}
                </button>
              ))}
            </div>
          )}
          {purpose.startsWith("Khác") && (
            <label className="block text-xs font-semibold text-primary">
              Nhập mục đích vay cụ thể <span className="text-error">*</span>
              <textarea
                value={details}
                onChange={(event) =>
                  updateDraft({ details: event.target.value })
                }
                maxLength={250}
                rows={3}
                className="mt-2 w-full resize-none rounded-2xl bg-surface-container-low p-3 text-xs font-normal outline-none focus:ring-2 focus:ring-secondary/30"
                placeholder="Mô tả mục đích sử dụng khoản vay..."
              />
              <span className="mt-1 block text-right font-normal text-outline">
                {details.length}/250 ký tự
              </span>
            </label>
          )}
        </section>

        <section className="space-y-3 rounded-2xl bg-white p-4 shadow-sm">
          <div>
            <h2 className="text-sm font-bold text-primary">
              Tài khoản nhận giải ngân
            </h2>
            <p className="mt-1 text-xs text-on-surface-variant">
              Khoản vay sẽ tự động chuyển vào tài khoản thanh toán chính.
            </p>
          </div>
          <div className="flex items-center justify-between rounded-2xl bg-gradient-to-r from-surface-container-low to-surface-container p-3">
            <div className="flex items-center gap-3">
              <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-secondary-container text-secondary">
                ▰
              </div>
              <div>
                <p className="text-sm font-bold text-primary">
                  AnomalyPay{" "}
                  <span className="rounded-full bg-secondary-container px-2 py-0.5 text-[9px] font-semibold text-primary">
                    Chính chủ
                  </span>
                </p>
                <p className="text-xs tracking-widest">
                  •••• {user?.accountNo.slice(-4) ?? "----"}
                </p>
                <p className="text-[10px] text-on-surface-variant">
                  Số dư khả dụng:{" "}
                  <strong className="text-primary">
                    {formatMoney(user?.amount ?? 0)} đ
                  </strong>
                </p>
              </div>
            </div>
            <span className="flex h-6 w-6 items-center justify-center rounded-full bg-secondary">
              <span className="h-2 w-2 rounded-full bg-white" />
            </span>
          </div>
        </section>

        <section className="space-y-3 rounded-2xl bg-white p-4 shadow-sm">
          <h2 className="text-sm font-bold text-primary">
            Thông tin báo giá dự kiến
          </h2>
          <label className="block text-xs">
            Thu nhập tháng (VND)
            <input
              type="number"
              min={1}
              value={income}
              onChange={(e) => {
                setIncome(Number(e.target.value));
                setQuote(null);
                quotedFingerprint.current = null;
              }}
              className="mt-1 w-full rounded-xl border p-3 text-sm"
            />
          </label>
          <div className="block text-xs">
            Khu vực
            <SelectSheet
              label="Khu vực"
              value={region}
              onChange={(value) => {
                setRegion(value as typeof region);
                setQuote(null);
                quotedFingerprint.current = null;
              }}
              options={[
                { value: "KV01", label: "KV01 - HCM / Hà Nội / Cần Thơ" },
                { value: "KV02", label: "KV02 - Huế / Đà Nẵng / Hải Phòng" },
                { value: "KV03", label: "KV03 - Khu vực khác" },
              ]}
            />
          </div>
          <label className="block text-xs">
            Nợ phải trả hàng tháng hiện tại (VND)
            <input
              type="number"
              min={0}
              value={debt}
              onChange={(e) => {
                setDebt(Number(e.target.value));
                setQuote(null);
                quotedFingerprint.current = null;
              }}
              className="mt-1 w-full rounded-xl border p-3 text-sm"
            />
          </label>
          <p className="text-xs text-on-surface-variant">
            Thông tin tự khai chỉ dùng để tính thử. Lãi suất chính thức lấy từ
            Evidence đã xác minh và Offer.
          </p>
          <button
            type="button"
            onClick={() => void handleQuote()}
            disabled={!withinPackage || busy}
            className="w-full rounded-full bg-primary p-3 text-sm font-bold text-white disabled:opacity-40"
          >
            {busy ? "Đang xử lý..." : "Tính lãi và lịch trả qua API"}
          </button>
          {quote && (
            <p role="status" className="text-xs">
              Lãi suất dự kiến:{" "}
              <strong>{percentBps(quote.annualRateBps)}</strong> · Tổng lãi:{" "}
              {vnd(quote.totalInterest)} · DTI:{" "}
              {quote.affordability?.dtiStatus ?? "Chưa rõ"}. Không phải Offer
              chính thức.
            </p>
          )}
        </section>
        {quote && (
          <details className="rounded-2xl bg-white p-4 shadow-sm">
            <summary className="cursor-pointer text-sm font-bold text-primary">
              Lịch trả góp dự kiến ({quote.provisionalSchedule?.length ?? 0} kỳ)
            </summary>
            <p className="mt-2 text-xs text-on-surface-variant">
              Chỉ là dự tính, không phải lịch trả nợ hợp đồng.
            </p>
            {quote.provisionalSchedule?.map((item) => (
              <div
                key={item.installmentNo}
                className="mt-2 rounded-xl border p-3 text-xs"
              >
                <strong>Kỳ {item.installmentNo}</strong> ·{" "}
                {item.dueDate.slice(0, 10)}
                <p>
                  Gốc {vnd(item.principalAmount)} · Lãi{" "}
                  {vnd(item.interestAmount)}
                </p>
                <p className="font-semibold">Tổng {vnd(item.totalPayment)}</p>
              </div>
            ))}
          </details>
        )}
        <section className="space-y-3 rounded-2xl bg-surface-container-high/60 p-4">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-bold text-primary">
              Tóm tắt khoản vay
            </h2>
            <span className="text-[10px] text-on-surface-variant">
              {quote ? percentBps(quote.annualRateBps) : "Chưa có báo giá"}
            </span>
          </div>
          <dl className="space-y-2 text-xs">
            <div className="flex justify-between">
              <dt className="text-on-surface-variant">
                Tổng số tiền đề nghị vay:
              </dt>
              <dd className="font-bold text-primary">
                {formatMoney(amount)} đ
              </dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-on-surface-variant">Thời hạn hoàn trả:</dt>
              <dd className="font-semibold text-primary">{tenure} tháng</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-on-surface-variant">Phương thức trả nợ:</dt>
              <dd className="rounded-full bg-surface-container px-2 py-0.5 font-semibold text-secondary">
                Trả góp đều
              </dd>
            </div>
          </dl>
          <div className="flex items-center justify-between rounded-2xl bg-white p-3">
            <div>
              <p className="text-[10px] font-bold uppercase tracking-wider text-secondary">
                Ước tính trả hàng tháng
              </p>
              <p className="text-xs text-on-surface-variant">
                Gốc + Lãi dự kiến
              </p>
            </div>
            <strong className="text-right text-lg text-primary">
              {quote ? `~${formatMoney(quote.estimatedMonthlyPayment)}` : "—"}
              <span className="ml-1 text-xs text-secondary">đ/tháng</span>
            </strong>
          </div>
          <p className="text-[10px] text-outline">
            <ShieldIcon className="mr-1 inline h-3.5 w-3.5 text-secondary" />
            Khoản thanh toán thực tế sẽ được xác định sau khi thẩm định hồ sơ.
          </p>
        </section>

        {saveMessage && (
          <p
            role={saveMessage.tone === "error" ? "alert" : "status"}
            className={`rounded-xl px-4 py-3 text-center text-xs font-semibold ${
              saveMessage.tone === "error"
                ? "bg-error-container text-on-error-container"
                : "bg-success-container text-on-success-container"
            }`}
          >
            {saveMessage.text}
          </p>
        )}
        <button
          type="button"
          onClick={handleSaveDraft}
          className="flex h-14 w-full items-center justify-center gap-2 rounded-full bg-secondary text-base font-bold text-white shadow-lg shadow-secondary/30 transition-transform active:scale-[0.98]"
        >
          Lưu bản nháp
        </button>
        <button
          type="button"
          disabled={busy || !quote || !withinPackage}
          onClick={() => void handleCreate()}
          className="flex h-14 w-full items-center justify-center rounded-full bg-primary text-base font-bold text-white disabled:opacity-40"
        >
          Tạo hồ sơ vay trên hệ thống
        </button>
        <button
          type="button"
          onClick={() => navigate(routes.loanApplications)}
          className="w-full rounded-xl border p-3 text-sm font-semibold text-primary"
        >
          Xem hồ sơ đã tạo
        </button>
        <p className="flex items-center justify-center gap-1 text-center text-[10px] text-on-surface-variant">
          <ShieldIcon className="h-3.5 w-3.5 text-secondary" /> Thông tin được
          bảo mật theo chuẩn PCI DSS
        </p>
      </main>
    </div>
  );
};

export default LoanRegisterPage;
