import { useState } from "react";
import { navigate, routes } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import "@/features/savings/savings.css";
import { ArrowLeftIcon } from "@/shared/icons";
import { BottomNav } from "@/shared/layout/BottomNav";
import { Logo } from "@/shared/layout/Logo";
import { ProfileSidebar } from "@/shared/layout/ProfileSidebar";
import { StepForm } from "@/shared/ui";
import "./cards.css";

type Screen = "home" | "review" | "pending";

const STEP_NUMBER = { review: 1, pending: 2 } as const;
const STEP_HEADING = {
  review: "Kiểm tra thông tin thẻ",
  pending: "Sắp ra mắt",
} as const;

function CardFace({
  ownerName,
  accountNo,
  badge,
}: {
  ownerName: string;
  accountNo: string;
  badge: string;
}) {
  return (
    <section aria-label={badge} className="cards-default">
      <div className="flex items-center justify-between">
        <strong>AnomalyBank</strong>
        <span className="cards-default-badge">{badge}</span>
      </div>
      <p className="cards-default-label">ANOMALYBANK SIGNATURE</p>
      <div className="flex items-end justify-between gap-3">
        <div className="min-w-0">
          <p className="cards-default-caption">Chủ thẻ</p>
          <p className="truncate font-semibold">{ownerName}</p>
        </div>
        <div className="shrink-0 text-right">
          <p className="cards-default-caption">Tài khoản liên kết</p>
          <p className="font-semibold">•••• {accountNo.slice(-4)}</p>
        </div>
      </div>
    </section>
  );
}

/** Chỉ là giao diện: server chưa có thẻ nên luồng không gọi API và không tạo thẻ. */
export default function CardsPage() {
  const { user, token, logout } = useAuth();
  const [screen, setScreen] = useState<Screen>("home");
  const [profileOpen, setProfileOpen] = useState(false);

  if (!user) {
    return (
      <div className="p-6 text-sm text-on-surface-variant">
        Đang tải thông tin tài khoản...
      </div>
    );
  }

  const ownerName = user.fullName || user.username;

  if (screen !== "home") {
    return (
      <div className="savings-shell">
        <StepForm
          title="Mở thẻ mới"
          stepId={screen}
          stepNumber={STEP_NUMBER[screen]}
          stepCount={2}
          heading={STEP_HEADING[screen]}
          hint={
            screen === "review"
              ? "Thẻ mới giống thẻ mặc định và liên kết với cùng tài khoản."
              : undefined
          }
          canContinue
          continueLabel={screen === "pending" ? "Về màn Thẻ" : "Tiếp tục"}
          onBack={() => setScreen(screen === "pending" ? "review" : "home")}
          onNext={() => setScreen(screen === "review" ? "pending" : "home")}
        >
          {screen === "review" && (
            <CardFace
              ownerName={ownerName}
              accountNo={user.accountNo}
              badge="Thẻ mới"
            />
          )}
          {screen === "pending" && (
            <p role="status" className="savings-panel">
              Tính năng mở thêm thẻ đang được phát triển. Chưa có thẻ mới nào
              được tạo và tài khoản của bạn không thay đổi.
            </p>
          )}
        </StepForm>
      </div>
    );
  }

  return (
    <div className="savings-shell">
      <header className="savings-topbar">
        <button
          type="button"
          className="savings-back"
          aria-label="Về trang chủ"
          onClick={() => navigate(routes.dashboard)}
        >
          <ArrowLeftIcon className="h-5 w-5" />
        </button>
        <div className="savings-topbar-copy">
          <h1>Thẻ của tôi</h1>
          <p>Quản lý thẻ AnomalyBank</p>
        </div>
        <Logo className="savings-logo" />
      </header>
      <main className="savings-content savings-content-with-nav">
        <CardFace
          ownerName={ownerName}
          accountNo={user.accountNo}
          badge="Thẻ mặc định"
        />
        <button
          type="button"
          className="savings-button"
          onClick={() => setScreen("review")}
        >
          Mở thẻ mới
        </button>
      </main>
      <BottomNav
        active={profileOpen ? "profile" : "card"}
        onQrScan={() => navigate(routes.qrScan)}
        onChange={(id) => {
          if (id === "home") navigate(routes.dashboard);
          if (id === "savings") navigate(routes.savings);
          if (id === "profile") setProfileOpen(true);
        }}
      />
      <ProfileSidebar
        open={profileOpen}
        name={ownerName}
        token={token}
        onClose={() => setProfileOpen(false)}
        onLogout={() => {
          setProfileOpen(false);
          logout();
        }}
      />
    </div>
  );
}
