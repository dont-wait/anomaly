import {
  type ReactNode,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  createDraft,
  createPackage,
  deleteDraft,
  deletePackage,
  explainProductError,
  getPackage,
  listPackages,
  publishPackage,
  retirePackage,
  simulateLoan,
  simulateSavings,
  updateDraft,
} from "./api";
import type {
  DraftInput,
  PackageDetail,
  PackageKind,
  PackageStatus,
  PackageSummary,
  PackageVersion,
} from "./model";
import { SavingsTermsPanel } from "./SavingsTermsPanel";
import {
  date,
  fieldInput,
  fieldLabel,
  panel,
  percent,
  tableCell,
  tableHead,
  vnd,
} from "./format";
import { ActionButton, Banner, MoneyField, PageHeader, Pager } from "./ui";

// Trùng tập kỳ hạn máy chủ cho phép; máy chủ vẫn kiểm tra lại khi lưu.
const TERMS: Record<PackageKind, number[]> = {
  loan: [6, 12, 24, 36, 48, 60],
  savings: [3, 6, 9, 12, 24, 36, 60],
};

const PACKAGES_PER_PAGE = 10;

const STATUS_LABELS: Record<PackageStatus, string> = {
  DRAFT: "Bản nháp",
  ACTIVE: "Đang bán",
  SUPERSEDED: "Đã thay thế",
  RETIRED: "Ngừng bán",
};

const USAGE_LABELS: Record<string, string> = {
  draft: "Hồ sơ nháp",
  submitted: "Chờ duyệt",
  under_review: "Đang thẩm định",
  approved: "Đã duyệt",
  rejected: "Từ chối",
  disbursed: "Đã giải ngân",
  active: "Đang gửi",
  matured: "Đáo hạn",
  settled: "Đã tất toán",
};

const ACTION_LABELS: Record<string, string> = {
  publish: "Phát hành",
  retire: "Ngừng bán",
  delete: "Xóa gói",
};

const statusClass = (status: PackageStatus) =>
  status === "ACTIVE"
    ? "border-(--admin-success) bg-(--admin-success-soft) text-(--admin-success)"
    : status === "RETIRED"
      ? "border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)"
      : "border-(--admin-border) bg-(--admin-surface-hover) text-(--admin-text-secondary)";

function StatusBadge({ status }: { status: PackageStatus }) {
  return (
    <span
      className={`rounded-sm border px-1.5 py-0.5 text-[10px] ${statusClass(status)}`}
    >
      {STATUS_LABELS[status]}
    </span>
  );
}

const describeLimits = (p: { minAmount: number; maxAmount?: number }) =>
  p.maxAmount
    ? `${vnd(p.minAmount)} – ${vnd(p.maxAmount)}`
    : `Từ ${vnd(p.minAmount)}`;

const describeTerm = (term: { months: number; annualRateBps?: number }) =>
  term.annualRateBps
    ? `${term.months} tháng: ${percent(term.annualRateBps)}`
    : `${term.months} tháng`;

/** Các dòng khác nhau giữa bản đang bán và bản nháp, để xem trước khi phát hành. */
function diffRows(active: PackageVersion | undefined, draft: PackageVersion) {
  const limits = (p?: PackageVersion) => (p ? describeLimits(p) : "—");
  const terms = (p?: PackageVersion) =>
    p ? p.terms.map(describeTerm).join(", ") : "—";
  return [
    ["Tên gói", active?.name ?? "—", draft.name],
    ["Hạn mức", limits(active), limits(draft)],
    ["Kỳ hạn", terms(active), terms(draft)],
  ].filter(([, before, after]) => before !== after);
}

/** Dạng so sánh được, để biết form có khác bản nháp đã lưu hay không. */
const draftFingerprint = (p: DraftInput | PackageVersion) =>
  JSON.stringify([
    p.name,
    p.description,
    p.minAmount,
    p.maxAmount ?? 0,
    [...p.terms]
      .sort((a, b) => a.months - b.months)
      .map((term) => [term.months, term.annualRateBps ?? 0]),
  ]);

function DraftForm({
  kind,
  draft,
  busy,
  onSave,
  onDirtyChange,
  title = `Bản nháp phiên bản ${draft.version}`,
  saveLabel = "Lưu bản nháp",
  extraValid = true,
  children,
}: {
  kind: PackageKind;
  draft: PackageVersion;
  busy: boolean;
  onSave: (input: DraftInput) => void;
  onDirtyChange?: (dirty: boolean) => void;
  title?: string;
  saveLabel?: string;
  extraValid?: boolean;
  children?: ReactNode;
}) {
  const [name, setName] = useState(draft.name);
  const [description, setDescription] = useState(draft.description);
  const [minAmount, setMinAmount] = useState(draft.minAmount);
  const [maxAmount, setMaxAmount] = useState(draft.maxAmount ?? 0);
  // Giữ chuỗi người dùng đang gõ; đổi sang bps chỉ khi lưu.
  const [rates, setRates] = useState<Record<number, string>>(() =>
    Object.fromEntries(
      draft.terms.map((term) => [
        term.months,
        String((term.annualRateBps ?? 0) / 100),
      ]),
    ),
  );
  const [selected, setSelected] = useState<number[]>(() =>
    draft.terms.map((term) => term.months),
  );

  const toBps = (months: number) =>
    Math.round(Number((rates[months] ?? "").replace(",", ".")) * 100);
  const ratesValid =
    kind === "loan" ||
    selected.every((months) => {
      const bps = toBps(months);
      return Number.isFinite(bps) && bps > 0 && bps <= 10_000;
    });
  const valid =
    name.trim() !== "" &&
    selected.length > 0 &&
    minAmount > 0 &&
    (kind === "savings" || maxAmount >= minAmount) &&
    ratesValid;

  const input: DraftInput = {
    name: name.trim(),
    description,
    minAmount,
    maxAmount: kind === "loan" ? maxAmount : 0,
    terms: selected.map((months) =>
      kind === "loan" ? { months } : { months, annualRateBps: toBps(months) },
    ),
  };
  const dirty = draftFingerprint(input) !== draftFingerprint(draft);
  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

  return (
    <div className="grid gap-3 rounded-sm border border-(--admin-border) p-3">
      <h3 className="text-[13px] font-semibold text-(--admin-text)">{title}</h3>
      {children}
      <label className={fieldLabel}>
        Tên gói
        <input
          className={fieldInput}
          maxLength={120}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </label>
      <label className={fieldLabel}>
        Mô tả
        <input
          className={fieldInput}
          maxLength={1000}
          value={description}
          onChange={(event) => setDescription(event.target.value)}
        />
      </label>
      <div className="grid gap-3 md:grid-cols-2">
        <MoneyField
          label={
            kind === "loan" ? "Số tiền vay tối thiểu" : "Tiền gửi tối thiểu"
          }
          value={minAmount}
          onChange={setMinAmount}
        />
        {kind === "loan" && (
          <MoneyField
            label="Số tiền vay tối đa"
            value={maxAmount}
            onChange={setMaxAmount}
          />
        )}
      </div>
      {kind === "savings" && (
        <p className="text-[11px] text-(--admin-text-muted)">
          Tiền gửi tối thiểu không được thấp hơn 1.000.000 đ.
        </p>
      )}
      <fieldset className="grid gap-1.5">
        <legend className="mb-1 text-[11px] font-semibold text-(--admin-text-muted)">
          Kỳ hạn{kind === "savings" ? " và lãi suất (%/năm)" : ""}
        </legend>
        {TERMS[kind].map((months) => (
          <div key={months} className="flex items-center gap-3 text-[12px]">
            <label className="flex w-28 items-center gap-2 text-(--admin-text)">
              <input
                type="checkbox"
                checked={selected.includes(months)}
                onChange={(event) =>
                  setSelected((old) =>
                    event.target.checked
                      ? [...old, months].sort((a, b) => a - b)
                      : old.filter((value) => value !== months),
                  )
                }
              />
              {months} tháng
            </label>
            {kind === "savings" && selected.includes(months) && (
              <input
                aria-label={`Lãi suất kỳ hạn ${months} tháng`}
                className={`${fieldInput} max-w-24`}
                inputMode="decimal"
                value={rates[months] ?? ""}
                onChange={(event) =>
                  setRates((old) => ({ ...old, [months]: event.target.value }))
                }
              />
            )}
          </div>
        ))}
      </fieldset>
      <div>
        <ActionButton
          variant="secondary"
          disabled={busy || !valid || !extraValid}
          onClick={() => onSave(input)}
        >
          {saveLabel}
        </ActionButton>
      </div>
    </div>
  );
}

function NewPackageForm({
  kind,
  onCreated,
  onCancel,
}: {
  kind: PackageKind;
  onCreated: (code: string) => void;
  onCancel: () => void;
}) {
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const empty: PackageVersion = {
    kind,
    code: "",
    version: 1,
    status: "DRAFT",
    name: "",
    description: "",
    minAmount: 1_000_000,
    maxAmount: kind === "loan" ? 500_000_000 : undefined,
    terms: [],
  };
  const save = async (input: DraftInput) => {
    setBusy(true);
    setError("");
    try {
      const created = await createPackage(kind, {
        ...input,
        code: code.trim(),
      });
      onCreated(created.code);
    } catch (failure) {
      setError(explainProductError(failure));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className={`${panel} grid gap-3 p-4`}>
      {error && <Banner message={error} error />}
      <DraftForm
        kind={kind}
        draft={empty}
        busy={busy}
        title={kind === "loan" ? "Gói vay mới" : "Gói tiết kiệm mới"}
        saveLabel="Tạo gói"
        extraValid={/^[A-Za-z0-9][A-Za-z0-9_-]{2,63}$/.test(code.trim())}
        onSave={(input) => void save(input)}
      >
        <label className={fieldLabel}>
          Mã gói
          <input
            className={fieldInput}
            maxLength={64}
            value={code}
            onChange={(event) => setCode(event.target.value.toUpperCase())}
          />
        </label>
        <p className="text-[11px] text-(--admin-text-muted)">
          Từ 3 đến 64 ký tự: chữ, số, gạch ngang hoặc gạch dưới. Không đổi được
          sau khi tạo.
        </p>
      </DraftForm>
      <p className="text-[11px] text-(--admin-text-muted)">
        Gói được lưu dưới dạng bản nháp, khách chỉ thấy sau khi phát hành.
      </p>
      <div>
        <ActionButton variant="secondary" disabled={busy} onClick={onCancel}>
          Hủy
        </ActionButton>
      </div>
    </section>
  );
}

function Simulator({
  kind,
  terms,
}: {
  kind: PackageKind;
  terms: PackageVersion["terms"];
}) {
  const [amount, setAmount] = useState(50_000_000);
  const [income, setIncome] = useState(15_000_000);
  const [region, setRegion] = useState("KV01");
  const [months, setMonths] = useState(terms[0]?.months ?? 12);
  const [result, setResult] = useState("");
  const [error, setError] = useState("");

  const run = async () => {
    setError("");
    setResult("");
    try {
      if (kind === "loan") {
        const simulated = await simulateLoan({
          amount,
          monthlyIncome: income,
          regionCode: region,
          termMonths: months,
        });
        setResult(
          `Lãi ${percent(simulated.annualRateBps)}/năm, trả mỗi tháng tối đa ${vnd(simulated.monthlyPayment)}, tổng lãi ${vnd(simulated.totalInterest)}.`,
        );
      } else {
        const bps = terms.find((t) => t.months === months)?.annualRateBps ?? 0;
        const simulated = await simulateSavings({
          amount,
          annualRateBps: bps,
          termMonths: months,
        });
        setResult(
          `Lãi ${percent(bps)}/năm, tiền lãi ${vnd(simulated.interest)}, nhận khi đáo hạn ${vnd(simulated.total)}.`,
        );
      }
    } catch (failure) {
      setError(explainProductError(failure));
    }
  };

  return (
    <details className="rounded-sm border border-(--admin-border) p-3">
      <summary className="cursor-pointer text-[12px] font-semibold text-(--admin-text)">
        Mô phỏng
      </summary>
      <div className="mt-3 grid gap-3 md:grid-cols-2">
        <MoneyField label="Số tiền" value={amount} onChange={setAmount} />
        <label className={fieldLabel}>
          Kỳ hạn
          <select
            className={fieldInput}
            value={months}
            onChange={(event) => setMonths(Number(event.target.value))}
          >
            {terms.map((term) => (
              <option key={term.months} value={term.months}>
                {term.months} tháng
              </option>
            ))}
          </select>
        </label>
        {kind === "loan" && (
          <>
            <MoneyField
              label="Thu nhập mỗi tháng"
              value={income}
              onChange={setIncome}
            />
            <label className={fieldLabel}>
              Khu vực
              <select
                className={fieldInput}
                value={region}
                onChange={(event) => setRegion(event.target.value)}
              >
                <option value="KV01">HCM, Hà Nội, Cần Thơ</option>
                <option value="KV02">Huế, Đà Nẵng, Hải Phòng</option>
                <option value="KV03">Khu vực khác</option>
              </select>
            </label>
          </>
        )}
      </div>
      <div className="mt-3 grid gap-2">
        <div>
          <ActionButton variant="secondary" onClick={() => void run()}>
            Tính thử
          </ActionButton>
        </div>
        {kind === "loan" && (
          <p className="text-[11px] text-(--admin-text-muted)">
            Lãi vay do chính sách thẩm định quyết định theo thu nhập, khu vực và
            kỳ hạn; không cấu hình theo gói.
          </p>
        )}
        {result && <Banner message={result} />}
        {error && <Banner message={error} error />}
      </div>
    </details>
  );
}

function PackageWorkspace({
  kind,
  code,
  onChanged,
  onRemoved,
}: {
  kind: PackageKind;
  code: string;
  onChanged: () => void;
  onRemoved: () => void;
}) {
  const [detail, setDetail] = useState<PackageDetail | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [retireReason, setRetireReason] = useState("");
  const [unsaved, setUnsaved] = useState(false);
  // Giữ khóa đến khi có kết quả rõ ràng để bấm lại không phát hành hai lần.
  const pendingKey = useRef<{ scope: string; key: string } | null>(null);
  const keyFor = (scope: string) => {
    if (pendingKey.current?.scope !== scope)
      pendingKey.current = { scope, key: crypto.randomUUID() };
    return pendingKey.current.key;
  };

  const load = useCallback(async () => {
    setDetail(await getPackage(kind, code));
  }, [kind, code]);

  useEffect(() => {
    let active = true;
    getPackage(kind, code)
      .then((loaded) => {
        if (active) setDetail(loaded);
      })
      .catch((failure: unknown) => {
        if (active) setError(explainProductError(failure));
      });
    return () => {
      active = false;
    };
  }, [kind, code]);

  const run = async (action: () => Promise<unknown>, success: string) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await action();
      pendingKey.current = null;
      setMessage(success);
      await load();
      onChanged();
    } catch (failure) {
      setError(explainProductError(failure));
    } finally {
      setBusy(false);
    }
  };

  const removePackage = async () => {
    if (!window.confirm("Xóa gói này khỏi danh mục?")) return;
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await deletePackage(kind, code);
      onRemoved();
    } catch (failure) {
      setError(explainProductError(failure));
      setBusy(false);
    }
  };

  if (!detail) {
    return (
      <section className={`${panel} p-4`}>
        {error ? (
          <Banner message={error} error />
        ) : (
          <p role="status" className="text-[12px] text-(--admin-text-muted)">
            Đang tải gói...
          </p>
        )}
      </section>
    );
  }

  const draft = detail.versions.find((v) => v.status === "DRAFT");
  const active = detail.versions.find((v) => v.status === "ACTIVE");
  const shown = active ?? draft ?? detail.versions[0];
  const diff = draft ? diffRows(active, draft) : [];
  const usage = Object.entries(detail.usage);
  const used = usage.some(([, count]) => count > 0);
  // Gói chưa từng phát hành: xóa bản nháp cũng là xóa gói, chỉ giữ một nút.
  const onlyDraft = Boolean(draft) && detail.versions.length === 1;

  return (
    <section className={`${panel} grid gap-4 p-4`}>
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 className="text-[16px] font-bold text-(--admin-text)">
            {detail.name}
          </h2>
          <p className="font-(family-name:--admin-font-mono) text-[10px] text-(--admin-text-muted)">
            {detail.code}
          </p>
        </div>
        <StatusBadge status={detail.status} />
      </div>
      {error && <Banner message={error} error />}
      {message && <Banner message={message} />}

      <dl className="grid gap-3 text-[12px] md:grid-cols-2">
        <div>
          <dt className="text-[11px] text-(--admin-text-muted)">Hạn mức</dt>
          <dd className="font-semibold text-(--admin-text)">
            {describeLimits(shown)}
          </dd>
        </div>
        <div>
          <dt className="text-[11px] text-(--admin-text-muted)">Kỳ hạn</dt>
          <dd className="font-semibold text-(--admin-text)">
            {shown.terms.map(describeTerm).join(", ")}
          </dd>
        </div>
        <div className="md:col-span-2">
          <dt className="text-[11px] text-(--admin-text-muted)">
            Đang sử dụng
          </dt>
          <dd className="text-(--admin-text)">
            {usage.length === 0
              ? "Chưa có"
              : usage
                  .map(
                    ([status, count]) =>
                      `${USAGE_LABELS[status] ?? status}: ${count}`,
                  )
                  .join(" · ")}
          </dd>
        </div>
      </dl>

      {draft ? (
        <>
          <DraftForm
            key={draft.version}
            kind={kind}
            draft={draft}
            busy={busy}
            onDirtyChange={setUnsaved}
            onSave={(input) =>
              void run(() => updateDraft(kind, code, input), "Đã lưu bản nháp.")
            }
          />
          <div className="grid gap-2 rounded-sm border border-(--admin-border) p-3">
            <h3 className="text-[13px] font-semibold text-(--admin-text)">
              So với bản đang bán
            </h3>
            {diff.length === 0 ? (
              <p className="text-[11px] text-(--admin-text-muted)">
                Chưa có khác biệt đã lưu.
              </p>
            ) : (
              <table className="w-full border-collapse">
                <thead>
                  <tr>
                    <th className={tableHead}>Mục</th>
                    <th className={tableHead}>Hiện tại</th>
                    <th className={tableHead}>Sau khi phát hành</th>
                  </tr>
                </thead>
                <tbody>
                  {diff.map(([label, before, after]) => (
                    <tr key={label}>
                      <td className={tableCell}>{label}</td>
                      <td className={tableCell}>{before}</td>
                      <td className={tableCell}>{after}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            <p className="text-[11px] text-(--admin-text-muted)">
              Có hiệu lực ngay với khách mới. Hợp đồng và hồ sơ đã tạo không bị
              ảnh hưởng.
            </p>
            {unsaved && (
              <Banner message="Có thay đổi chưa lưu. Hãy lưu bản nháp trước khi phát hành." />
            )}
            <div className="flex flex-wrap gap-2">
              <ActionButton
                disabled={busy || unsaved}
                onClick={() => {
                  if (window.confirm("Phát hành bản nháp này?"))
                    void run(
                      () =>
                        publishPackage(
                          kind,
                          code,
                          draft.version,
                          keyFor(`publish:${code}:${draft.version}`),
                        ),
                      "Đã phát hành.",
                    );
                }}
              >
                Phát hành
              </ActionButton>
              {!onlyDraft && (
                <ActionButton
                  variant="secondary"
                  disabled={busy}
                  onClick={() => {
                    if (window.confirm("Xóa bản nháp này?"))
                      void run(
                        () => deleteDraft(kind, code),
                        "Đã xóa bản nháp.",
                      );
                  }}
                >
                  Xóa bản nháp
                </ActionButton>
              )}
            </div>
          </div>
        </>
      ) : (
        <div>
          <ActionButton
            variant="secondary"
            disabled={busy}
            onClick={() =>
              void run(
                () => createDraft(kind, code),
                "Hãy sửa thông tin rồi lưu và phát hành. Khách vẫn thấy bản cũ cho đến khi phát hành.",
              )
            }
          >
            Sửa gói
          </ActionButton>
        </div>
      )}

      {active && (
        <div className="grid gap-2 rounded-sm border border-(--admin-border) p-3">
          <label className={fieldLabel}>
            Lý do ngừng bán
            <input
              className={fieldInput}
              maxLength={500}
              value={retireReason}
              onChange={(event) => setRetireReason(event.target.value)}
            />
          </label>
          <p className="text-[11px] text-(--admin-text-muted)">
            Khách không mở được khoản mới. Sổ và khoản vay đang có vẫn hoạt
            động, kể cả rút tiền khi đáo hạn.
          </p>
          <div>
            <ActionButton
              variant="danger"
              disabled={busy || !retireReason.trim()}
              onClick={() => {
                if (window.confirm("Ngừng bán gói này?"))
                  void run(async () => {
                    await retirePackage(
                      kind,
                      code,
                      active.version,
                      retireReason.trim(),
                      keyFor(`retire:${code}:${active.version}`),
                    );
                    setRetireReason("");
                  }, "Đã ngừng bán.");
              }}
            >
              Ngừng bán
            </ActionButton>
          </div>
        </div>
      )}

      <div className="grid gap-2 rounded-sm border border-(--admin-border) p-3">
        <p className="text-[11px] text-(--admin-text-muted)">
          {active
            ? "Gói đang bán. Muốn xóa, hãy ngừng bán trước."
            : used
              ? "Gói đã có hồ sơ hoặc hợp đồng nên không xóa được."
              : "Gói chưa có hồ sơ hay hợp đồng nào nên có thể xóa."}
        </p>
        <div>
          <ActionButton
            variant="danger"
            disabled={busy || Boolean(active) || used}
            onClick={() => void removePackage()}
          >
            Xóa gói
          </ActionButton>
        </div>
      </div>

      <Simulator kind={kind} terms={shown.terms} />

      <div className="grid gap-1.5">
        <h3 className="text-[13px] font-semibold text-(--admin-text)">
          Các phiên bản
        </h3>
        <table className="w-full border-collapse">
          <thead>
            <tr>
              <th className={tableHead}>Phiên bản</th>
              <th className={tableHead}>Trạng thái</th>
              <th className={tableHead}>Hạn mức</th>
              <th className={tableHead}>Phát hành</th>
            </tr>
          </thead>
          <tbody>
            {detail.versions.map((version) => (
              <tr key={version.version}>
                <td className={tableCell}>{version.version}</td>
                <td className={tableCell}>
                  <StatusBadge status={version.status} />
                </td>
                <td className={tableCell}>{describeLimits(version)}</td>
                <td className={tableCell}>{date(version.publishedAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {detail.audit.length > 0 && (
        <div className="grid gap-1.5">
          <h3 className="text-[13px] font-semibold text-(--admin-text)">
            Lịch sử thay đổi
          </h3>
          <ul className="grid gap-1 text-[11px] text-(--admin-text-muted)">
            {detail.audit.map((entry) => (
              <li key={`${entry.action}-${entry.version}-${entry.at}`}>
                {date(entry.at)} · {ACTION_LABELS[entry.action] ?? entry.action}{" "}
                phiên bản {entry.version}
                {entry.reason ? ` · ${entry.reason}` : ""}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

export function PackagesPage({ kind }: { kind: PackageKind }) {
  const [items, setItems] = useState<PackageSummary[]>([]);
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [tab, setTab] = useState<"packages" | "terms">("packages");
  const [creating, setCreating] = useState(false);
  const [page, setPage] = useState(1);
  // Danh sách có thể ngắn đi sau khi xóa gói.
  const lastPage = Math.max(1, Math.ceil(items.length / PACKAGES_PER_PAGE));
  const shownPage = Math.min(page, lastPage);
  const pageItems = items.slice(
    (shownPage - 1) * PACKAGES_PER_PAGE,
    shownPage * PACKAGES_PER_PAGE,
  );

  const refresh = useCallback(() => {
    listPackages(kind)
      .then((loaded) => {
        setItems(loaded);
        setCode((old) => old || loaded[0]?.code || "");
        setError("");
      })
      .catch((failure: unknown) => setError(explainProductError(failure)))
      .finally(() => setLoading(false));
  }, [kind]);

  useEffect(refresh, [refresh]);

  const isSavings = kind === "savings";
  return (
    <div className="grid gap-3">
      <PageHeader
        title={isSavings ? "Gói tiết kiệm" : "Gói vay"}
        description="Tạo, sửa, phát hành, ngừng bán và xóa gói. Mỗi lần sửa tạo phiên bản mới và không ảnh hưởng hợp đồng đang chạy."
      />
      {error && <Banner message={error} error />}
      {isSavings && (
        <div
          role="tablist"
          className="flex gap-1 border-b border-(--admin-border)"
        >
          {(
            [
              ["packages", "Gói"],
              ["terms", "Điều khoản"],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={tab === id}
              onClick={() => setTab(id)}
              className={`-mb-px border-b-2 px-3 py-2 text-[12px] font-semibold ${
                tab === id
                  ? "border-(--admin-primary) text-(--admin-text)"
                  : "border-transparent text-(--admin-text-muted)"
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      )}
      {isSavings && tab === "terms" ? (
        <SavingsTermsPanel />
      ) : (
        <div className="grid items-start gap-3 xl:grid-cols-[minmax(280px,0.7fr)_minmax(0,1.6fr)]">
          <section className={`${panel} grid gap-1.5 p-4`}>
            <div className="mb-1">
              <ActionButton onClick={() => setCreating(true)}>
                Tạo gói mới
              </ActionButton>
            </div>
            {items.length === 0 && (
              <p className="py-6 text-center text-[11px] text-(--admin-text-muted)">
                {loading ? "Đang tải gói..." : "Chưa có gói nào."}
              </p>
            )}
            {pageItems.map((item) => (
              <button
                key={item.code}
                type="button"
                onClick={() => {
                  setCreating(false);
                  setCode(item.code);
                }}
                className={`grid gap-1 rounded-sm border px-3 py-2 text-left ${
                  item.code === code
                    ? "border-(--admin-primary) bg-(--admin-primary-soft)"
                    : "border-(--admin-border) bg-(--admin-background) hover:border-(--admin-primary)"
                }`}
              >
                <span className="flex items-center justify-between gap-2">
                  <strong className="text-[13px] text-(--admin-text)">
                    {item.name}
                  </strong>
                  <StatusBadge status={item.status} />
                </span>
                <span className="text-[11px] text-(--admin-text-muted)">
                  {describeLimits(item)}
                  {item.draftVersion ? " · Có bản nháp" : ""}
                </span>
              </button>
            ))}
            <Pager
              page={shownPage}
              hasNext={shownPage < lastPage}
              onPrevious={() => setPage(shownPage - 1)}
              onNext={() => setPage(shownPage + 1)}
            />
          </section>
          {creating ? (
            <NewPackageForm
              kind={kind}
              onCancel={() => setCreating(false)}
              onCreated={(created) => {
                setCreating(false);
                setCode(created);
                refresh();
              }}
            />
          ) : (
            code && (
              <PackageWorkspace
                key={code}
                kind={kind}
                code={code}
                onChanged={refresh}
                onRemoved={() => {
                  setCode("");
                  refresh();
                }}
              />
            )
          )}
        </div>
      )}
    </div>
  );
}
