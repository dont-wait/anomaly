import { useEffect, useState } from "react";
import { navigate, routes } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import { TransactionReceipt } from "@/features/transactions";
import { getTransaction } from "@/features/transactions/api";
import type { TransactionRecord } from "@/features/transactions/model";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { PageHeader } from "@/shared/layout";
import { ApiError } from "@/shared/lib/http";
import { Button } from "@/shared/ui";

const TransactionDetailPage = ({
  transactionId,
}: {
  transactionId: string;
}) => {
  const { token, logout } = useAuth();
  const [record, setRecord] = useState<TransactionRecord | null>(null);
  const [loading, setLoading] = useState(true);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    void Promise.resolve().then(() => {
      if (controller.signal.aborted) return;
      setLoading(true);
      setRecord(null);
      setNotFound(false);
      setError(null);
    });

    void getTransaction(token, transactionId, { signal: controller.signal })
      .then(setRecord)
      .catch((requestError: unknown) => {
        if (controller.signal.aborted) return;
        if (requestError instanceof ApiError) {
          if (requestError.status === HTTP_STATUS.UNAUTHORIZED) {
            logout();
            return;
          }
          if (requestError.status === HTTP_STATUS.NOT_FOUND) {
            setNotFound(true);
            return;
          }
        }
        setError(
          requestError instanceof Error
            ? requestError.message
            : "Không thể tải giao dịch.",
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => controller.abort();
  }, [token, transactionId, reloadKey, logout]);

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-linear-to-b from-secondary-container/40 to-surface-container-lowest">
      <PageHeader
        title="Chi tiết giao dịch"
        onBack={() => navigate(routes.transactions)}
        backLabel="Về lịch sử giao dịch"
      />
      <main className="flex-1 space-y-4 overflow-y-auto px-4 py-5">
        {loading ? (
          <p
            className="py-10 text-center text-sm text-on-surface-variant"
            role="status"
          >
            Đang tải giao dịch...
          </p>
        ) : record ? (
          <TransactionReceipt record={record} />
        ) : notFound ? (
          <div className="rounded-3xl bg-surface-container-lowest px-6 py-10 text-center">
            <p className="font-semibold text-on-surface">
              Không tìm thấy giao dịch
            </p>
            <p className="mt-1 text-sm text-on-surface-variant">
              Giao dịch không tồn tại hoặc đã bị xoá khỏi lịch sử.
            </p>
          </div>
        ) : (
          <div
            className="rounded-3xl bg-surface-container-lowest px-6 py-10 text-center"
            role="alert"
          >
            <p className="font-semibold text-on-surface">
              Không thể tải giao dịch
            </p>
            <p className="mt-1 text-sm text-on-surface-variant">{error}</p>
            <Button
              className="mt-4"
              onClick={() => setReloadKey((key) => key + 1)}
            >
              Thử lại
            </Button>
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
