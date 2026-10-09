import { useEffect, useRef, useState } from "react";
import { navigate, routes, transactionDetailRoute } from "@/app/routes";
import { useAuth } from "@/features/auth/useAuth";
import { TransactionHistory } from "@/features/transactions";
import {
  getTransactionSummary,
  listTransactions,
  type TransactionSummary,
} from "@/features/transactions/api";
import type {
  TransactionFilter,
  TransactionRecord,
} from "@/features/transactions/model";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { PageHeader } from "@/shared/layout";
import { ApiError } from "@/shared/lib/http";
import { Button } from "@/shared/ui";

const SEARCH_DEBOUNCE_MS = 350;

const TransactionHistoryPage = () => {
  const { token, logout } = useAuth();
  const [records, setRecords] = useState<TransactionRecord[]>([]);
  const [summary, setSummary] = useState<TransactionSummary | null>(null);
  const [filter, setFilter] = useState<TransactionFilter>("all");
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const loadMoreController = useRef<AbortController | null>(null);

  useEffect(() => {
    const timer = setTimeout(
      () => setDebouncedQuery(query),
      SEARCH_DEBOUNCE_MS,
    );
    return () => clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();

    loadMoreController.current?.abort();
    void Promise.resolve().then(() => {
      if (controller.signal.aborted) return;
      setLoadingMore(false);
      setNextCursor(null);
      setLoading(true);
      setError(null);
    });
    void listTransactions(token, {
      direction: filter,
      query: debouncedQuery,
      signal: controller.signal,
    })
      .then((page) => {
        setRecords(page.items);
        setNextCursor(page.nextCursor);
      })
      .catch((requestError: unknown) => {
        if (controller.signal.aborted) return;
        if (
          requestError instanceof ApiError &&
          requestError.status === HTTP_STATUS.UNAUTHORIZED
        ) {
          logout();
          return;
        }
        setError(
          requestError instanceof Error
            ? requestError.message
            : "Không thể tải lịch sử giao dịch.",
        );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });

    return () => {
      controller.abort();
      loadMoreController.current?.abort();
    };
  }, [token, filter, debouncedQuery, reloadKey, logout]);

  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();

    void getTransactionSummary(token, new Date(), { signal: controller.signal })
      .then(setSummary)
      .catch((requestError: unknown) => {
        if (controller.signal.aborted) return;
        if (
          requestError instanceof ApiError &&
          requestError.status === HTTP_STATUS.UNAUTHORIZED
        ) {
          logout();
          return;
        }
        setError(
          requestError instanceof Error
            ? requestError.message
            : "Không thể tải tổng quan tháng.",
        );
      });

    return () => controller.abort();
  }, [token, reloadKey, logout]);

  const loadMore = async () => {
    if (!token || !nextCursor || loadingMore) return;
    const controller = new AbortController();
    loadMoreController.current = controller;
    setLoadingMore(true);
    setError(null);
    try {
      const page = await listTransactions(token, {
        direction: filter,
        query: debouncedQuery,
        cursor: nextCursor,
        signal: controller.signal,
      });
      setRecords((current) => [...current, ...page.items]);
      setNextCursor(page.nextCursor);
    } catch (requestError) {
      if (controller.signal.aborted) return;
      if (
        requestError instanceof ApiError &&
        requestError.status === HTTP_STATUS.UNAUTHORIZED
      ) {
        logout();
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Không thể tải thêm giao dịch.",
      );
    } finally {
      if (!controller.signal.aborted) setLoadingMore(false);
    }
  };

  const retry = () => setReloadKey((key) => key + 1);
  const initialLoading = loading && records.length === 0;

  return (
    <div className="mx-auto flex h-screen max-w-md flex-col overflow-hidden bg-gradient-to-b from-secondary-container/40 to-surface-container-lowest">
      <PageHeader
        title="Lịch sử giao dịch"
        onBack={() => navigate(routes.dashboard)}
        backLabel="Về trang chủ"
      />
      <main className="flex-1 overflow-y-auto px-4 py-5">
        {initialLoading || !summary ? (
          error ? (
            <div
              className="rounded-3xl bg-surface-container-lowest px-6 py-10 text-center"
              role="alert"
            >
              <p className="font-semibold text-on-surface">
                Không thể tải lịch sử giao dịch
              </p>
              <p className="mt-1 text-sm text-on-surface-variant">{error}</p>
              <Button className="mt-4" onClick={retry}>
                Thử lại
              </Button>
            </div>
          ) : (
            <p
              className="py-10 text-center text-sm text-on-surface-variant"
              role="status"
            >
              Đang tải lịch sử giao dịch...
            </p>
          )
        ) : (
          <div className="space-y-4">
            {error && (
              <div
                className="rounded-2xl bg-error-container p-4 text-sm text-on-error-container"
                role="alert"
              >
                <p>{error}</p>
                <button
                  type="button"
                  className="mt-2 font-semibold underline"
                  onClick={retry}
                >
                  Thử lại
                </button>
              </div>
            )}
            <TransactionHistory
              records={records}
              monthlySummary={summary}
              filter={filter}
              onFilterChange={setFilter}
              query={query}
              onQueryChange={setQuery}
              onSelect={(record) => navigate(transactionDetailRoute(record.id))}
            />
            {nextCursor && (
              <Button
                variant="secondary"
                className="w-full"
                disabled={loadingMore}
                onClick={() => void loadMore()}
              >
                {loadingMore ? "Đang tải thêm..." : "Tải thêm giao dịch"}
              </Button>
            )}
          </div>
        )}
      </main>
    </div>
  );
};

export default TransactionHistoryPage;
