import { useCallback, useEffect, useRef, useState } from "react";
import {
  getTransactionReport,
  type TransactionReport,
} from "./api/transactionsReportApi";
import { isReportDate } from "./api/reportValidation";

interface ReportState {
  date: string;
  report: TransactionReport | null;
  pending: boolean;
  stale: boolean;
  error: string | null;
}

export function useTransactionReport(date: string) {
  const [state, setState] = useState<ReportState>({
    date,
    report: null,
    pending: true,
    stale: false,
    error: null,
  });
  const requestRef = useRef<AbortController | null>(null);
  const load = useCallback(
    async (background = false) => {
      // A polling tick must not repeatedly abort a slow request. Explicit refresh
      // can supersede it; date changes and unmount always invalidate the old one.
      if (background && requestRef.current) return;
      requestRef.current?.abort();
      if (!isReportDate(date)) {
        requestRef.current = null;
        setState({
          date,
          report: null,
          pending: false,
          stale: false,
          error: "Chọn ngày báo cáo hợp lệ (YYYY-MM-DD).",
        });
        return;
      }
      const controller = new AbortController();
      requestRef.current = controller;
      const isCurrent = () =>
        !controller.signal.aborted && requestRef.current === controller;
      setState((previous) => ({
        date,
        report: previous.date === date ? previous.report : null,
        pending: true,
        stale: previous.date === date && previous.stale,
        error: null,
      }));
      try {
        const report = await getTransactionReport(date, controller.signal);
        if (isCurrent())
          setState({ date, report, pending: false, stale: false, error: null });
      } catch (cause) {
        if (!isCurrent()) return;
        setState((previous) => ({
          ...previous,
          pending: false,
          stale: previous.report !== null,
          error:
            cause instanceof Error
              ? cause.message
              : "Không thể tải báo cáo giao dịch.",
        }));
      } finally {
        if (requestRef.current === controller) requestRef.current = null;
      }
    },
    [date],
  );

  useEffect(() => {
    const initial = window.setTimeout(() => void load(), 0);
    const poll = isReportDate(date)
      ? window.setInterval(() => void load(true), 15000)
      : undefined;
    return () => {
      window.clearTimeout(initial);
      window.clearInterval(poll);
      requestRef.current?.abort();
      requestRef.current = null;
    };
  }, [date, load]);

  const report = state.date === date ? state.report : null;
  const pending = state.date !== date || state.pending;
  const stale = state.date === date && state.stale;
  const error = state.date === date ? state.error : null;
  return {
    report,
    pending,
    stale,
    error,
    refresh: () => load(),
    canExport: !!report && !pending && !stale && !error,
  };
}
