/** Persist the exact command for safe retry after an ambiguous network/503 result. */
export type PendingSavingsCommand =
  | { kind: "open"; key: string; quoteId: string; sourceAccountId: string }
  | { kind: "withdraw"; key: string; quoteId: string; savingsId: string };

const keyFor = (actorId: string) => `anomaly:savings:pending:${actorId}`;

export function loadPending(actorId: string): PendingSavingsCommand | null {
  try {
    const raw = sessionStorage.getItem(keyFor(actorId));
    if (!raw) return null;
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object") return null;
    const data = value as Record<string, unknown>;
    if ((data.kind !== "open" && data.kind !== "withdraw") ||
      typeof data.key !== "string" || typeof data.quoteId !== "string") return null;
    if (data.kind === "open" && typeof data.sourceAccountId === "string") return data as unknown as PendingSavingsCommand;
    if (data.kind === "withdraw" && typeof data.savingsId === "string") return data as unknown as PendingSavingsCommand;
  } catch { /* storage may be blocked */ }
  return null;
}
export function savePending(actorId: string, operation: PendingSavingsCommand): void {
  try { sessionStorage.setItem(keyFor(actorId), JSON.stringify(operation)); }
  catch { /* in-memory component state still retains the operation */ }
}
export function clearPending(actorId: string): void {
  try { sessionStorage.removeItem(keyFor(actorId)); } catch { /* ignore */ }
}
