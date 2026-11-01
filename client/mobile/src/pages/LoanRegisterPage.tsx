import { useEffect, useRef, useState } from "react";
import { faShieldHalved, faWallet } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
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
import {
  dtiStatusMeta,
  explainLoanError,
  percentBps,
  vnd,
} from "@/features/loans/format";
import { MoneyInput } from "@/features/loans/components/MoneyInput";
import {
  clearIdempotencyKey,
  getOrCreateIdempotencyKey,
} from "@/features/loans/idempotency";
import { ChoiceList, StepForm } from "@/shared/ui";

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

const REGIONS = [
  { value: "KV01", label: "HCM, Hà Nội, Cần Thơ" },
  { value: "KV02", label: "Huế, Đà Nẵng, Hải Phòng" },
  { value: "KV03", label: "Khu vực khác" },
] as const;
type RegionCode = (typeof REGIONS)[number]["value"];

const ALL_STEPS = [
  "package",
  "amount",
  "term",
  "purpose",
  "income",
  "debt",
  "region",
  "review",
] as const;
type StepId = (typeof ALL_STEPS)[number];

const STEP_TEXT: Record<StepId, { heading: string; hint?: string }> = {
  package: { heading: "Chọn gói vay" },
  amount: { heading: "Bạn muốn vay bao nhiêu?" },
  term: { heading: "Bạn muốn trả trong bao lâu?" },
  purpose: {
    heading: "Bạn vay để làm gì?",
    hint: "Giúp AnomalyBank áp dụng chính sách phù hợp nhất.",
  },
  income: {
    heading: "Thu nhập mỗi tháng của bạn?",
    hint: "Thông tin tự khai chỉ để tính thử, chưa phải kết quả thẩm định.",
  },
  debt: {
    heading: "Bạn đang trả nợ bao nhiêu mỗi tháng?",
    hint: "Không có khoản nợ nào thì để trống.",
  },
  region: { heading: "Bạn đang sống ở khu vực nào?" },
  review: {
    heading: "Xem lại khoản vay",
    hint: "Kiểm tra thông tin trước khi tải chứng từ.",
  },
};

interface LoanDraft {
  amount: number;
  tenure: number;
  purpose: string;
  details: string;
  income: number;
  debt: number;
  region: RegionCode;
  step?: StepId;
}

const DEFAULT_DRAFT: LoanDraft = {
  amount: DEFAULT_AMOUNT,
  tenure: DEFAULT_TENURE,
  purpose: PURPOSES[0],
  details: "",
  income: 20_000_000,
  debt: 0,
  region: "KV01",
};

const validMoney = (value: unknown, fallback: number) =>
  Number.isSafeInteger(value) && (value as number) >= 0
    ? (value as number)
    : fallback;

interface SaveMessage {
  tone: "success" | "error";
  text: string;
}

const LoanRegisterPage = () => {
  const { user, token } = useAuth();
  const draftKey = `anomaly:loan-draft:${user?.id ?? "anonymous"}`;
  const [draft, setDraft] = useLocalStorage<LoanDraft>(draftKey, DEFAULT_DRAFT);
  const [saveMessage, setSaveMessage] = useState<SaveMessage | null>(null);
  const [packages, setPackages] = useState<LoanPackage[]>([]);
  const [packageCode, setPackageCode] = useState("");
  const [loadingPackages, setLoadingPackages] = useState(true);
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

  const income = validMoney(storedDraft.income, DEFAULT_DRAFT.income);
  const debt = validMoney(storedDraft.debt, DEFAULT_DRAFT.debt);
  const region =
    REGIONS.find((item) => item.value === storedDraft.region)?.value ??
    DEFAULT_DRAFT.region;

  // Chỉ có một gói thì không cần hỏi.
  const steps = ALL_STEPS.filter(
    (item) => item !== "package" || packages.length > 1,
  );
  const step = steps.find((item) => item === storedDraft.step) ?? steps[0];
  const stepIndex = steps.indexOf(step);

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
  const amountError =
    selectedPackage && amount > 0
      ? amount < selectedPackage.minAmount
        ? `Tối thiểu ${vnd(selectedPackage.minAmount)}`
        : amount > selectedPackage.maxAmount
          ? `Tối đa ${vnd(selectedPackage.maxAmount)}`
          : undefined
      : undefined;
  const dti = quote
    ? dtiStatusMeta[quote.affordability?.dtiStatus ?? ""]
    : null;
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

  // Đổi bước không làm mất báo giá; chỉ đổi dữ liệu mới hủy báo giá.
  const goTo = (next: StepId) => {
    setSaveMessage(null);
    setDraft((previous) => ({
      ...DEFAULT_DRAFT,
      ...(previous ?? DEFAULT_DRAFT),
      step: next,
    }));
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
    setSaveMessage(null);
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
        text: "Vui lòng xem báo giá mới nhất trước khi tạo hồ sơ.",
      });
      return;
    }
    if (!sourceAccountId.trim()) {
      setSaveMessage({
        tone: "error",
        text: "Không xác định được tài khoản nhận giải ngân.",
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
      // Hồ sơ đã tạo: lần đăng ký sau phải bắt đầu lại từ đầu.
      window.localStorage.removeItem(draftKey);
      navigate(loanApplication(result.id));
    } catch (error) {
      setSaveMessage({ tone: "error", text: explainLoanError(error) });
    } finally {
      setBusy(false);
    }
  };

  const goNext = () => {
    if (step === "review") {
      void handleCreate();
      return;
    }
    const next = steps[stepIndex + 1];
    goTo(next);
    if (next === "review") void handleQuote();
  };
  const goBack = () => {
    if (stepIndex === 0) navigate(routes.loanApplications);
    else goTo(steps[stepIndex - 1]);
  };
  const needsDetails = purpose.startsWith("Khác");
  const canContinue: Record<StepId, boolean> = {
    package: !!selectedPackage,
    amount: !!selectedPackage && amount > 0 && !amountError,
    term: !selectedPackage || selectedPackage.termMonths.includes(tenure),
    purpose: !needsDetails || !!details.trim(),
    income: income > 0,
    debt: true,
    region: withinPackage && income > 0,
    review: !!quote && withinPackage,
  };
  const message = saveMessage && (
    <p
      role={saveMessage.tone === "error" ? "alert" : "status"}
      className={`rounded-xl px-4 py-3 text-center text-sm font-semibold ${
        saveMessage.tone === "error"
          ? "bg-error-container text-on-error-container"
          : "bg-success-container text-on-success-container"
      }`}
    >
      {saveMessage.text}
    </p>
  );

  return (
    <StepForm
      title="Đăng ký vay vốn"
      stepId={step}
      stepNumber={stepIndex + 1}
      stepCount={steps.length}
      heading={STEP_TEXT[step].heading}
      hint={STEP_TEXT[step].hint}
      canContinue={canContinue[step]}
      continueLabel={step === "review" ? "Tiếp tục tải chứng từ" : "Tiếp tục"}
      busy={busy}
      onBack={goBack}
      onNext={goNext}
    >
      {step === "package" && (
        <ChoiceList
          label="Gói vay"
          value={packageCode}
          options={packages.map((item) => ({
            value: item.packageCode,
            label: item.name,
            description: `Từ ${vnd(item.minAmount)} đến ${vnd(item.maxAmount)}`,
          }))}
          onChoose={(value) => {
            setPackageCode(value);
            setQuote(null);
            quotedFingerprint.current = null;
          }}
          onConfirm={goNext}
        />
      )}

      {step === "amount" && (
        <>
          <MoneyInput
            label="Số tiền đề nghị vay"
            size="lg"
            value={amount}
            onChange={(value) => updateDraft({ amount: value })}
            error={amountError}
            hint={
              selectedPackage
                ? `Từ ${vnd(selectedPackage.minAmount)} đến ${vnd(selectedPackage.maxAmount)}`
                : undefined
            }
            suggestMin={selectedPackage?.minAmount}
            suggestMax={selectedPackage?.maxAmount}
          />
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
                className={`min-h-11 rounded-xl text-xs font-semibold disabled:opacity-35 ${amount === value ? "bg-secondary text-white" : "bg-surface-container text-on-surface-variant"}`}
              >
                {value / 1_000_000} Tr
              </button>
            ))}
          </div>
          {!loadingPackages && packages.length === 0 && (
            <p role="alert" className="text-sm text-error">
              Hiện chưa có gói vay khả dụng. Vui lòng quay lại sau.
            </p>
          )}
        </>
      )}

      {step === "term" && (
        <ChoiceList
          label="Kỳ hạn vay"
          value={String(tenure)}
          options={Array.from(
            new Set<number>([
              ...TENURES,
              ...(selectedPackage?.termMonths ?? []),
            ]),
          )
            .sort((a, b) => a - b)
            .map((value) => ({
              value: String(value),
              label: `${value} tháng`,
              disabled:
                !!selectedPackage &&
                !selectedPackage.termMonths.includes(value),
            }))}
          onChoose={(value) => {
            updateDraft({ tenure: Number(value) });
          }}
          onConfirm={goNext}
        />
      )}

      {step === "purpose" && (
        <>
          <ChoiceList
            label="Mục đích vay"
            value={purpose}
            options={PURPOSES.map((item) => ({ value: item, label: item }))}
            onChoose={(value) => {
              updateDraft({ purpose: value });
            }}
            onConfirm={() => {
              if (canContinue.purpose) goNext();
            }}
          />
          {needsDetails && (
            <label className="block text-xs font-semibold text-primary">
              Nhập mục đích vay cụ thể <span className="text-error">*</span>
              <textarea
                value={details}
                onChange={(event) =>
                  updateDraft({ details: event.target.value })
                }
                maxLength={250}
                rows={3}
                className="mt-2 w-full resize-none rounded-2xl bg-surface-container-low p-3 text-sm font-normal outline-none focus:ring-2 focus:ring-secondary/30"
                placeholder="Mô tả mục đích sử dụng khoản vay..."
              />
              <span className="mt-1 block text-right font-normal text-outline">
                {details.length}/250 ký tự
              </span>
            </label>
          )}
        </>
      )}

      {step === "income" && (
        <MoneyInput
          label="Thu nhập mỗi tháng"
          size="lg"
          value={income}
          onChange={(value) => updateDraft({ income: value })}
        />
      )}

      {step === "debt" && (
        <MoneyInput
          label="Khoản trả nợ hiện tại mỗi tháng"
          size="lg"
          value={debt}
          onChange={(value) => updateDraft({ debt: value })}
        />
      )}

      {step === "region" && (
        <ChoiceList
          label="Khu vực"
          value={region}
          options={[...REGIONS]}
          onChoose={(value) => {
            updateDraft({ region: value as RegionCode });
          }}
          onConfirm={() => {
            if (canContinue.region) goNext();
          }}
        />
      )}

      {step === "review" && (
        <>
          <dl className="space-y-2 rounded-2xl bg-white p-4 text-sm shadow-sm">
            {[
              ["Gói vay", selectedPackage?.name ?? "Đang tải..."],
              ["Số tiền vay", vnd(amount)],
              ["Kỳ hạn", `${tenure} tháng`],
              ["Mục đích", needsDetails ? details : purpose],
              ["Thu nhập mỗi tháng", vnd(income)],
              ["Trả nợ mỗi tháng", vnd(debt)],
              [
                "Khu vực",
                REGIONS.find((item) => item.value === region)?.label ?? "",
              ],
            ].map(([label, value]) => (
              <div key={label} className="flex justify-between gap-3">
                <dt className="shrink-0 text-on-surface-variant">{label}</dt>
                <dd className="min-w-0 text-right font-semibold break-words text-primary">
                  {value}
                </dd>
              </div>
            ))}
          </dl>

          {quote ? (
            <div
              role="status"
              className="space-y-3 rounded-2xl bg-surface-container-low p-4"
            >
              <div>
                <p className="text-xs text-on-surface-variant">
                  Trả mỗi tháng khoảng
                </p>
                <p className="text-2xl font-bold text-primary">
                  {vnd(quote.estimatedMonthlyPayment)}
                </p>
              </div>
              <dl className="space-y-2 text-xs">
                <div className="flex justify-between gap-3">
                  <dt className="text-on-surface-variant">Lãi suất dự kiến</dt>
                  <dd className="font-semibold text-primary">
                    {percentBps(quote.annualRateBps)}
                  </dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-on-surface-variant">Tổng lãi</dt>
                  <dd className="font-semibold text-primary">
                    {vnd(quote.totalInterest)}
                  </dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="text-on-surface-variant">Tổng phải trả</dt>
                  <dd className="font-semibold text-primary">
                    {vnd(quote.totalRepayment)}
                  </dd>
                </div>
                <div className="flex items-center justify-between gap-3">
                  <dt className="text-on-surface-variant">Khả năng trả nợ</dt>
                  <dd
                    className={`rounded-full px-2.5 py-0.5 font-semibold ${dti?.className ?? "bg-gray-100 text-gray-700"}`}
                  >
                    {dti?.label ?? "Chưa rõ"}
                  </dd>
                </div>
              </dl>
              {quote.affordability?.withinIncomeLimit === false && (
                <p className="text-xs text-error">
                  Số tiền vượt hạn mức theo thu nhập (
                  {vnd(quote.affordability.incomeLimit)}).
                </p>
              )}
              <p className="text-xs text-on-surface-variant">
                <FontAwesomeIcon
                  icon={faShieldHalved}
                  className="mr-1 text-secondary"
                />
                Không phải đề nghị chính thức. Lãi suất chính thức do ngân hàng
                xác định sau thẩm định.
              </p>
            </div>
          ) : (
            !busy && (
              <button
                type="button"
                disabled={!withinPackage || income <= 0}
                onClick={() => void handleQuote()}
                className="min-h-12 w-full rounded-full bg-surface-container px-4 text-sm font-bold text-primary disabled:opacity-40"
              >
                Xem báo giá dự kiến
              </button>
            )
          )}
          {busy && !quote && (
            <p
              role="status"
              className="text-center text-sm text-on-surface-variant"
            >
              Đang tính báo giá...
            </p>
          )}

          {quote && (
            <details className="rounded-2xl bg-white p-4 shadow-sm">
              <summary className="cursor-pointer text-sm font-bold text-primary">
                Lịch trả góp dự kiến ({quote.provisionalSchedule?.length ?? 0}{" "}
                kỳ)
              </summary>
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

          <div className="flex items-center gap-3 rounded-2xl bg-surface-container-low p-3">
            <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-secondary-container text-secondary">
              <FontAwesomeIcon icon={faWallet} />
            </div>
            <div className="min-w-0">
              <p className="text-xs text-on-surface-variant">
                Tài khoản nhận giải ngân
              </p>
              <p className="text-sm font-bold text-primary">
                •••• {user?.accountNo.slice(-4) ?? "----"}
              </p>
            </div>
          </div>
        </>
      )}

      {message}
    </StepForm>
  );
};

export default LoanRegisterPage;
