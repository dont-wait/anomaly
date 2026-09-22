import { navigate, routes } from "@/app/routes";
import { TransactionReceipt, transactionStore } from "@/features/transactions";
import { PageHeader } from "@/shared/layout";
import { Button } from "@/shared/ui";

const TransactionDetailPage = ({
  transactionId,
}: {
  transactionId: string;
}) => {
  const record = transactionStore.get(transactionId);

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-gradient-to-b from-secondary-container/40 to-surface-container-lowest">
      <PageHeader
        title="Chi tiết giao dịch"
        onBack={() => navigate(routes.transactions)}
        backLabel="Về lịch sử giao dịch"
      />
      <main className="flex-1 space-y-4 overflow-y-auto px-4 py-5">
        {record ? (
          <TransactionReceipt record={record} />
        ) : (
          <div className="rounded-3xl bg-surface-container-lowest px-6 py-10 text-center">
            <p className="font-semibold text-on-surface">
              Không tìm thấy giao dịch
            </p>
            <p className="mt-1 text-sm text-on-surface-variant">
              Giao dịch không tồn tại hoặc đã bị xoá khỏi lịch sử.
            </p>
          </div>
        )}
        <Button
          variant="secondary"
          className="w-full"
          onClick={() => navigate(routes.transactions)}
        >
          Về lịch sử giao dịch
        </Button>
      </main>
    </div>
  );
};

export default TransactionDetailPage;
