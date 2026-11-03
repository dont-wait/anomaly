import { navigate, routes, transferToAccount } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import { MyQrCode } from "@/features/qr/MyQrCode";
import { QrScanner } from "@/features/qr/QrScanner";
import { PageHeader } from "@/shared/layout";

const TABS = [
  { label: "Quét mã", route: routes.qrScan },
  { label: "QR của tôi", route: routes.myQr },
] as const;

const QrPage = ({ tab }: { tab: "scan" | "mine" }) => {
  const { user } = useAuth();
  const current = tab === "scan" ? routes.qrScan : routes.myQr;

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-background">
      <PageHeader
        title="Mã QR"
        onBack={() => navigate(routes.dashboard)}
        backLabel="Về trang chủ"
      />
      <nav aria-label="Mã QR" className="grid grid-cols-2 gap-2 px-4 pt-4">
        {TABS.map(({ label, route }) => (
          <button
            key={route}
            type="button"
            aria-current={route === current ? "page" : undefined}
            onClick={() => navigate(route)}
            className={`min-h-11 rounded-full text-sm font-bold ${
              route === current
                ? "bg-primary text-white"
                : "bg-surface-container text-primary"
            }`}
          >
            {label}
          </button>
        ))}
      </nav>
      <main className="flex-1 overflow-y-auto p-4">
        {tab === "scan" ? (
          <QrScanner
            onAccountNo={(accountNo) => navigate(transferToAccount(accountNo))}
          />
        ) : user ? (
          <MyQrCode
            accountNo={user.accountNo}
            ownerName={user.fullName || user.username}
          />
        ) : (
          <p className="text-sm text-on-surface-variant" role="status">
            Đang tải thông tin tài khoản...
          </p>
        )}
      </main>
    </div>
  );
};

export default QrPage;
