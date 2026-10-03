import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowLeft,
  faShieldHalved,
  faFingerprint,
} from "@fortawesome/free-solid-svg-icons";
import { stages, titles } from "./model";
import { useRegistrationFlow } from "./useRegistrationFlow";
import { RegistrationStep } from "./RegistrationStep";
import { InfoDialog } from "./components/InfoDialog";
import "./registration.css";

export function RegistrationFlow({ onLogin }: { onLogin: () => void }) {
  const flow = useRegistrationFlow();
  const { screen, busy, createdAccount, dialog, setDialog, step, go } = flow;
  return (
    <div className="auth-app">
      <aside className="brand-panel">
        <a
          className="brand"
          href="#"
          onClick={(e) => {
            e.preventDefault();
            go("email");
          }}
        >
          <span className="brand-mark">A</span>AnomalyBank
        </a>
        <div className="brand-copy">
          <span className="eyebrow">MỘT KHỞI ĐẦU MỚI</span>
          <h2>
            Chạm mở
            <br />
            tương lai của bạn.
          </h2>
          <p>
            Mở tài khoản trực tuyến chỉ trong vài phút.
            <br />
            Đơn giản, thuận tiện và luôn bên bạn.
          </p>
          <div className="bank-card">
            <div>
              Anomaly<span>◈</span>
            </div>
            <FontAwesomeIcon icon={faFingerprint} />
            <p>YOUR NEXT CHAPTER</p>
            <strong>Bắt đầu từ hôm nay.</strong>
          </div>
        </div>
        <div className="brand-bottom">
          <FontAwesomeIcon icon={faShieldHalved} /> Đồng hành trên từng bước
        </div>
      </aside>
      <main className="flow-shell">
        <div className="mobile-brand">
          <span className="brand-mark">A</span> AnomalyBank
        </div>
        <nav className="step-nav" aria-label="Tiến trình mở tài khoản">
          <button
            className="icon-button"
            aria-label="Quay lại"
            disabled={
              busy ||
              !!createdAccount ||
              screen === "email" ||
              screen === "processing" ||
              screen === "success" ||
              screen === "error"
            }
            onClick={() => {
              go(stages[Math.max(0, step - 1)]);
            }}
          >
            <FontAwesomeIcon icon={faArrowLeft} />
          </button>
          <span>
            {step >= 0 ? `BƯỚC ${step + 1}/${stages.length}` : "AnomalyBank"}
          </span>
          <button className="help-button" onClick={() => setDialog("Hỗ trợ")}>
            Trợ giúp
          </button>
        </nav>
        {step >= 0 && (
          <div
            className="progress"
            role="progressbar"
            aria-label={titles[step]}
            aria-valuemin={0}
            aria-valuemax={stages.length}
            aria-valuenow={step + 1}
          >
            <span style={{ width: `${((step + 1) / stages.length) * 100}%` }} />
          </div>
        )}
        <div className="screen" key={screen}>
          <RegistrationStep flow={flow} onLogin={onLogin} />
        </div>
        <footer className="flow-footer">
          <FontAwesomeIcon icon={faShieldHalved} /> AnomalyBank · Khởi đầu hành
          trình của bạn
        </footer>
      </main>
      {dialog && <InfoDialog title={dialog} onClose={() => setDialog("")} />}
    </div>
  );
}
