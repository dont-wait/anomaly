import { useState, type ReactNode } from "react";
import { navigate, routes } from "@/app/routes";
import { ArrowLeftIcon, ChevronRightIcon, SavingsIcon } from "@/shared/icons";
import { BottomNav } from "@/shared/layout/BottomNav";
import { Logo } from "@/shared/layout/Logo";
import { SelectSheet } from "@/shared/ui";
import { DEMO_ACCOUNT_BALANCE, DEMO_SOURCE_ACCOUNT, demoSavings } from "./mocks/savings";
import { SAVINGS_TERMS, estimateSavings, formatDate, formatVnd, parseAmount } from "./utils/format";
import type { SavingsContract, SavingsDraft, SavingsStatus } from "./model";
import "./savings.css";

type Screen = "list" | "open" | "open-preview" | "open-success" | "detail" | "withdraw-preview" | "withdraw-success";
type Filter = "all" | SavingsStatus;

const labels: Record<SavingsStatus, string> = {
  active: "Đang gửi",
  matured: "Đã đáo hạn",
  settled: "Đã tất toán",
};

const DEMO_LABEL = "Bản xem trước giao diện · Dữ liệu mô phỏng · Không giao dịch thật";

function Button({ children, onClick, secondary = false, disabled = false }: {
  children: ReactNode;
  onClick: () => void;
  secondary?: boolean;
  disabled?: boolean;
}) {
  return (
    <button type="button" onClick={onClick} disabled={disabled}
      className={`savings-button ${secondary ? "savings-button-secondary" : ""}`}>
      {children}
    </button>
  );
}

function Section({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <section className={`savings-panel ${className}`}>{children}</section>;
}

function Line({ label, value, strong = false }: { label: string; value: ReactNode; strong?: boolean }) {
  return <div className={`savings-line ${strong ? "savings-line-strong" : ""}`}>
    <span>{label}</span><span>{value}</span>
  </div>;
}

function Badge({ status }: { status: SavingsStatus }) {
  return <span className={`savings-status savings-status-${status}`}>{labels[status]}</span>;
}

function Head({ title, onBack, subtitle }: { title: string; onBack: () => void; subtitle?: string }) {
  return <header className="savings-topbar">
    <button type="button" className="savings-back" onClick={onBack} aria-label="Quay lại">
      <ArrowLeftIcon className="h-5 w-5" />
    </button>
    <div className="savings-topbar-copy"><h1>{title}</h1>{subtitle && <p>{subtitle}</p>}</div>
    <Logo className="savings-logo" />
  </header>;
}

function SuccessMark() {
  return <div className="savings-success-mark" aria-hidden="true"><span>✓</span></div>;
}

export default function SavingsPage() {
  const [screen, setScreen] = useState<Screen>("list");
  const [contracts, setContracts] = useState<SavingsContract[]>(() => demoSavings.map((c) => ({ ...c })));
  const [filter, setFilter] = useState<Filter>("all");
  const [selectedId, setSelectedId] = useState(demoSavings[0].id);
  const [amountText, setAmountText] = useState("10000000");
  const [termMonths, setTermMonths] = useState(12);
  const [accepted, setAccepted] = useState(false);
  const [draft, setDraft] = useState<SavingsDraft | null>(null);
  const [withdrawAccepted, setWithdrawAccepted] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const selected = contracts.find((c) => c.id === selectedId) ?? null;
  const principalTotal = contracts.filter((c) => c.status !== "settled").reduce((sum, c) => sum + c.principal, 0);
  const projectedInterest = contracts.filter((c) => c.status !== "settled").reduce((sum, c) => sum + c.interest, 0);
  const count = (status: Filter) => contracts.filter((c) => status === "all" || c.status === status).length;
  const visible = contracts.filter((c) => filter === "all" || c.status === filter);
  const amount = parseAmount(amountText);
  const canOpen = Number.isSafeInteger(amount) && amount >= 1_000_000 && amount <= DEMO_ACCOUNT_BALANCE;

  const showContract = (id: string) => { setSelectedId(id); setScreen("detail"); };
  const startOpen = () => { setAccepted(false); setFormError(null); setScreen("open"); };
  const previewOpen = () => {
    try {
      if (amount > DEMO_ACCOUNT_BALANCE) throw new Error("Số dư mô phỏng không đủ để gửi");
      const quote = estimateSavings(amount, termMonths, new Date().toISOString());
      setDraft(quote); setAccepted(false); setFormError(null); setScreen("open-preview");
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Giá trị không hợp lệ");
    }
  };
  const confirmOpen = () => {
    if (!accepted || !draft) return;
    const newContract: SavingsContract = {
      id: `demo-created-${contracts.length + 1}`,
      number: `TK-DEMO-${String(contracts.length + 1).padStart(4, "0")}`,
      principal: draft.amount,
      termMonths: draft.termMonths,
      annualRate: draft.annualRate,
      openedAt: draft.openedAt,
      maturityAt: draft.maturityAt,
      status: "active",
      sourceAccount: DEMO_SOURCE_ACCOUNT,
      interest: draft.interest,
    };
    setContracts((current) => [newContract, ...current]);
    setSelectedId(newContract.id);
    setScreen("open-success");
  };
  const confirmWithdraw = () => {
    if (!withdrawAccepted || !selected || selected.status !== "matured") return;
    setContracts((current) => current.map((item) => item.id === selected.id ? {
      ...item, status: "settled", settledAt: new Date().toISOString(),
    } : item));
    setScreen("withdraw-success");
  };

  return <div className="savings-shell">
    <div className="savings-demo-banner" role="status">{DEMO_LABEL}</div>
    {screen === "list" ? <>
      <Head title="Tiết kiệm của tôi" subtitle="Tích lũy hôm nay, vững vàng tương lai" onBack={() => navigate(routes.dashboard)} />
      <main className="savings-content">
        <section className="savings-hero">
          <div className="savings-hero-top"><span className="savings-hero-eyebrow"><SavingsIcon className="h-5 w-5" /> Tiết kiệm có kỳ hạn</span><span className="savings-hero-pill">◈ Chuẩn VND</span></div>
          <strong className="savings-hero-amount">{formatVnd(principalTotal)}</strong>
          <p>{count("active") + count("matured")} sổ tiết kiệm chưa tất toán</p>
          <div className="savings-hero-metrics">
            <span>Đang gửi <b>{count("active")} sổ</b></span>
            <span>Tổng lãi khi đáo hạn <b>+{formatVnd(projectedInterest)}</b></span>
          </div>
        </section>
        <Button onClick={startOpen}>⊕ &nbsp; Gửi tiết kiệm mới</Button>
        <div className="savings-section-title"><h2>Sổ tiết kiệm của bạn</h2><span>{contracts.length} hợp đồng</span></div>
        <div className="savings-filters" aria-label="Lọc sổ tiết kiệm">
          {(["all", "active", "matured", "settled"] as const).map((key) => <button key={key} type="button" aria-pressed={filter === key}
            onClick={() => setFilter(key)} className={filter === key ? "savings-filter-active" : ""}>
            {key === "all" ? "Tất cả" : labels[key]} ({count(key)})
          </button>)}
        </div>
        <div className="savings-contracts">
          {visible.length === 0 && <Section><p className="savings-muted">Chưa có sổ tiết kiệm nào ở trạng thái này.</p></Section>}
          {visible.map((item) => <button type="button" key={item.id} className="savings-contract" onClick={() => showContract(item.id)}>
            <div className="savings-contract-head"><span>#{item.number}</span><Badge status={item.status} /></div>
            <span className="savings-muted">Tiền gốc gửi</span><div className="savings-contract-amount">{formatVnd(item.principal)}</div>
            <div className="savings-contract-meta"><span>Lãi suất <b>{item.annualRate}%/năm</b></span><span>Kỳ hạn <b>{item.termMonths} tháng</b></span></div>
            <div className="savings-progress"><span style={{ width: item.status === "active" ? "35%" : "100%" }} /></div>
            <div className="savings-contract-dates"><span>Ngày mở <b>{formatDate(item.openedAt)}</b></span><span>Ngày đáo hạn <b>{formatDate(item.maturityAt)}</b></span></div>
            <div className="savings-card-action">{item.status === "matured" ? "Sẵn sàng rút gốc & lãi" : "Xem chi tiết sổ"}<ChevronRightIcon className="h-4 w-4" /></div>
          </button>)}
        </div>
      </main>
      <BottomNav active="savings" onChange={(id) => { if (id === "home") navigate(routes.dashboard); }} />
    </> : null}

    {screen === "open" && <>
      <Head title="Mở sổ tiết kiệm" onBack={() => setScreen("list")} />
      <main className="savings-content">
        <section className="savings-banner"><span className="savings-overline">TÍCH LŨY</span><h2>Tiết kiệm có kỳ hạn</h2><p>Lãi suất cố định, an tâm với kế hoạch tương lai.</p></section>
        <div className="savings-section-title"><h2>Tài khoản trích tiền</h2><span>Khả dụng</span></div>
        <Section><Line label="Tài khoản thanh toán" value={DEMO_SOURCE_ACCOUNT} /><Line label="Số dư mô phỏng" value={formatVnd(DEMO_ACCOUNT_BALANCE)} /></Section>
        <div className="savings-section-title"><h2>Số tiền gửi</h2><span>Từ 1.000.000 VND</span></div>
        <Section>
          <label htmlFor="savings-amount" className="savings-muted">Nhập số tiền (VND)</label>
          <input id="savings-amount" className="savings-amount-input" inputMode="numeric" value={amountText}
            onChange={(event) => { setAmountText(event.target.value); setFormError(null); }} />
          <div className="savings-amount-chips">{[1_000_000, 5_000_000, 10_000_000].map((value) => <button type="button" key={value} onClick={() => { setAmountText(String(value)); setFormError(null); }}>{formatVnd(value)}</button>)}</div>
        </Section>
        <div className="savings-section-title"><h2>Chọn kỳ hạn</h2><span>Lãi suất cố định</span></div>
        <Section><label className="savings-muted">Kỳ hạn gửi</label>
          <SelectSheet id="savings-term" label="Kỳ hạn gửi" value={String(termMonths)} onChange={(value) => setTermMonths(Number(value))}
            options={SAVINGS_TERMS.map(({ months, rate }) => ({ value: String(months), label: `${months} tháng`, description: `Lãi suất ${rate}%/năm` }))} />
        </Section>
        <Section className="savings-summary"><h3>Ước tính lợi nhuận</h3>
          <Line label="Tiền gốc" value={Number.isFinite(amount) ? formatVnd(amount) : "—"} />
          <Line label="Lãi suất năm" value={`${SAVINGS_TERMS.find((t) => t.months === termMonths)?.rate}%`} />
          <Line label="Tiền lãi dự kiến" value={canOpen ? formatVnd(estimateSavings(amount, termMonths, new Date().toISOString()).interest) : "—"} />
        </Section>
        {formError && <p role="alert" className="savings-error">{formError}</p>}
        {!canOpen && <p className="savings-hint">Nhập số tiền từ 1.000.000 VND và không vượt số dư mô phỏng.</p>}
        <Button onClick={previewOpen} disabled={!canOpen}>Tiếp tục <span aria-hidden="true">→</span></Button>
      </main>
    </>}

    {screen === "open-preview" && draft && <>
      <Head title="Xác nhận gửi tiết kiệm" onBack={() => setScreen("open")} />
      <main className="savings-content">
        <section className="savings-amount-hero"><span>Số tiền gửi</span><strong>{formatVnd(draft.amount)}</strong><small>Tài khoản nguồn: {DEMO_SOURCE_ACCOUNT}</small></section>
        <Section><h3>Thông tin chi tiết</h3>
          <Line label="Sản phẩm" value="Tiết kiệm có kỳ hạn" />
          <Line label="Thời hạn" value={`${draft.termMonths} tháng`} />
          <Line label="Lãi suất cố định" value={`${draft.annualRate}%/năm`} />
          <Line label="Ngày bắt đầu" value={formatDate(draft.openedAt)} />
          <Line label="Ngày đáo hạn dự kiến" value={formatDate(draft.maturityAt)} />
          <Line label="Tiền lãi dự kiến" value={`+${formatVnd(draft.interest)}`} />
          <Line label="Tổng nhận khi đáo hạn" strong value={formatVnd(draft.amount + draft.interest)} />
        </Section>
        <Section className="savings-info"><h3>Lưu ý quan trọng</h3><p>Không hỗ trợ rút trước hạn hoặc rút một phần. Lãi hiển thị chỉ là ước tính UI, chưa phải báo giá API.</p></Section>
        <label className="savings-checkbox"><input type="checkbox" checked={accepted} onChange={(e) => setAccepted(e.target.checked)} /> Tôi đã đọc và đồng ý với các điều kiện gửi tiết kiệm mô phỏng.</label>
        <Button onClick={confirmOpen} disabled={!accepted}>Xác nhận gửi {formatVnd(draft.amount)}</Button>
      </main>
    </>}

    {screen === "open-success" && selected && <>
      <Head title="Kết quả gửi tiết kiệm" onBack={() => setScreen("list")} />
      <main className="savings-content">
        <SuccessMark /><h2 className="savings-success-title">Gửi tiết kiệm thành công!</h2>
        <p className="savings-center savings-muted">Đây là kết quả mô phỏng giao diện, chưa có giao dịch ngân hàng.</p>
        <section className="savings-amount-hero"><span>Số tiền gửi</span><strong>{formatVnd(selected.principal)}</strong></section>
        <Section><h3>Biên nhận mô phỏng</h3>
          <Line label="Mã sổ" value={selected.number} />
          <Line label="Tiền gốc" value={formatVnd(selected.principal)} />
          <Line label="Kỳ hạn" value={`${selected.termMonths} tháng`} />
          <Line label="Lãi suất" value={`${selected.annualRate}%/năm`} />
          <Line label="Đáo hạn" value={formatDate(selected.maturityAt)} />
          <Line label="Dự kiến nhận" strong value={formatVnd(selected.principal + selected.interest)} />
        </Section>
        <Button onClick={() => setScreen("detail")}>Xem sổ tiết kiệm</Button>
        <Button secondary onClick={() => setScreen("list")}>Về trang tiết kiệm</Button>
      </main>
    </>}

    {screen === "detail" && selected && <>
      <Head title="Chi tiết sổ tiết kiệm" onBack={() => setScreen("list")} />
      <main className="savings-content">
        <Section className="savings-detail-hero">
          <div className="savings-contract-head"><span>#{selected.number}</span><Badge status={selected.status} /></div>
          <span className="savings-muted">Tiền gốc gửi</span>
          <div className="savings-detail-amount">{formatVnd(selected.principal)}</div>
          <div className="savings-contract-meta"><span>Kỳ hạn <b>{selected.termMonths} tháng</b></span><span>Lãi suất <b>{selected.annualRate}%/năm</b></span></div>
        </Section>
        <Section><h3>Tiến độ kỳ hạn</h3>
          <div className="savings-progress"><span style={{ width: selected.status === "active" ? "35%" : "100%" }} /></div>
          <Line label="Ngày mở" value={formatDate(selected.openedAt)} />
          <Line label="Đáo hạn" value={formatDate(selected.maturityAt)} />
          <p className="savings-hint">{selected.status === "active" ? "Sổ đang trong kỳ hạn gửi." : selected.status === "matured" ? "Sổ đã đến hạn, có thể xem trước tất toán." : "Sổ đã tất toán trong bản mô phỏng."}</p>
        </Section>
        <Section><h3>Bảng chi tiết thanh toán</h3>
          <Line label="Tổng gốc ban đầu" value={formatVnd(selected.principal)} />
          <Line label="Tiền lãi đến hạn" value={`+${formatVnd(selected.interest)}`} />
          <Line label="Tổng nhận khi đáo hạn" strong value={formatVnd(selected.principal + selected.interest)} />
          <Line label="Tài khoản nhận" value={selected.sourceAccount} />
        </Section>
        <Section className="savings-info"><h3>Lưu ý tất toán</h3><p>Chỉ tất toán toàn bộ khi đã đủ kỳ hạn, tiền về tài khoản nguồn. Trạng thái và quyền rút thật phải do backend xác nhận.</p></Section>
        {selected.status === "matured" ? <Button onClick={() => { setWithdrawAccepted(false); setScreen("withdraw-preview"); }}>Rút tiền tiết kiệm</Button> :
          <Button disabled onClick={() => {}}> {selected.status === "settled" ? "Đã tất toán" : "Chưa đến ngày đáo hạn"} </Button>}
        <Button secondary onClick={() => setScreen("list")}>Về trang tiết kiệm</Button>
      </main>
    </>}

    {screen === "withdraw-preview" && selected && selected.status === "matured" && <>
      <Head title="Xác nhận rút tiết kiệm" onBack={() => setScreen("detail")} />
      <main className="savings-content">
        <section className="savings-amount-hero"><span>Tổng tiền dự kiến nhận</span>
          <strong>{formatVnd(selected.principal + selected.interest)}</strong><small>Toàn bộ gốc + lãi đến hạn</small>
        </section>
        <Section><h3>Chi tiết tất toán</h3>
          <Line label="Mã sổ" value={selected.number} />
          <Line label="Tiền gốc" value={formatVnd(selected.principal)} />
          <Line label="Tiền lãi" value={`+${formatVnd(selected.interest)}`} />
          <Line label="Lãi suất" value={`${selected.annualRate}%/năm`} />
          <Line label="Ngày đáo hạn" value={formatDate(selected.maturityAt)} />
          <Line label="Phí tất toán" value="0 VND (mock)" />
          <Line label="Tổng cộng" strong value={formatVnd(selected.principal + selected.interest)} />
        </Section>
        <Section className="savings-account"><h3>Tài khoản nhận tiền</h3><p>Tài khoản thanh toán</p><strong>{selected.sourceAccount}</strong></Section>
        <Section className="savings-info"><h3>Thông tin quan trọng</h3><p>Rút toàn bộ, không rút một phần. Lãi chỉ tính đến ngày đáo hạn. Nút xác nhận chỉ thay đổi mock data trong phiên UI này.</p></Section>
        <label className="savings-checkbox"><input type="checkbox" checked={withdrawAccepted} onChange={(e) => setWithdrawAccepted(e.target.checked)} /> Tôi xác nhận xem trước tất toán sổ mô phỏng này.</label>
        <Button onClick={confirmWithdraw} disabled={!withdrawAccepted}>Xác nhận rút tiền</Button>
      </main>
    </>}

    {screen === "withdraw-success" && selected && selected.status === "settled" && <>
      <Head title="Kết quả rút tiết kiệm" onBack={() => setScreen("list")} />
      <main className="savings-content">
        <SuccessMark /><h2 className="savings-success-title">Rút tiết kiệm thành công!</h2>
        <p className="savings-center savings-muted">Chỉ mô phỏng UI; tiền không được cộng vào tài khoản thật.</p>
        <section className="savings-amount-hero"><span>Tổng tiền tất toán mô phỏng</span><strong>{formatVnd(selected.principal + selected.interest)}</strong></section>
        <Section><h3>Biên nhận mô phỏng</h3>
          <Line label="Mã sổ" value={selected.number} />
          <Line label="Trạng thái" value={<Badge status="settled" />} />
          <Line label="Tổng gốc" value={formatVnd(selected.principal)} />
          <Line label="Tiền lãi" value={`+${formatVnd(selected.interest)}`} />
          <Line label="Tổng tiền nhận" strong value={formatVnd(selected.principal + selected.interest)} />
          <Line label="Tài khoản nhận" value={selected.sourceAccount} />
          <Line label="Thời điểm" value={formatDate(selected.settledAt ?? new Date().toISOString())} />
        </Section>
        <Button onClick={() => setScreen("detail")}>Xem biên nhận</Button>
        <Button secondary onClick={() => { setFilter("all"); setScreen("list"); }}>Về trang tiết kiệm</Button>
      </main>
    </>}
  </div>;
}
