import { navigate, routes } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import { TransferFlow } from "@/features/transfer";

const TransferPage = () => {
  const { user } = useAuth();

  if (!user) {
    return (
      <div className="p-6 text-sm text-on-surface-variant">
        Đang tải thông tin tài khoản...
      </div>
    );
  }

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-gradient-to-b from-secondary-container/40 to-surface-container-lowest">
      <TransferFlow
        source={{
          accountNo: user.accountNo,
          ownerName: user.fullName || user.username,
          balance: user.amount,
        }}
        onExit={() => navigate(routes.dashboard)}
        onViewHistory={() => navigate(routes.transactions)}
      />
    </div>
  );
};

export default TransferPage;
