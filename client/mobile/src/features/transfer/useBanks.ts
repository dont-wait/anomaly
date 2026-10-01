import { useCallback, useEffect, useState } from "react";
import { fetchBanks } from "@/features/transfer/api/banks";
import { ANOMALY_BANK, type Bank } from "@/features/transfer/model";

export type BanksStatus = "loading" | "ready" | "error";

export function useBanks() {
  const [banks, setBanks] = useState<Bank[]>([ANOMALY_BANK]);
  const [status, setStatus] = useState<BanksStatus>("loading");
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let active = true;
    fetchBanks()
      .then((list) => {
        if (!active) return;
        setBanks(list);
        setStatus("ready");
      })
      .catch(() => {
        if (active) setStatus("error");
      });
    return () => {
      active = false;
    };
  }, [attempt]);

  const retry = useCallback(() => {
    setStatus("loading");
    setAttempt((n) => n + 1);
  }, []);

  return { banks, status, retry };
}
