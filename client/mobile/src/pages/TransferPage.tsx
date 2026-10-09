import { navigate, routes } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import { TransferFlow } from "@/features/transfer";

const TransferPage = () => {
  const { user, token, logout, refreshProfile } = useAuth();

  if (!user || !token) {
    return (
      <div className="p-6 text-sm text-on-surface-variant">
        Đang tải thông tin tài khoản...
      </div>
    );
  }

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-linear-to-b from-secondary-container/40 to-surface-container-lowest">
      <TransferFlow
        token={token}
        source={{
          accountNo: user.accountNo,
          ownerName: user.fullName || user.username,
          balance: user.amount,
        }}
        onUnauthorized={logout}
        onExit={() =>
          void refreshProfile()
            .catch(() => undefined)
            .finally(() => navigate(routes.dashboard))
        }
        onViewHistory={() => navigate(routes.transactions)}
      />
    </div>
  );
};

export default TransferPage;
